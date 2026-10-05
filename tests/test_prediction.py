import math
import unittest

from pedigree_inference import PedigreeError, infer, predict_offspring
from tests.test_inference import make_pedigree


class TestOffspringPrediction(unittest.TestCase):
    def test_joint_posterior_used_for_related_parents(self):
        ped = make_pedigree(
            [
                {"id": "gf"},
                {"id": "gm"},
                {"id": "s1", "parents": ["gf", "gm"], "observation": "AB"},
                {"id": "s2", "parents": ["gf", "gm"], "observation": "AB"},
            ],
            allele_freq=0.5,
            error_rate=0.1,
        )
        result = infer(ped)
        pred = predict_offspring(ped, result, "s1", "s2")
        self.assertAlmostEqual(sum(pred.values()), 1.0)
        # Related parents are correlated: joint != product of marginals.
        joint_ab_ab = sum(
            p for s, p in result.joint.items() if s[2] == "AB" and s[3] == "AB"
        )
        marg = result.posterior["s1"]["AB"] * result.posterior["s2"]["AB"]
        self.assertFalse(
            math.isclose(joint_ab_ab / result.evidence, marg)
        )

    def test_unobserved_founders_child_prediction(self):
        ped = make_pedigree([{"id": "f"}, {"id": "m"}], allele_freq=0.5)
        pred = predict_offspring(ped, infer(ped), "f", "m")
        self.assertAlmostEqual(pred["AA"], 0.25)
        self.assertAlmostEqual(pred["AB"], 0.5)
        self.assertAlmostEqual(pred["BB"], 0.25)

    def test_prediction_does_not_modify_pedigree(self):
        ped = make_pedigree([{"id": "f"}, {"id": "m"}], allele_freq=0.5)
        before = infer(ped).posterior
        predict_offspring(ped, infer(ped), "f", "m")
        after = infer(ped).posterior
        self.assertEqual(before, after)
        self.assertEqual(len(ped.members), 2)

    def test_same_parent_rejected(self):
        ped = make_pedigree([{"id": "f"}])
        with self.assertRaises(PedigreeError):
            predict_offspring(ped, infer(ped), "f", "f")

    def test_unknown_parent_rejected(self):
        ped = make_pedigree([{"id": "f"}, {"id": "m"}])
        with self.assertRaises(PedigreeError):
            predict_offspring(ped, infer(ped), "f", "ghost")

    def test_known_parent_cross(self):
        ped = make_pedigree(
            [
                {"id": "f", "observation": "AA"},
                {"id": "m", "observation": "BB"},
            ],
            error_rate=0.0,
        )
        pred = predict_offspring(ped, infer(ped), "f", "m")
        self.assertEqual(pred, {"AA": 0.0, "AB": 1.0, "BB": 0.0})

    def test_contradictory_evidence_blocks_prediction(self):
        ped = make_pedigree(
            [
                {"id": "f", "observation": "AA"},
                {"id": "m", "observation": "AA"},
                {"id": "c", "parents": ["f", "m"], "observation": "BB"},
            ],
            error_rate=0.0,
        )
        with self.assertRaises(PedigreeError):
            predict_offspring(ped, infer(ped), "f", "m")


if __name__ == "__main__":
    unittest.main()
