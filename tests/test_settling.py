import csv
import json
import math
import os
import tempfile
import unittest

from settling.cli import main, read_samples
from settling.model import (
    ConvergenceError,
    SettlingColumn,
    SizeClass,
    ValidationError,
    load_scenario,
    solve_terminal_velocity,
)

FLUID = dict(fluid_density=998.2, viscosity=1.002e-3, gravity=9.81)


def make_column(classes, depth=2.0, area=0.05):
        return SettlingColumn(
        depth=depth,
        area=area,
        fluid_density=FLUID["fluid_density"],
        viscosity=FLUID["viscosity"],
        gravity=FLUID["gravity"],
        classes=classes,
    )


class TerminalVelocityTests(unittest.TestCase):
    def test_force_balance_residual_is_small(self):
        result = solve_terminal_velocity(1.0e-4, 2650.0, **FLUID)
        self.assertLess(result.relative_residual, 1e-10)
        self.assertGreater(result.velocity, 0.0)
        self.assertGreater(result.reynolds, 0.0)

    def test_unique_root_drag_monotonic(self):
        result = solve_terminal_velocity(5.0e-4, 2650.0, **FLUID)
        v = result.velocity
        area = math.pi * 5.0e-4**2 / 4.0
        volume = math.pi * 5.0e-4**3 / 6.0
        weight = (2650.0 - FLUID["fluid_density"]) * FLUID["gravity"] * volume

        def drag(vel):
            re = FLUID["fluid_density"] * vel * 5.0e-4 / FLUID["viscosity"]
            cd = 24.0 / re + 3.0 / math.sqrt(re) + 0.34
            return 0.5 * FLUID["fluid_density"] * cd * area * vel * vel

        self.assertLess(drag(0.5 * v), weight)
        self.assertGreater(drag(1.5 * v), weight)

    def test_fine_particle_limit_matches_stokes(self):
        # For Re << 1, Cd ~ 24/Re and the solution approaches Stokes' law.
        d = 1.0e-5
        result = solve_terminal_velocity(d, 2650.0, **FLUID)
        stokes = (2650.0 - FLUID["fluid_density"]) * FLUID["gravity"] * d * d / (
            18.0 * FLUID["viscosity"]
        )
        self.assertLess(result.reynolds, 0.1)
        self.assertAlmostEqual(result.velocity, stokes, delta=0.02 * stokes)

    def test_non_convergence_raises(self):
        with self.assertRaises(ConvergenceError):
            solve_terminal_velocity(1.0e-4, 2650.0, **FLUID, max_iterations=2)

    def test_particle_must_be_denser_than_fluid(self):
        with self.assertRaises(ValidationError):
            solve_terminal_velocity(1.0e-4, 900.0, **FLUID)


class MassBalanceTests(unittest.TestCase):
    def setUp(self):
        self.column = make_column(
            [SizeClass(1.0e-4, 2650.0, 0.02), SizeClass(2.0e-5, 2650.0, 0.01)]
        )

    def test_mass_conserved_at_several_times(self):
        for time in (0.0, 10.0, 100.0, 1.0e5):
            suspended, deposited, totals = self.column.mass_balance(time)
            for cls, s, d, tot in zip(
                self.column.classes, suspended, deposited, totals
            ):
                self.assertAlmostEqual(s + d, cls.mass, places=15)
                self.assertAlmostEqual(tot, cls.mass, places=15)
                self.assertGreaterEqual(s, 0.0)
                self.assertGreaterEqual(d, 0.0)

    def test_initial_state_all_suspended(self):
        suspended, deposited, _ = self.column.mass_balance(0.0)
        self.assertEqual(suspended, [0.02, 0.01])
        self.assertEqual(deposited, [0.0, 0.0])

    def test_fully_settled_after_long_time(self):
        v = self.column.terminal[0].velocity
        t_full = self.column.depth / v
        suspended, deposited, _ = self.column.mass_balance(t_full * 1.001)
        self.assertEqual(suspended[0], 0.0)
        self.assertAlmostEqual(deposited[0], 0.02, places=15)

    def test_linear_depletion_while_settling(self):
        v = self.column.terminal[0].velocity
        t_half = 0.5 * self.column.depth / v
        suspended = self.column.suspended_mass(t_half)
        self.assertAlmostEqual(suspended[0], 0.01, places=15)


