import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from bayesnet.inference import QueryError, query
from bayesnet.model import ModelError, parse_network


def converging_network():
    # A and B are roots, C converges on both (explaining-away structure).
    return parse_network({
        "variables": {"A": ["a0", "a1"], "B": ["b0", "b1"], "C": ["c0", "c1"]},
        "cpt": {
            "A": {"parents": [], "rows": [
                {"parent_states": {}, "probs": {"a0": 0.6, "a1": 0.4}},
            ]},
            "B": {"parents": [], "rows": [
                {"parent_states": {}, "probs": {"b0": 0.3, "b1": 0.7}},
            ]},
            "C": {"parents": ["A", "B"], "rows": [
                {"parent_states": {"A": "a0", "B": "b0"}, "probs": {"c0": 0.9, "c1": 0.1}},
                {"parent_states": {"A": "a0", "B": "b1"}, "probs": {"c0": 0.2, "c1": 0.8}},
                {"parent_states": {"A": "a1", "B": "b0"}, "probs": {"c0": 0.7, "c1": 0.3}},
                {"parent_states": {"A": "a1", "B": "b1"}, "probs": {"c0": 0.0, "c1": 1.0}},
            ]},
        },
    })


def deterministic_network():
    return parse_network({
        "variables": {"A": ["a0", "a1"], "B": ["b0", "b1"]},
        "cpt": {
            "A": {"parents": [], "rows": [
                {"parent_states": {}, "probs": {"a0": 1.0, "a1": 0.0}},
            ]},
            "B": {"parents": ["A"], "rows": [
                {"parent_states": {"A": "a0"}, "probs": {"b0": 1.0, "b1": 0.0}},
                {"parent_states": {"A": "a1"}, "probs": {"b0": 0.0, "b1": 1.0}},
            ]},
        },
    })


class ModelValidationTests(unittest.TestCase):
    def test_duplicate_states_rejected(self):
        with self.assertRaises(ModelError):
            parse_network({
                "variables": {"A": ["x", "x"]},
                "cpt": {"A": {"parents": [], "rows": [
                    {"parent_states": {}, "probs": {"x": 1.0}},
                ]}},
            })

    def test_missing_parent_combination_rejected(self):
        with self.assertRaises(ModelError):
            parse_network({
                "variables": {"A": ["a0", "a1"], "B": ["b0", "b1"]},
                "cpt": {
                    "A": {"parents": [], "rows": [
                        {"parent_states": {}, "probs": {"a0": 0.5, "a1": 0.5}},
                    ]},
                    "B": {"parents": ["A"], "rows": [
                        {"parent_states": {"A": "a0"}, "probs": {"b0": 1.0, "b1": 0.0}},
                    ]},
                },
            })

    def test_bad_probability_sum_rejected(self):
        with self.assertRaises(ModelError):
            parse_network({
                "variables": {"A": ["a0", "a1"]},
                "cpt": {"A": {"parents": [], "rows": [
                    {"parent_states": {}, "probs": {"a0": 0.5, "a1": 0.6}},
                ]}},
            })

    def test_negative_probability_rejected(self):
        with self.assertRaises(ModelError):
            parse_network({
                "variables": {"A": ["a0", "a1"]},
                "cpt": {"A": {"parents": [], "rows": [
                    {"parent_states": {}, "probs": {"a0": -0.1, "a1": 1.1}},
                ]}},
            })

    def test_cycle_rejected(self):
        with self.assertRaises(ModelError):
            parse_network({
                "variables": {"A": ["a0", "a1"], "B": ["b0", "b1"]},
                "cpt": {
                    "A": {"parents": ["B"], "rows": [
                        {"parent_states": {"B": "b0"}, "probs": {"a0": 0.5, "a1": 0.5}},
                        {"parent_states": {"B": "b1"}, "probs": {"a0": 0.5, "a1": 0.5}},
                    ]},
                    "B": {"parents": ["A"], "rows": [
                        {"parent_states": {"A": "a0"}, "probs": {"b0": 0.5, "b1": 0.5}},
                        {"parent_states": {"A": "a1"}, "probs": {"b0": 0.5, "b1": 0.5}},
                    ]},
                },
            })


