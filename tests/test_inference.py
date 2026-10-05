import unittest

from pedigree_inference import PedigreeError, infer, load_pedigree


def make_pedigree(individuals, allele_freq=0.5, error_rate=0.0):
    return load_pedigree(
        {
            "allele_freq": allele_freq,
            "error_rate": error_rate,
            "individuals": individuals,
        }
    )


class TestValidation(unittest.TestCase):
    def test_duplicate_ids_rejected(self):
        with self.assertRaises(PedigreeError):
            make_pedigree([{"id": "a"}, {"id": "a"}])

    def test_cycle_rejected(self):
        with self.assertRaises(PedigreeError):
            make_pedigree(
                [
                    {"id": "a", "parents": ["b", "c"]},
                    {"id": "b", "parents": ["a", "c"]},
                    {"id": "c"},
                ]
            )

    def test_forward_reference_allowed(self):
        ped = make_pedigree(
            [{"id": "child", "parents": ["f", "m"]}, {"id": "f"}, {"id": "m"}]
        )
        self.assertEqual(len(ped.members), 3)

    def test_unknown_parent_rejected(self):
        with self.assertRaises(PedigreeError):
            make_pedigree([{"id": "a", "parents": ["x", "y"]}])

    def test_identical_parents_rejected(self):
        with self.assertRaises(PedigreeError):
            make_pedigree([{"id": "a"}, {"id": "b", "parents": ["a", "a"]}])

    def test_allele_freq_bounds(self):
        with self.assertRaises(PedigreeError):
            make_pedigree([{"id": "a"}], allele_freq=0.0)
        with self.assertRaises(PedigreeError):
            make_pedigree([{"id": "a"}], allele_freq=1.0)

    def test_error_rate_bounds(self):
        with self.assertRaises(PedigreeError):
            make_pedigree([{"id": "a"}], error_rate=1.0)
        with self.assertRaises(PedigreeError):
            make_pedigree([{"id": "a"}], error_rate=-0.1)

    def test_max_ten_individuals(self):
        with self.assertRaises(PedigreeError):
            make_pedigree([{"id": f"i{k}"} for k in range(11)])


class TestInference(unittest.TestCase):
    def test_founder_hw_prior_without_observation(self):
        ped = make_pedigree([{"id": "a"}], allele_freq=0.3)
        post = infer(ped).posterior["a"]
        self.assertAlmostEqual(post["AA"], 0.09)
        self.assertAlmostEqual(post["AB"], 0.42)
        self.assertAlmostEqual(post["BB"], 0.49)

    def test_missing_observation_adds_no_evidence(self):
        ped = make_pedigree(
            [{"id": "f"}, {"id": "m"}, {"id": "c", "parents": ["f", "m"]}],
            allele_freq=0.5,
        )
        result = infer(ped)
        self.assertAlmostEqual(result.evidence, 1.0)
        post = result.posterior["c"]
        self.assertAlmostEqual(post["AA"], 0.25)
        self.assertAlmostEqual(post["AB"], 0.5)
        self.assertAlmostEqual(post["BB"], 0.25)

    def test_observation_updates_but_does_not_fix(self):
        ped = make_pedigree(
            [{"id": "a", "observation": "AA"}], allele_freq=0.5, error_rate=0.1
        )
        post = infer(ped).posterior["a"]
        expected = 0.225 / 0.2625
        self.assertAlmostEqual(post["AA"], expected)
        self.assertGreater(post["AB"], 0.0)

    def test_kinship_propagates_evidence(self):
        ped = make_pedigree(
            [
                {"id": "f"},
                {"id": "m"},
                {"id": "c", "parents": ["f", "m"], "observation": "BB"},
            ],
            allele_freq=0.5,
            error_rate=0.0,
        )
        post = infer(ped).posterior
        self.assertEqual(post["f"]["AA"], 0.0)
        self.assertEqual(post["m"]["AA"], 0.0)
        self.assertAlmostEqual(post["f"]["BB"], 0.5)
        self.assertAlmostEqual(post["f"]["AB"], 0.5)

    def test_zero_error_contradiction(self):
        ped = make_pedigree(
            [
                {"id": "f", "observation": "AA"},
                {"id": "m", "observation": "AA"},
                {"id": "c", "parents": ["f", "m"], "observation": "BB"},
            ],
            error_rate=0.0,
        )
        result = infer(ped)
        self.assertTrue(result.contradictory)
        self.assertEqual(result.evidence, 0.0)

    def test_posterior_sums_to_one(self):
        ped = make_pedigree(
            [
                {"id": "f", "observation": "AB"},
                {"id": "m"},
                {"id": "c", "parents": ["f", "m"], "observation": "AA"},
            ],
            allele_freq=0.4,
            error_rate=0.05,
        )
        result = infer(ped)
        for dist in result.posterior.values():
            self.assertAlmostEqual(sum(dist.values()), 1.0)


if __name__ == "__main__":
    unittest.main()
