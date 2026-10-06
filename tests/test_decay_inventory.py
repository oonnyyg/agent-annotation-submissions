import json
import math
import os
import subprocess
import sys
import tempfile
import unittest
 
from decay_inventory.model import ModelError, load_network
from decay_inventory.solver import SolverError, solve
 
LN2 = math.log(2.0)
 
 
def make_net(nuclides, edges, feeds=None):
    return load_network({
        "nuclides": nuclides,
        "edges": edges,
        "feeds": feeds or [],
    })
 
 
class EqualRateChainTest(unittest.TestCase):
    """A -> B -> C(stable) with identical half-lives (repeated eigenvalue)."""
 
    def setUp(self):
        self.net = make_net(
            [{"name": "A", "half_life": 1.0, "initial": 1.0},
             {"name": "B", "half_life": 1.0, "initial": 0.0},
             {"name": "C", "half_life": None, "initial": 0.0}],
            [{"parent": "A", "child": "B", "probability": 1.0},
             {"parent": "B", "child": "C", "probability": 1.0}])
 
    def test_quantities_match_bateman_with_equal_rates(self):
        times = [0.0, 0.5, 1.0, 2.5, 7.0]
        sol = solve(self.net, times, [(0.0, 7.0)])
        for k, t in enumerate(times):
            x = LN2 * t
            a = math.exp(-x)
            b = x * math.exp(-x)
            c = 1.0 - (1.0 + x) * math.exp(-x)
            for got, want in zip(sol.quantities[k], (a, b, c)):
                self.assertAlmostEqual(got, want, places=12)
 
    def test_cumulative_decays_match_integral(self):
        times = [0.0, 3.0]
        sol = solve(self.net, times, [(0.0, 3.0)])
        window = sol.windows[0]
        t = 3.0
        x = LN2 * t
        decays_a = 1.0 - math.exp(-x)
        decays_b = 1.0 - (1.0 + x) * math.exp(-x)
        self.assertAlmostEqual(window.decays[0], decays_a, places=12)
        self.assertAlmostEqual(window.decays[1], decays_b, places=12)
        self.assertAlmostEqual(window.decays[2], 0.0, places=15)
        self.assertAlmostEqual(window.branch_yields[0][0], decays_a, places=12)
        self.assertAlmostEqual(window.branch_yields[1][0], decays_b, places=12)
 
    def test_conservation_residual_is_zero(self):
        sol = solve(self.net, [0.0, 1.0, 5.0], [(0.0, 5.0)])
        for r in sol.conservation_residual:
            self.assertAlmostEqual(r, 0.0, places=12)
 
 
class BranchConvergenceTest(unittest.TestCase):
    """A branches to B and C; both decay into stable D (converging DAG)."""
 
    def setUp(self):
        self.net = make_net(
            [{"name": "A", "half_life": 1.0, "initial": 2.0},
             {"name": "B", "half_life": 2.0, "initial": 0.0},
             {"name": "C", "half_life": 2.0, "initial": 0.0},
             {"name": "D", "half_life": None, "initial": 0.5}],
            [{"parent": "A", "child": "B", "probability": 0.25},
             {"parent": "A", "child": "C", "probability": 0.75},
             {"parent": "B", "child": "D", "probability": 1.0},
             {"parent": "C", "child": "D", "probability": 1.0}])
 
    def test_branch_yields_and_totals(self):
        t = 4.0
        sol = solve(self.net, [0.0, t], [(0.0, t)])
        window = sol.windows[0]
        decays_a = 2.0 * (1.0 - math.exp(-LN2 * t / 1.0))
        self.assertAlmostEqual(window.decays[0], decays_a, places=12)
        self.assertAlmostEqual(window.branch_yields[0][0], 0.25 * decays_a,
                               places=12)
        self.assertAlmostEqual(window.branch_yields[0][1], 0.75 * decays_a,
                               places=12)
        # D only grows by decays of B and C (decays of A stay in B/C first).
        gained_d = sol.quantities[1][3] - 0.5
        self.assertAlmostEqual(gained_d, window.decays[1] + window.decays[2],
                               places=10)
        for r in sol.conservation_residual:
            self.assertAlmostEqual(r, 0.0, places=11)
 
    def test_time_grid_density_does_not_change_results(self):
        coarse = solve(self.net, [0.0, 4.0], [(0.0, 4.0)])
        fine = solve(self.net, [i * 0.1 for i in range(41)], [(0.0, 4.0)])
        for q_c, q_f in zip(coarse.quantities[1], fine.quantities[-1]):
            self.assertAlmostEqual(q_c, q_f, places=12)
        for d_c, d_f in zip(coarse.windows[0].decays, fine.windows[0].decays):
            self.assertAlmostEqual(d_c, d_f, places=12)
 
 