class ConcentrationTests(unittest.TestCase):
    def setUp(self):
        self.column = make_column([SizeClass(1.0e-4, 2650.0, 0.02)])
        self.v = self.column.terminal[0].velocity
        self.uniform = 0.02 / (self.column.depth * self.column.area)

    def test_initial_uniform_concentration(self):
        (conc,) = self.column.concentration(0.0, 0.0, self.column.depth)
        self.assertAlmostEqual(conc, self.uniform, places=12)

    def test_partial_clear_water_layer(self):
        # Front has moved to mid-depth: top half is clear, bottom half uniform.
        t = 0.5 * self.column.depth / self.v
        (top,) = self.column.concentration(t, 0.0, 0.4 * self.column.depth)
        self.assertEqual(top, 0.0)
        (bottom,) = self.column.concentration(
            t, 0.6 * self.column.depth, self.column.depth
        )
        self.assertAlmostEqual(bottom, self.uniform, places=12)

    def test_overlap_not_centre(self):
        # Interval straddling the front: centre-only logic would give 0 or
        # full concentration; overlap gives exactly half.
        t = 0.5 * self.column.depth / self.v
        front = 0.5 * self.column.depth
        (conc,) = self.column.concentration(
            t, front - 0.1 * self.column.depth, front + 0.1 * self.column.depth
        )
        self.assertAlmostEqual(conc, 0.5 * self.uniform, places=12)

    def test_fully_settled_gives_zero_everywhere(self):
        t = 1.01 * self.column.depth / self.v
        (conc,) = self.column.concentration(t, 0.0, self.column.depth)
        self.assertEqual(conc, 0.0)

    def test_interval_validation(self):
        with self.assertRaises(ValidationError):
            self.column.concentration(0.0, 1.0, 1.0)  # zero thickness
        with self.assertRaises(ValidationError):
            self.column.concentration(0.0, 1.0, 0.5)  # inverted
        with self.assertRaises(ValidationError):
            self.column.concentration(0.0, 0.0, self.column.depth + 0.1)
        with self.assertRaises(ValidationError):
            self.column.concentration(-1.0, 0.0, 1.0)  # negative time


class ValidationTests(unittest.TestCase):
    def test_rejects_non_finite_and_non_positive(self):
        with self.assertRaises(ValidationError):
            make_column([SizeClass(float("nan"), 2650.0, 0.01)])
        with self.assertRaises(ValidationError):
            make_column([SizeClass(1.0e-4, 2650.0, 0.0)])
        with self.assertRaises(ValidationError):
            make_column([SizeClass(1.0e-4, 2650.0, 0.01)], depth=-1.0)
        with self.assertRaises(ValidationError):
            make_column([SizeClass(1.0e-4, 2650.0, 0.01)], area=float("inf"))

    def test_requires_at_least_one_class(self):
        with self.assertRaises(ValidationError):
            make_column([])


class CliTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.dir = self.tmp.name
        scenario = {
            "column": {"depth": 2.0, "area": 0.05},
            "fluid": {"density": FLUID["fluid_density"], "viscosity": FLUID["viscosity"], "gravity": FLUID["gravity"]},
            "classes": [
                {"diameter": 1.0e-4, "density": 2650.0, "mass": 0.02},
                {"diameter": 2.0e-5, "density": 2650.0, "mass": 0.01},
            ],
        }
        self.scenario_path = os.path.join(self.dir, "scenario.json")
        with open(self.scenario_path, "w", encoding="utf-8") as handle:
            json.dump(scenario, handle)
        self.samples_path = os.path.join(self.dir, "samples.csv")
        with open(self.samples_path, "w", encoding="utf-8", newline="") as handle:
            writer = csv.writer(handle)
            writer.writerow(["time", "z_top", "z_bottom"])
            writer.writerow([0.0, 0.0, 2.0])
            writer.writerow([50.0, 0.0, 0.5])
            writer.writerow([1.0e6, 1.0, 2.0])

    def test_cli_writes_three_reports(self):
        prefix = os.path.join(self.dir, "out")
        code = main([self.scenario_path, self.samples_path, "--out-prefix", prefix])
        self.assertEqual(code, 0)
        for suffix in ("velocities", "concentrations", "mass_balance"):
            self.assertTrue(os.path.exists(f"{prefix}_{suffix}.csv"))
        with open(f"{prefix}_mass_balance.csv", encoding="utf-8") as handle:
            rows = list(csv.DictReader(handle))
        self.assertEqual(len(rows), 3)
        for row in rows:
            self.assertAlmostEqual(
                float(row["suspended_total_kg"]) + float(row["deposited_total_kg"]),
                0.03,
                delta=1e-8,
            )
        with open(f"{prefix}_velocities.csv", encoding="utf-8") as handle:
            vel_rows = list(csv.DictReader(handle))
        self.assertEqual(len(vel_rows), 2)
        for row in vel_rows:
            self.assertLess(float(row["relative_residual"]), 1e-10)

    def test_cli_rejects_bad_interval(self):
        with open(self.samples_path, "a", encoding="utf-8", newline="") as handle:
            csv.writer(handle).writerow([10.0, 1.5, 2.5])
        code = main([self.scenario_path, self.samples_path])
        self.assertEqual(code, 1)

    def test_load_scenario_and_read_samples(self):
        column = load_scenario(self.scenario_path)
        self.assertEqual(len(column.classes), 2)
        requests = read_samples(self.samples_path)
        self.assertEqual(len(requests), 3)


if __name__ == "__main__":
    unittest.main()
