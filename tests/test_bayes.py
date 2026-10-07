import itertools
import json
import math
import unittest
from pathlib import Path

from bayes.inference import QueryError, run_query
from bayes.model import ModelError, load_network, parse_network

EXAMPLE = Path(__file__).resolve().parent.parent / "examples" / "network.json"


def make_net(variables):
    return parse_network({"variables": variables})


def converging_net():
    """A -> C <- B collider."""
    return make_net([
        {"name": "A", "states": ["f", "t"], "parents": [],
         "cpt": [{"when": [], "probs": {"f": 0.4, "t": 0.6}}]},
        {"name": "B", "states": ["f", "t"], "parents": [],
         "cpt": [{"when": [], "probs": {"f": 0.7, "t": 0.3}}]},
        {"name": "C", "states": ["x", "y"], "parents": ["A", "B"],
         "cpt": [
             {"when": ["f", "f"], "probs": {"x": 1.0, "y": 0.0}},
             {"when": ["f", "t"], "probs": {"x": 0.0, "y": 1.0}},
             {"when": ["t", "f"], "probs": {"x": 0.0, "y": 1.0}},
             {"when": ["t", "t"], "probs": {"x": 0.5, "y": 0.5}},
         ]},
    ])


class ModelValidationTests(unittest.TestCase):
    def test_example_loads(self):
        net = load_network(str(EXAMPLE))
        self.assertEqual(net.order[0:2], ["Burglary", "Earthquake"])

    def test_duplicate_states_rejected(self):
        with self.assertRaises(ModelError):
            make_net([{"name": "A", "states": ["x", "x"], "parents": [],
                       "cpt": [{"when": [], "probs": {"x": 1.0}}]}])

    def test_missing_parent_combo_rejected(self):
        with self.assertRaisesRegex(ModelError, "missing CPT rows"):
            make_net([
                {"name": "A", "states": ["f", "t"], "parents": [],
                 "cpt": [{"when": [], "probs": {"f": 0.5, "t": 0.5}}]},
                {"name": "B", "states": ["f", "t"], "parents": ["A"],
                 "cpt": [{"when": ["f"], "probs": {"f": 0.5, "t": 0.5}}]},
            ])

    def test_row_not_summing_to_one_rejected(self):
        with self.assertRaisesRegex(ModelError, "sum to 1"):
            make_net([{"name": "A", "states": ["f", "t"], "parents": [],
                       "cpt": [{"when": [], "probs": {"f": 0.5, "t": 0.4}}]}])

    def test_negative_probability_rejected(self):
        with self.assertRaisesRegex(ModelError, "negative"):
            make_net([{"name": "A", "states": ["f", "t"], "parents": [],
                       "cpt": [{"when": [], "probs": {"f": 1.5, "t": -0.5}}]}])

    def test_cycle_rejected(self):
        with self.assertRaisesRegex(ModelError, "cycle"):
            make_net([
                {"name": "A", "states": ["x"], "parents": ["B"],
                 "cpt": [{"when": ["x"], "probs": {"x": 1.0}}]},
                {"name": "B", "states": ["x"], "parents": ["A"],
                 "cpt": [{"when": ["x"], "probs": {"x": 1.0}}]},
            ])

    def test_too_many_variables_rejected(self):
        variables = [{"name": f"V{i}", "states": ["x"], "parents": [],
                      "cpt": [{"when": [], "probs": {"x": 1.0}}]}
                     for i in range(13)]
        with self.assertRaisesRegex(ModelError, "at most 12"):
            make_net(variables)


