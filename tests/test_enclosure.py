import json
import os
import subprocess
import sys
import tempfile
import unittest

from enclosure_radiation.model import InputError, load_enclosure
from enclosure_radiation.solver import (
    STEFAN_BOLTZMANN,
    UnderconstrainedError,
    solve_case,
)


def make_config(temperatures, emissivities=None, areas=None, view_factors=None):
    n = len(temperatures)
    emissivities = emissivities or [0.8] * n
    areas = areas or [1.0] * n
    surfaces = [
        {"name": f"s{i}", "area": areas[i], "emissivity": emissivities[i]}
        for i in range(n)
    ]
    if view_factors is None:
        view_factors = [
            [0.0 if i == j else 1.0 / (n - 1) for j in range(n)]
            for i in range(n)
        ]
    return {
        "surfaces": surfaces,
        "view_factors": view_factors,
        "cases": [{"name": "c1", "temperatures": {
            f"s{i}": temperatures[i] for i in range(n)}}],
    }


def solve(config):
    enclosure = load_enclosure(config)
    return solve_case(enclosure, enclosure.cases[0])


class IsothermalLimitTest(unittest.TestCase):
    def test_zero_net_flux_when_isothermal(self):
        result = solve(make_config([500.0, 500.0, 500.0]))
        for surf in result.surfaces:
            self.assertAlmostEqual(surf.net_heat_flux, 0.0, places=6)
            self.assertAlmostEqual(
                surf.radiosity, STEFAN_BOLTZMANN * 500.0 ** 4, places=4)
        self.assertAlmostEqual(result.global_residual, 0.0, places=6)

    def test_adiabatic_surface_settles_at_common_temperature(self):
        result = solve(make_config([500.0, 500.0, None]))
        adiabatic = result.surfaces[2]
        self.assertTrue(adiabatic.adiabatic)
        self.assertAlmostEqual(adiabatic.temperature, 500.0, places=6)
        self.assertAlmostEqual(result.global_residual, 0.0, places=6)


class AdiabaticCaseTest(unittest.TestCase):
    def test_two_plates_with_reradiating_wall(self):
        # Two fixed plates plus an adiabatic wall; equal areas.
        vf = [
            [0.0, 0.5, 0.5],
            [0.5, 0.0, 0.5],
            [0.5, 0.5, 0.0],
        ]
        result = solve(make_config([1000.0, 300.0, None],
                                   emissivities=[0.9, 0.4, 0.7],
                                   view_factors=vf))
        hot, cold, wall = result.surfaces
        self.assertGreater(hot.net_power, 0.0)
        self.assertLess(cold.net_power, 0.0)
        self.assertEqual(wall.net_power, 0.0)
        self.assertAlmostEqual(wall.radiosity, wall.irradiation, places=6)
        self.assertTrue(300.0 < wall.temperature < 1000.0)
        self.assertAlmostEqual(result.global_residual, 0.0, places=4)
        # Energy leaving the hot plate reaches the cold plate.
        self.assertAlmostEqual(hot.net_power, -cold.net_power, places=4)
        # Pair exchange is antisymmetric.
        for i in range(3):
            for j in range(3):
                self.assertAlmostEqual(
                    result.pair_powers[i][j], -result.pair_powers[j][i],
                    places=6)

    def test_concave_self_view_factor_allowed(self):
        vf = [
            [0.4, 0.6],
            [0.6, 0.4],
        ]
        result = solve(make_config([800.0, None], view_factors=vf))
        self.assertAlmostEqual(result.surfaces[1].temperature, 800.0, places=4)
        self.assertAlmostEqual(result.global_residual, 0.0, places=6)


