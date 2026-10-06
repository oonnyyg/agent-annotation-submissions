import json
import math
import os
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from competitive_binding import (
    SiteClass,
    SolverError,
    solve_equilibrium,
    solve_inverse,
)


def make_sites():
    return [
        SiteClass(name="tight", total=10.0, kd_a=1.0, kd_b=5.0),
        SiteClass(name="loose", total=50.0, kd_a=100.0, kd_b=2.0),
    ]


class EquilibriumTests(unittest.TestCase):
    def test_mass_balance_and_bounds(self):
        sites = make_sites()
        res = solve_equilibrium(sites, 30.0, 40.0)
        self.assertAlmostEqual(res.residual_a, 0.0, places=8)
        self.assertAlmostEqual(res.residual_b, 0.0, places=8)
        self.assertGreaterEqual(res.free_a, 0.0)
        self.assertLessEqual(res.free_a, 30.0)
        self.assertGreaterEqual(res.free_b, 0.0)
        self.assertLessEqual(res.free_b, 40.0)
        bound_a = sum(s.bound_a for s in res.sites)
        bound_b = sum(s.bound_b for s in res.sites)
        self.assertAlmostEqual(res.free_a + bound_a, 30.0, places=8)
        self.assertAlmostEqual(res.free_b + bound_b, 40.0, places=8)

    def test_competition_reduces_occupancy(self):
        sites = make_sites()
        alone = solve_equilibrium(sites, 30.0, 0.0)
        competed = solve_equilibrium(sites, 30.0, 200.0)
        for solo, comp in zip(alone.sites, competed.sites):
            self.assertLess(comp.occupancy_a, solo.occupancy_a)
            self.assertGreater(comp.occupancy_b, 0.0)

    def test_free_not_assumed_equal_total(self):
        sites = make_sites()
        res = solve_equilibrium(sites, 5.0, 0.0)
        self.assertLess(res.free_a, 5.0)
        self.assertGreater(res.free_a, 0.0)

    def test_zero_totals(self):
        sites = make_sites()
        res = solve_equilibrium(sites, 0.0, 0.0)
        self.assertEqual(res.free_a, 0.0)
        self.assertEqual(res.free_b, 0.0)
        for s in res.sites:
            self.assertEqual(s.occupancy_a, 0.0)
            self.assertEqual(s.occupancy_b, 0.0)
            self.assertAlmostEqual(s.empty, s.total)

    def test_zero_b_only(self):
        sites = make_sites()
        res = solve_equilibrium(sites, 20.0, 0.0)
        self.assertEqual(res.free_b, 0.0)
        self.assertAlmostEqual(res.residual_a, 0.0, places=8)

    def test_states_sum_to_total(self):
        sites = make_sites()
        res = solve_equilibrium(sites, 100.0, 100.0)
        for s in res.sites:
            self.assertAlmostEqual(s.empty + s.bound_a + s.bound_b, s.total)

    def test_joint_not_independent(self):
        sites = make_sites()
        joint = solve_equilibrium(sites, 30.0, 40.0)
        naive_bound_a = 0.0
        for s in sites:
            solo = solve_equilibrium([s], 30.0, 40.0)
            naive_bound_a += solo.sites[0].bound_a
        joint_bound_a = sum(s.bound_a for s in joint.sites)
        self.assertNotAlmostEqual(joint_bound_a, naive_bound_a, places=4)

    def test_invalid_inputs(self):
        with self.assertRaises(ValueError):
            SiteClass(name="bad", total=0.0, kd_a=1.0, kd_b=1.0)
        with self.assertRaises(ValueError):
            SiteClass(name="bad", total=1.0, kd_a=-1.0, kd_b=1.0)
        with self.assertRaises(ValueError):
            SiteClass(name="bad", total=1.0, kd_a=1.0, kd_b=math.inf)
        with self.assertRaises(ValueError):
            solve_equilibrium(make_sites(), -1.0, 0.0)
        with self.assertRaises(ValueError):
            solve_equilibrium(make_sites(), math.nan, 0.0)

    def test_nonconvergence_reports_residual(self):
        sites = make_sites()
        with self.assertRaises(SolverError) as ctx:
            solve_equilibrium(sites, 30.0, 40.0, max_iter=1)
        self.assertIsNotNone(ctx.exception.residual_b)