class InferenceTests(unittest.TestCase):
    def test_converging_dependency(self):
        net = converging_net()
        prior = run_query(net, ["A"], {})
        self.assertAlmostEqual(prior.distribution["t"], 0.6)
        post = run_query(net, ["A"], {"C": "y"})
        # P(C=y) = 0.4*0.3 + 0.6*0.7 + 0.6*0.3*0.5 = 0.63
        self.assertAlmostEqual(post.evidence_probability, 0.63)
        self.assertAlmostEqual(post.distribution["t"], (0.42 + 0.09) / 0.63)
        joint = run_query(net, ["A", "B"], {"C": "y"})
        self.assertAlmostEqual(sum(joint.distribution.values()), 1.0)
        self.assertEqual(joint.map_assignment, {"A": "t", "B": "f"})

    def test_impossible_evidence_raises(self):
        net = make_net([
            {"name": "A", "states": ["f", "t"], "parents": [],
             "cpt": [{"when": [], "probs": {"f": 0.5, "t": 0.5}}]},
            {"name": "B", "states": ["x", "y"], "parents": ["A"],
             "cpt": [
                 {"when": ["f"], "probs": {"x": 0.5, "y": 0.5}},
                 {"when": ["t"], "probs": {"x": 1.0, "y": 0.0}},
             ]},
            {"name": "C", "states": ["u", "v"], "parents": ["B"],
             "cpt": [
                 {"when": ["x"], "probs": {"u": 0.5, "v": 0.5}},
                 {"when": ["y"], "probs": {"u": 0.5, "v": 0.5}},
             ]},
        ])
        with self.assertRaisesRegex(QueryError, "zero probability"):
            run_query(net, ["C"], {"A": "t", "B": "y"})

    def test_empty_evidence_supported(self):
        net = load_network(str(EXAMPLE))
        result = run_query(net, ["Alarm"], {})
        self.assertAlmostEqual(result.evidence_probability, 1.0)
        self.assertAlmostEqual(sum(result.distribution.values()), 1.0)

    def test_overlap_rejected(self):
        net = converging_net()
        with self.assertRaisesRegex(QueryError, "overlap"):
            run_query(net, ["A"], {"A": "t"})

    def test_order_must_cover_hidden(self):
        net = load_network(str(EXAMPLE))
        with self.assertRaisesRegex(QueryError, "hidden"):
            run_query(net, ["Burglary"], {"JohnCalls": "yes"}, ["Alarm"])

    def test_order_invariance_within_tolerance(self):
        net = load_network(str(EXAMPLE))
        evidence = {"JohnCalls": "yes", "MaryCalls": "yes"}
        hidden = ["Alarm", "Earthquake"]
        reference = run_query(net, ["Burglary"], evidence)
        for perm in itertools.permutations(hidden):
            result = run_query(net, ["Burglary"], evidence, list(perm))
            self.assertEqual(result.elimination_order, list(perm))
            for state, prob in reference.distribution.items():
                self.assertTrue(
                    math.isclose(prob, result.distribution[state],
                                 rel_tol=1e-9, abs_tol=1e-12))

    def test_map_uses_joint_not_marginals(self):
        net = make_net([
            {"name": "A", "states": ["f", "t"], "parents": [],
             "cpt": [{"when": [], "probs": {"f": 0.49, "t": 0.51}}]},
            {"name": "B", "states": ["f", "t"], "parents": ["A"],
             "cpt": [
                 {"when": ["f"], "probs": {"f": 0.99, "t": 0.01}},
                 {"when": ["t"], "probs": {"f": 0.6, "t": 0.4}},
             ]},
        ])
        result = run_query(net, ["A", "B"], {})
        # P(f,f)=0.4851 > P(t,f)=0.306; marginals: A t=0.51, B f=0.7911.
        self.assertEqual(result.map_assignment, {"A": "f", "B": "f"})

    def test_tie_break_lexicographic(self):
        net = make_net([
            {"name": "A", "states": ["b", "a"], "parents": [],
             "cpt": [{"when": [], "probs": {"a": 0.5, "b": 0.5}}]},
        ])
        result = run_query(net, ["A"], {})
        self.assertEqual(result.map_assignment, {"A": "a"})

    def test_batch_queries_are_independent(self):
        net = load_network(str(EXAMPLE))
        with_evidence = run_query(net, ["Burglary"], {"JohnCalls": "yes"})
        without = run_query(net, ["Burglary"], {})
        again = run_query(net, ["Burglary"], {})
        self.assertEqual(without.distribution, again.distribution)
        self.assertNotEqual(with_evidence.distribution, without.distribution)

    def test_summary_reports_order_and_factor_size(self):
        net = load_network(str(EXAMPLE))
        result = run_query(net, ["Burglary"],
                           {"JohnCalls": "yes", "MaryCalls": "yes"})
        self.assertEqual(sorted(result.elimination_order), ["Alarm", "Earthquake"])
        self.assertGreaterEqual(result.max_factor_size, 1)


if __name__ == "__main__":
    unittest.main()