class FeedTest(unittest.TestCase):
    def setUp(self):
        self.net = make_net(
            [{"name": "A", "half_life": 1.0, "initial": 0.0},
             {"name": "S", "half_life": None, "initial": 0.0}],
            [{"parent": "A", "child": "S", "probability": 1.0}],
            feeds=[{"nuclide": "A", "rate": 2.0, "start": 1.0, "end": 3.0},
                   {"nuclide": "S", "rate": 0.5, "start": 0.0, "end": 0.5}])
 
    def test_feed_intervals_half_open(self):
        times = [0.0, 0.5, 1.0, 2.0, 3.0, 4.0]
        sol = solve(self.net, times, [(0.0, 4.0)])
        # Stable S: rate 0.5 on [0, 0.5) only, plus decay of A afterwards.
        self.assertAlmostEqual(sol.quantities[0][1], 0.0, places=15)
        self.assertAlmostEqual(sol.quantities[1][1], 0.25, places=12)
        # A receives 2.0 per unit time on [1, 3): A(t) = (2/lam)(1-e^{-lam(t-1)})
        a_at = lambda t: (2.0 / LN2) * (1.0 - math.exp(-LN2 * (t - 1.0)))
        self.assertAlmostEqual(sol.quantities[2][0], 0.0, places=15)
        self.assertAlmostEqual(sol.quantities[3][0], a_at(2.0), places=12)
        self.assertAlmostEqual(sol.quantities[4][0], a_at(3.0), places=12)
        self.assertAlmostEqual(sol.quantities[5][0],
                               a_at(3.0) * math.exp(-LN2), places=12)
        # Cumulative feed = 0.5*0.5 + 2.0*2.0 = 4.25
        self.assertAlmostEqual(sol.cumulative_feed[-1], 4.25, places=12)
        for r in sol.conservation_residual:
            self.assertAlmostEqual(r, 0.0, places=10)
 
    def test_overlapping_feeds_rejected(self):
        with self.assertRaises(ModelError):
            make_net(
                [{"name": "A", "half_life": 1.0, "initial": 0.0},
                 {"name": "S", "half_life": None, "initial": 0.0}],
                [{"parent": "A", "child": "S", "probability": 1.0}],
                feeds=[{"nuclide": "A", "rate": 1.0, "start": 0.0, "end": 2.0},
                       {"nuclide": "A", "rate": 1.0, "start": 1.0, "end": 3.0}])
 
 
class ValidationTest(unittest.TestCase):
    def test_branch_probabilities_must_sum_to_one(self):
        with self.assertRaises(ModelError):
            make_net(
                [{"name": "A", "half_life": 1.0, "initial": 1.0},
                 {"name": "B", "half_life": None, "initial": 0.0},
                 {"name": "C", "half_life": None, "initial": 0.0}],
                [{"parent": "A", "child": "B", "probability": 0.3},
                 {"parent": "A", "child": "C", "probability": 0.3}])
 
    def test_cycle_rejected(self):
        with self.assertRaises(ModelError):
            make_net(
                [{"name": "A", "half_life": 1.0, "initial": 1.0},
                 {"name": "B", "half_life": 1.0, "initial": 0.0}],
                [{"parent": "A", "child": "B", "probability": 1.0},
                 {"parent": "B", "child": "A", "probability": 1.0}])
 
    def test_stable_node_with_edge_rejected(self):
        with self.assertRaises(ModelError):
            make_net(
                [{"name": "A", "half_life": None, "initial": 1.0},
                 {"name": "B", "half_life": None, "initial": 0.0}],
                [{"parent": "A", "child": "B", "probability": 1.0}])
 
    def test_invalid_half_life_and_initial_rejected(self):
        with self.assertRaises(ModelError):
            make_net([{"name": "A", "half_life": -1.0, "initial": 0.0}], [])
        with self.assertRaises(ModelError):
            make_net([{"name": "A", "half_life": None, "initial": -2.0}], [])
 
    def test_too_many_nuclides_rejected(self):
        nuclides = [{"name": f"N{i}", "half_life": None, "initial": 0.0}
                    for i in range(13)]
        with self.assertRaises(ModelError):
            make_net(nuclides, [])
 
 
class CliTest(unittest.TestCase):
    def test_end_to_end(self):
        doc = {
            "nuclides": [
                {"name": "A", "half_life": 1.0, "initial": 1.0},
                {"name": "B", "half_life": None, "initial": 0.0}],
            "edges": [{"parent": "A", "child": "B", "probability": 1.0}],
            "schedule": {"start": 0.0, "stop": 2.0, "step": 1.0},
            "windows": [{"start": 0.0, "end": 2.0}],
        }
        with tempfile.TemporaryDirectory() as tmp:
            src = os.path.join(tmp, "in.json")
            csv_path = os.path.join(tmp, "out.csv")
            json_path = os.path.join(tmp, "out.json")
            with open(src, "w", encoding="utf-8") as handle:
                json.dump(doc, handle)
            proc = subprocess.run(
                [sys.executable, "-m", "decay_inventory", src,
                 "--csv", csv_path, "--json", json_path],
                capture_output=True, text=True)
            self.assertEqual(proc.returncode, 0, proc.stderr)
            with open(csv_path, encoding="utf-8") as handle:
                lines = handle.read().strip().splitlines()
            self.assertEqual(len(lines), 4)  # header + 3 rows
            with open(json_path, encoding="utf-8") as handle:
                report = json.load(handle)
            decays_a = report["windows"][0]["decays"]["A"]
            self.assertAlmostEqual(decays_a, 1.0 - 0.25, places=12)
 
    def test_invalid_input_fails_without_reports(self):
        doc = {"nuclides": [{"name": "A", "half_life": -1.0}], "edges": []}
        with tempfile.TemporaryDirectory() as tmp:
            src = os.path.join(tmp, "in.json")
            csv_path = os.path.join(tmp, "out.csv")
            json_path = os.path.join(tmp, "out.json")
            with open(src, "w", encoding="utf-8") as handle:
                json.dump(doc, handle)
            proc = subprocess.run(
                [sys.executable, "-m", "decay_inventory", src,
                 "--csv", csv_path, "--json", json_path],
                capture_output=True, text=True)
            self.assertEqual(proc.returncode, 1)
            self.assertIn("error", proc.stderr)
            self.assertFalse(os.path.exists(csv_path))
            self.assertFalse(os.path.exists(json_path))
 
 
if __name__ == "__main__":
    unittest.main()