class ValidationTest(unittest.TestCase):
    def test_row_sum_violation_rejected(self):
        config = make_config([300.0, 300.0],
                             view_factors=[[0.0, 0.9], [0.9, 0.0]])
        with self.assertRaisesRegex(InputError, "sums"):
            load_enclosure(config)

    def test_reciprocity_violation_rejected(self):
        config = make_config([300.0, 300.0], areas=[1.0, 2.0],
                             view_factors=[[0.0, 1.0], [1.0, 0.0]])
        with self.assertRaisesRegex(InputError, "reciprocity"):
            load_enclosure(config)

    def test_negative_view_factor_rejected(self):
        config = make_config([300.0, 300.0],
                             view_factors=[[-0.1, 1.1], [1.0, 0.0]])
        with self.assertRaisesRegex(InputError, "non-negative"):
            load_enclosure(config)

    def test_invalid_emissivity_rejected(self):
        for bad in (0.0, 1.2, -0.5):
            config = make_config([300.0, 300.0], emissivities=[bad, 0.5])
            with self.assertRaises(InputError):
                load_enclosure(config)

    def test_invalid_area_rejected(self):
        config = make_config([300.0, 300.0], areas=[0.0, 1.0])
        with self.assertRaisesRegex(InputError, "area"):
            load_enclosure(config)

    def test_underconstrained_component_rejected(self):
        config = make_config([None, None])
        enclosure = load_enclosure(config)
        with self.assertRaises(UnderconstrainedError):
            solve_case(enclosure, enclosure.cases[0])

    def test_disconnected_components_each_need_anchor(self):
        # Two independent pairs; the second pair has no fixed temperature.
        vf = [
            [0.0, 1.0, 0.0, 0.0],
            [1.0, 0.0, 0.0, 0.0],
            [0.0, 0.0, 0.0, 1.0],
            [0.0, 0.0, 1.0, 0.0],
        ]
        config = make_config([400.0, None, None, None], view_factors=vf)
        enclosure = load_enclosure(config)
        with self.assertRaisesRegex(UnderconstrainedError, "under-constrained"):
            solve_case(enclosure, enclosure.cases[0])

        config["cases"][0]["temperatures"]["s2"] = 350.0
        enclosure = load_enclosure(config)
        result = solve_case(enclosure, enclosure.cases[0])
        self.assertAlmostEqual(result.surfaces[1].temperature, 400.0, places=4)
        self.assertAlmostEqual(result.surfaces[3].temperature, 350.0, places=4)


class CliTest(unittest.TestCase):
    def run_cli(self, config):
        tmp = tempfile.mkdtemp()
        config_path = os.path.join(tmp, "config.json")
        csv_path = os.path.join(tmp, "out.csv")
        json_path = os.path.join(tmp, "out.json")
        with open(config_path, "w", encoding="utf-8") as handle:
            json.dump(config, handle)
        proc = subprocess.run(
            [sys.executable, "-m", "enclosure_radiation", config_path,
             "--csv", csv_path, "--json", json_path],
            capture_output=True, text=True)
        return proc, csv_path, json_path

    def test_multiple_cases_share_geometry(self):
        config = make_config([1000.0, 300.0, None])
        config["cases"].append({
            "name": "c2",
            "temperatures": {"s0": 800.0, "s1": 400.0, "s2": None},
        })
        proc, csv_path, json_path = self.run_cli(config)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        with open(csv_path, encoding="utf-8") as handle:
            lines = handle.read().strip().splitlines()
        self.assertEqual(len(lines), 1 + 2 * 3)
        with open(json_path, encoding="utf-8") as handle:
            details = json.load(handle)
        self.assertEqual([c["name"] for c in details["cases"]], ["c1", "c2"])
        for case in details["cases"]:
            self.assertEqual(case["status"], "solved")
            self.assertAlmostEqual(case["global_energy_residual_W"], 0.0,
                                   places=4)
            for pair in case["pair_exchange_W"]:
                self.assertAlmostEqual(pair["antisymmetry_residual_W"], 0.0,
                                       places=6)

    def test_failed_case_not_silent(self):
        config = make_config([1000.0, 300.0, None])
        config["cases"].append({
            "name": "broken",
            "temperatures": {"s0": None, "s1": None, "s2": None},
        })
        proc, csv_path, json_path = self.run_cli(config)
        self.assertEqual(proc.returncode, 1)
        self.assertIn("under-constrained", proc.stderr)
        with open(json_path, encoding="utf-8") as handle:
            details = json.load(handle)
        broken = [c for c in details["cases"] if c["name"] == "broken"][0]
        self.assertEqual(broken["status"], "failed")
        self.assertIn("error", broken)

    def test_invalid_config_exit_code(self):
        config = make_config([300.0, 300.0],
                             view_factors=[[0.0, 0.5], [0.5, 0.0]])
        proc, _, _ = self.run_cli(config)
        self.assertEqual(proc.returncode, 2)
        self.assertIn("sums", proc.stderr)


if __name__ == "__main__":
    unittest.main()