class InferenceTests(unittest.TestCase):
    def test_posterior_with_converging_evidence(self):
        net = converging_network()
        result = query(net, ["A"], {"C": "c1"})
        pa_c1 = {"a0": 0.6 * (0.3 * 0.1 + 0.7 * 0.8), "a1": 0.4 * (0.3 * 0.3 + 0.7 * 1.0)}
        total = sum(pa_c1.values())
        self.assertAlmostEqual(result.evidence_probability, total)
        self.assertAlmostEqual(result.distribution[("a0",)], pa_c1["a0"] / total)
        self.assertAlmostEqual(result.distribution[("a1",)], pa_c1["a1"] / total)
        self.assertAlmostEqual(sum(result.distribution.values()), 1.0)

    def test_empty_evidence_marginal(self):
        net = converging_network()
        result = query(net, ["C"], {})
        p_c0 = 0.6 * 0.3 * 0.9 + 0.6 * 0.7 * 0.2 + 0.4 * 0.3 * 0.7 + 0.4 * 0.7 * 0.0
        self.assertAlmostEqual(result.evidence_probability, 1.0)
        self.assertAlmostEqual(result.distribution[("c0",)], p_c0)

    def test_impossible_evidence_raises(self):
        net = deterministic_network()
        with self.assertRaises(QueryError):
            query(net, ["A"], {"B": "b1"})

    def test_target_evidence_overlap_rejected(self):
        net = converging_network()
        with self.assertRaises(QueryError):
            query(net, ["A"], {"A": "a0"})

    def test_order_must_cover_hidden(self):
        net = converging_network()
        with self.assertRaises(QueryError):
            query(net, ["A"], {}, order=["B"])

    def test_orders_agree_within_tolerance(self):
        net = converging_network()
        auto = query(net, ["A"], {"C": "c1"})
        manual = query(net, ["A"], {"C": "c1"}, order=["B"])
        for key in auto.distribution:
            self.assertAlmostEqual(auto.distribution[key], manual.distribution[key], places=12)
        self.assertEqual(manual.elimination_order, ["B"])

    def test_map_assignment_joint(self):
        net = converging_network()
        result = query(net, ["A", "B"], {"C": "c1"})
        joint = {
            ("a0", "b0"): 0.6 * 0.3 * 0.1,
            ("a0", "b1"): 0.6 * 0.7 * 0.8,
            ("a1", "b0"): 0.4 * 0.3 * 0.3,
            ("a1", "b1"): 0.4 * 0.7 * 1.0,
        }
        best = max(joint, key=joint.get)
        self.assertEqual(result.map_assignment, {"A": best[0], "B": best[1]})

    def test_map_not_per_variable(self):
        net = parse_network({
            "variables": {"X": ["x0", "x1"], "Y": ["y0", "y1"]},
            "cpt": {
                "X": {"parents": [], "rows": [
                    {"parent_states": {}, "probs": {"x0": 0.5, "x1": 0.5}},
                ]},
                "Y": {"parents": ["X"], "rows": [
                    {"parent_states": {"X": "x0"}, "probs": {"y0": 0.51, "y1": 0.49}},
                    {"parent_states": {"X": "x1"}, "probs": {"y0": 0.49, "y1": 0.51}},
                ]},
            },
        })
        result = query(net, ["X", "Y"], {})
        self.assertEqual(result.map_assignment, {"X": "x0", "Y": "y0"})

    def test_no_state_leak_between_queries(self):
        net = converging_network()
        first = query(net, ["A"], {"C": "c1"})
        second = query(net, ["A"], {})
        self.assertAlmostEqual(second.distribution[("a0",)], 0.6)
        self.assertNotEqual(first.distribution, second.distribution)


class CliTests(unittest.TestCase):
    def test_cli_batch(self):
        net = converging_network()
        with tempfile.TemporaryDirectory() as tmp:
            net_path = Path(tmp) / "net.json"
            q_path = Path(tmp) / "q.jsonl"
            net_path.write_text(json.dumps({
                "variables": {v: net.states[v] for v in net.states},
                "cpt": {
                    v: {
                        "parents": net.cpts[v].parents,
                        "rows": [
                            {"parent_states": dict(zip(net.cpts[v].parents, key)), "probs": row}
                            for key, row in net.cpts[v].rows.items()
                        ],
                    }
                    for v in net.cpts
                },
            }), encoding="utf-8")
            q_path.write_text(
                '{"target": "A", "evidence": {"C": "c1"}}\n'
                '{"target": ["A", "B"], "evidence": {"C": "c1"}}\n'
                '{"target": "A", "evidence": {"A": "a0"}}\n',
                encoding="utf-8",
            )
            proc = subprocess.run(
                [sys.executable, "-m", "bayesnet", str(net_path), str(q_path)],
                capture_output=True, text=True, cwd=Path(__file__).resolve().parent.parent,
            )
            lines = [json.loads(l) for l in proc.stdout.strip().splitlines()]
            self.assertEqual(len(lines), 3)
            self.assertIn("distribution", lines[0])
            self.assertIn("distribution", lines[1])
            self.assertIn("error", lines[2])
            self.assertEqual(proc.returncode, 1)


if __name__ == "__main__":
    unittest.main()
