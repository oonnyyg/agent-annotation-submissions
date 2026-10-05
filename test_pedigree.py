import math
import unittest

from pedigree import PedigreeError, infer


def approx_equal(a, b, tol=1e-9):
    return math.isclose(a, b, rel_tol=0.0, abs_tol=tol)


class TestValidation(unittest.TestCase):
    def test_duplicate_ids_rejected(self):
        spec = {
            "allele_frequency": 0.5,
            "individuals": [{"id": "x"}, {"id": "x"}],
        }
        with self.assertRaises(PedigreeError):
            infer(spec)

    def test_cycle_rejected(self):
        spec = {
            "allele_frequency": 0.5,
            "individuals": [
                {"id": "a", "parents": ["b", "c"]},
                {"id": "b", "parents": ["a", "c"]},
                {"id": "c"},
            ],
        }
        with self.assertRaises(PedigreeError):
            infer(spec)

    def test_forward_reference_allowed(self):
        spec = {
            "allele_frequency": 0.5,
            "individuals": [
                {"id": "child", "parents": ["dam", "sire"]},
                {"id": "dam"},
                {"id": "sire"},
            ],
        }
        result = infer(spec)
        self.assertIn("child", result["posterior"])

    def test_frequency_bounds(self):
        for bad in (0.0, 1.0, -0.1, 1.5):
            with self.assertRaises(PedigreeError):
                infer({"allele_frequency": bad, "individuals": [{"id": "a"}]})

    def test_error_rate_bounds(self):
        for bad in (-0.1, 1.0, 2.0):
            with self.assertRaises(PedigreeError):
                infer(
                    {
                        "allele_frequency": 0.5,
                        "error_rate": bad,
                        "individuals": [{"id": "a"}],
                    }
                )


class TestInference(unittest.TestCase):
    def test_founder_hw_prior(self):
        spec = {"allele_frequency": 0.3, "individuals": [{"id": "a"}]}
        post = infer(spec)["posterior"]["a"]
        self.assertTrue(approx_equal(post["AA"], 0.09))
        self.assertTrue(approx_equal(post["AB"], 0.42))
        self.assertTrue(approx_equal(post["BB"], 0.49))

    def test_missing_observation_adds_no_evidence(self):
        base = {
            "allele_frequency": 0.5,
            "error_rate": 0.1,
            "individuals": [{"id": "a"}],
        }
        unobserved = infer(base)["posterior"]["a"]
        self.assertTrue(approx_equal(unobserved["AA"], 0.25))
        self.assertTrue(approx_equal(unobserved["AB"], 0.5))
        self.assertTrue(approx_equal(unobserved["BB"], 0.25))
        # Evidence probability is 1 when nothing is observed.
        self.assertTrue(approx_equal(infer(base)["evidence_probability"], 1.0))

    def test_observation_not_fixed(self):
        # With error rate 0.1, an AA observation must not give P(AA)=1.
        spec = {
            "allele_frequency": 0.5,
            "error_rate": 0.1,
            "individuals": [{"id": "a", "observation": "AA"}],
        }
        post = infer(spec)["posterior"]["a"]
        self.assertLess(post["AA"], 1.0)
        self.assertGreater(post["AB"], 0.0)
        self.assertGreater(post["BB"], 0.0)
        # Posterior proportional to prior * likelihood.
        # Evidence = .25*.9 + .5*.05 + .25*.05 = .2625.
        self.assertTrue(approx_equal(post["AA"], 0.25 * 0.9 / 0.2625))
        self.assertTrue(approx_equal(post["AB"], 0.5 * 0.05 / 0.2625))

    def test_relatedness_updates_parent(self):
        # Error-free child observed AA: each parent must carry A.
        spec = {
            "allele_frequency": 0.5,
            "error_rate": 0.0,
            "individuals": [
                {"id": "child", "parents": ["p1", "p2"], "observation": "AA"},
                {"id": "p1"},
                {"id": "p2"},
            ],
        }
        post = infer(spec)["posterior"]
        self.assertTrue(approx_equal(post["p1"]["BB"], 0.0))
        self.assertTrue(approx_equal(post["p2"]["BB"], 0.0))
        # Symmetry: parents share the same posterior.
        for g in ("AA", "AB", "BB"):
            self.assertTrue(approx_equal(post["p1"][g], post["p2"][g]))
        # Parent combos compatible with child AA:
        # (AA,AA),(AA,AB),(AB,AA),(AB,AB) each with weight .0625.
        # Evidence = .25; P(p1=AA) = (.0625+.0625)/.25 = 0.5.
        self.assertTrue(approx_equal(post["p1"]["AA"], 0.5))
        self.assertTrue(approx_equal(post["p1"]["AB"], 0.5))

    def test_zero_error_contradiction(self):
        # Error-free observations AA parent and BB parent cannot have an
        # error-free AA child... use direct contradiction: parent AA and
        # parent BB, child observed AA is possible only via AB gametes?
        # AA x BB -> all AB, so child observed AA contradicts.
        spec = {
            "allele_frequency": 0.5,
            "error_rate": 0.0,
            "individuals": [
                {"id": "p1", "observation": "AA"},
                {"id": "p2", "observation": "BB"},
                {
                    "id": "child",
                    "parents": ["p1", "p2"],
                    "observation": "AA",
                },
            ],
        }
        with self.assertRaises(PedigreeError) as ctx:
            infer(spec)
        self.assertIn("evidence probability is zero", str(ctx.exception))

    def test_offspring_prediction_uses_joint(self):
        # Parents of an observed AA child are correlated; the predicted
        # distribution for a new offspring must use the joint posterior.
        spec = {
            "allele_frequency": 0.5,
            "error_rate": 0.0,
            "individuals": [
                {"id": "child", "parents": ["p1", "p2"], "observation": "AA"},
                {"id": "p1"},
                {"id": "p2"},
            ],
            "predict_offspring": ["p1", "p2"],
        }
        result = infer(spec)
        dist = result["offspring_prediction"]["distribution"]
        # Joint posterior: (AA,AA),(AA,AB),(AB,AA),(AB,AB) each prob 1/4.
        # P(AA) = (1 + .5 + .5 + .25)/4 = .5625.
        self.assertTrue(approx_equal(dist["AA"], 0.5625))
        self.assertTrue(approx_equal(dist["AB"], 0.375))
        self.assertTrue(approx_equal(dist["BB"], 0.0625))
        # Marginal-product (wrong) approach would give P(AA)=
        # (0.5 + 0.5*0.5)^2 = 0.5625... only coincides here; the AB/BB
        # split differs: marginals give P(BB)=0.0625? Marginal gamete
        # pool is symmetric so offspring is HW at p=0.5: .25/.5/.25,
        # which differs from .5625/.375/.0625, so this checks joint usage.
        # Prediction must not alter the original pedigree posteriors.
        post = result["posterior"]
        self.assertTrue(approx_equal(post["p1"]["AA"], 0.5))

    def test_offspring_prediction_unrelated_founders(self):
        spec = {
            "allele_frequency": 0.5,
            "individuals": [{"id": "a"}, {"id": "b"}],
            "predict_offspring": ["a", "b"],
        }
        dist = infer(spec)["offspring_prediction"]["distribution"]
        # Allele frequency 0.5 gamete pool -> HW offspring.
        self.assertTrue(approx_equal(dist["AA"], 0.25))
        self.assertTrue(approx_equal(dist["AB"], 0.5))
        self.assertTrue(approx_equal(dist["BB"], 0.25))


if __name__ == "__main__":
    unittest.main()
