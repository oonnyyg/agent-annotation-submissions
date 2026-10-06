import math
import unittest

from pipenet.model import ModelError, Network, Pipe, Scenario
from pipenet.solver import SolverOptions, solve


def make_network(fixed_heads, pipes):
    return Network(
        fixed_heads=fixed_heads,
        pipes=tuple(Pipe(**pipe) for pipe in pipes),
    )


class TreeNetworkTest(unittest.TestCase):
    """Single source feeding two demand nodes in series."""

    def setUp(self):
        self.network = make_network(
            {"R": 100.0},
            [
                {"name": "p1", "start": "R", "end": "A", "resistance": 2.0},
                {"name": "p2", "start": "A", "end": "B", "resistance": 8.0},
            ],
        )

    def test_heads_and_flows(self):
        # demands: A=1, B=2 -> q1=3, q2=2
        result = solve(self.network, Scenario("base", {"A": 1.0, "B": 2.0}))
        self.assertTrue(result.converged, result.message)
        self.assertAlmostEqual(result.flows["p1"], 3.0, places=9)
        self.assertAlmostEqual(result.flows["p2"], 2.0, places=9)
        # h_A = 100 - 2*9 = 82 ; h_B = 82 - 8*4 = 50
        self.assertAlmostEqual(result.heads["A"], 82.0, places=7)
        self.assertAlmostEqual(result.heads["B"], 50.0, places=7)
        self.assertLessEqual(result.node_residual, 1e-9)
        self.assertLessEqual(result.pipe_residual, 1e-9)

    def test_zero_demand_gives_zero_flow(self):
        result = solve(self.network, Scenario("idle", {}))
        self.assertTrue(result.converged, result.message)
        self.assertAlmostEqual(result.flows["p1"], 0.0, places=12)
        self.assertAlmostEqual(result.flows["p2"], 0.0, places=12)
        self.assertAlmostEqual(result.heads["A"], 100.0, places=9)


class LoopedNetworkTest(unittest.TestCase):
    """One loop: flow splits between two parallel paths."""

    def setUp(self):
        # R -> A via p1, then A -> B via two parallel pipes p2 (r=1) and
        # p3 (r=4). Equal head drop => q2*|q2| = 4*q3*|q3| => q2 = 2*q3.
        self.network = make_network(
            {"R": 50.0},
            [
                {"name": "p1", "start": "R", "end": "A", "resistance": 0.5},
                {"name": "p2", "start": "A", "end": "B", "resistance": 1.0},
                {"name": "p3", "start": "A", "end": "B", "resistance": 4.0},
            ],
        )

    def test_loop_split(self):
        result = solve(self.network, Scenario("load", {"B": 3.0}))
        self.assertTrue(result.converged, result.message)
        self.assertAlmostEqual(result.flows["p1"], 3.0, places=9)
        self.assertAlmostEqual(result.flows["p2"], 2.0, places=9)
        self.assertAlmostEqual(result.flows["p3"], 1.0, places=9)
        # h_A = 50 - 0.5*9 = 45.5 ; h_B = 45.5 - 1*4 = 41.5
        self.assertAlmostEqual(result.heads["A"], 45.5, places=7)
        self.assertAlmostEqual(result.heads["B"], 41.5, places=7)


class ReverseFlowTest(unittest.TestCase):
    """A stronger downstream source pushes flow backwards through a pipe."""

    def test_negative_flow_is_kept(self):
        # Pipe direction R1 -> R2, but R2 has the higher head.
        network = make_network(
            {"R1": 100.0, "R2": 110.0},
            [{"name": "link", "start": "R1", "end": "R2", "resistance": 10.0}],
        )
        result = solve(network, Scenario("reverse", {}))
        self.assertTrue(result.converged, result.message)
        # 100 - 110 = 10*q*|q| -> q = -1
        self.assertAlmostEqual(result.flows["link"], -1.0, places=9)

    def test_reversed_demand_node(self):
        # Injection at A (negative demand) sends water back toward R.
        network = make_network(
            {"R": 10.0, "S": 20.0},
            [
                {"name": "ra", "start": "R", "end": "A", "resistance": 1.0},
                {"name": "sa", "start": "S", "end": "A", "resistance": 1.0},
            ],
        )
        result = solve(network, Scenario("inject", {"A": -4.0}))
        self.assertTrue(result.converged, result.message)
        q_ra = result.flows["ra"]
        q_sa = result.flows["sa"]
        self.assertAlmostEqual(q_ra + q_sa, -4.0, places=9)
        self.assertLess(q_ra, 0.0)  # flow actually goes A -> R


class FailureHandlingTest(unittest.TestCase):
    def test_iteration_limit_reports_residuals(self):
        network = make_network(
            {"R": 100.0},
            [{"name": "p1", "start": "R", "end": "A", "resistance": 2.0}],
        )
        options = SolverOptions(max_iterations=1, node_tolerance=1e-12, pipe_tolerance=1e-12)
        result = solve(network, Scenario("load", {"A": 1.0}), options)
        self.assertFalse(result.converged)
        self.assertIn("iteration limit", result.message)
        self.assertTrue(math.isfinite(result.node_residual))
        self.assertTrue(math.isfinite(result.pipe_residual))
        self.assertEqual(result.heads, {})  # no fake solution exposed
        self.assertEqual(result.flows, {})

    def test_both_tolerances_must_be_met(self):
        # With an unreachable pipe tolerance, convergence must fail even
        # though the node residual is tiny.
        network = make_network(
            {"R": 100.0},
            [{"name": "p1", "start": "R", "end": "A", "resistance": 2.0}],
        )
        options = SolverOptions(max_iterations=100, node_tolerance=1e9, pipe_tolerance=0.0)
        result = solve(network, Scenario("load", {"A": 1.0}), options)
        if result.pipe_residual > 0.0:
            self.assertFalse(result.converged)
        # With an unreachable node tolerance, same story.
        options = SolverOptions(max_iterations=100, node_tolerance=0.0, pipe_tolerance=1e9)
        result = solve(network, Scenario("load", {"A": 1.0}), options)
        if result.node_residual > 0.0:
            self.assertFalse(result.converged)


class ValidationTest(unittest.TestCase):
    def _write_json(self, data):
        import json
        import tempfile

        handle = tempfile.NamedTemporaryFile(
            "w", suffix=".json", delete=False, encoding="utf-8"
        )
        with handle:
            json.dump(data, handle)
        return handle.name

    def test_self_loop_rejected(self):
        from pipenet.model import load_network

        path = self._write_json({
            "fixed_heads": {"R": 10.0},
            "pipes": [{"name": "bad", "from": "R", "to": "R", "resistance": 1.0}],
        })
        with self.assertRaises(ModelError):
            load_network(path)

    def test_unanchored_component_rejected(self):
        from pipenet.model import load_network

        path = self._write_json({
            "fixed_heads": {"R": 10.0},
            "pipes": [
                {"name": "ok", "from": "R", "to": "A", "resistance": 1.0},
                {"name": "lost", "from": "X", "to": "Y", "resistance": 1.0},
            ],
        })
        with self.assertRaises(ModelError):
            load_network(path)


if __name__ == "__main__":
    unittest.main()