class InverseTests(unittest.TestCase):
    def test_inverse_reaches_target(self):
        sites = make_sites()
        out = solve_inverse(sites, "tight", 0.5, fixed_total_b=10.0,
                            limit_a=1000.0)
        self.assertTrue(out.success)
        self.assertIsNotNone(out.total_a)
        self.assertGreaterEqual(out.achieved_occupancy, 0.5 - 1e-6)
        self.assertLessEqual(out.total_a, 1000.0)
        check = solve_equilibrium(sites, out.total_a * 0.95, 10.0)
        self.assertLess(check.sites[0].occupancy_a, 0.5)

    def test_inverse_zero_target(self):
        sites = make_sites()
        out = solve_inverse(sites, "tight", 0.0, fixed_total_b=10.0,
                            limit_a=1000.0)
        self.assertTrue(out.success)
        self.assertEqual(out.total_a, 0.0)

    def test_inverse_unreachable_limit(self):
        sites = make_sites()
        out = solve_inverse(sites, "tight", 0.9, fixed_total_b=500.0,
                            limit_a=5.0)
        self.assertFalse(out.success)
        self.assertIsNone(out.total_a)
        self.assertLess(out.achieved_occupancy, 0.9)
        self.assertIn("not reachable", out.message)

    def test_inverse_invalid_target(self):
        sites = make_sites()
        with self.assertRaises(ValueError):
            solve_inverse(sites, "tight", 1.0, 0.0, 100.0)
        with self.assertRaises(ValueError):
            solve_inverse(sites, "nope", 0.5, 0.0, 100.0)


class CliTests(unittest.TestCase):
    def _write_inputs(self, tmp):
        sites_path = os.path.join(tmp, "sites.json")
        with open(sites_path, "w", encoding="utf-8") as fh:
            json.dump({"site_classes": [
                {"name": "tight", "total": 10, "kd_a": 1, "kd_b": 5},
                {"name": "loose", "total": 50, "kd_a": 100, "kd_b": 2},
            ]}, fh)
        csv_path = os.path.join(tmp, "mixtures.csv")
        with open(csv_path, "w", encoding="utf-8") as fh:
            fh.write("total_a,total_b\n30,40\n0,0\n")
        return sites_path, csv_path

    def test_cli_solve(self):
        with tempfile.TemporaryDirectory() as tmp:
            sites_path, csv_path = self._write_inputs(tmp)
            out_csv = os.path.join(tmp, "results.csv")
            report = os.path.join(tmp, "report.json")
            root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
            proc = subprocess.run(
                [sys.executable, "-m", "competitive_binding", "solve",
                 sites_path, csv_path, "--csv-out", out_csv,
                 "--report", report],
                capture_output=True, text=True, cwd=root)
            self.assertEqual(proc.returncode, 0, proc.stderr)
            with open(report, encoding="utf-8") as fh:
                data = json.load(fh)
            self.assertEqual(len(data["mixtures"]), 2)
            first = data["mixtures"][0]
            self.assertAlmostEqual(
                first["free_a_nM"] + first["bound_a_nM"], 30.0, places=6)
            with open(out_csv, encoding="utf-8") as fh:
                lines = fh.read().strip().splitlines()
            self.assertEqual(len(lines), 1 + 2 * 2)

    def test_cli_inverse_success_and_failure(self):
        with tempfile.TemporaryDirectory() as tmp:
            sites_path, _ = self._write_inputs(tmp)
            base = [sys.executable, "-m", "competitive_binding", "inverse",
                    sites_path, "--class", "tight"]
            root = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
            ok = subprocess.run(
                base + ["--target", "0.5", "--total-b", "10",
                        "--limit-a", "1000"],
                capture_output=True, text=True, cwd=root)
            self.assertEqual(ok.returncode, 0, ok.stderr)
            self.assertIn('"success": true', ok.stdout)
            bad = subprocess.run(
                base + ["--target", "0.9", "--total-b", "500",
                        "--limit-a", "5"],
                capture_output=True, text=True, cwd=root)
            self.assertEqual(bad.returncode, 1)
            self.assertIn('"success": false', bad.stdout)
            self.assertIn('"total_a_nM": null', bad.stdout)


if __name__ == "__main__":
    unittest.main()
