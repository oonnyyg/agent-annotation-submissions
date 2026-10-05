import json
import math
import os
import subprocess
import sys
import tempfile
import unittest

from retarded_transport import TransportError, simulate


def base_config(**overrides):
    config = {
        "length": 10.0,
        "cells": 50,
        "velocity": 1.0,
        "dispersion": 0.0,
        "retardation": 2.0,
        "decay": 0.0,
        "inlet": [[0.0, 1.0]],
        "output_times": [0.0, 2.5, 5.0, 10.0],
    }
    config.update(overrides)
    return config


class TestValidation(unittest.TestCase):
    def test_rejects_bad_cells(self):
        with self.assertRaises(TransportError):
            simulate(base_config(cells=1))

    def test_rejects_retardation_below_one(self):
        with self.assertRaises(TransportError):
            simulate(base_config(retardation=0.5))

    def test_rejects_negative_values(self):
        for key in ("velocity", "dispersion", "decay"):
            with self.assertRaises(TransportError):
                simulate(base_config(**{key: -1.0}))

    def test_rejects_unsorted_inlet(self):
        with self.assertRaises(TransportError):
            simulate(base_config(inlet=[[0.0, 1.0], [0.5, 0.5], [0.4, 0.0]]))

    def test_rejects_inlet_not_starting_at_zero(self):
        with self.assertRaises(TransportError):
            simulate(base_config(inlet=[[1.0, 1.0]]))

    def test_rejects_nonfinite(self):
        with self.assertRaises(TransportError):
            simulate(base_config(velocity=float("nan")))


class TestPureAdvection(unittest.TestCase):
    def test_front_moves_at_retarded_speed(self):
        cfg = base_config(output_times=[4.0])
        result = simulate(cfg)
        c = result["profiles"][-1]["concentration"]
        dx = result["dx"]
        # Retarded front at x = v*t/R = 2.0 m; upstream cells ~1, downstream ~0.
        front_index = int(2.0 / dx)
        self.assertTrue(all(v > 0.99 for v in c[:front_index - 3]))
        self.assertTrue(all(v < 1e-12 for v in c[front_index + 2:]))

    def test_no_negative_concentrations(self):
        result = simulate(base_config(output_times=[1.0, 3.0, 5.0]))
        for profile in result["profiles"]:
            self.assertTrue(all(v >= 0.0 for v in profile["concentration"]))


class TestZeroVelocityDiffusion(unittest.TestCase):
    def test_mass_conserved_and_spreads(self):
        cfg = base_config(
            velocity=0.0,
            dispersion=0.5,
            retardation=1.0,
            inlet=[[0.0, 0.0]],
            initial=[1.0] * 10 + [0.0] * 40,
            output_times=[0.0, 1.0, 4.0],
        )
        result = simulate(cfg)
        first = result["profiles"][0]["concentration"]
        last = result["profiles"][-1]["concentration"]
        self.assertAlmostEqual(sum(first), sum(last), places=9)
        self.assertGreater(last[20], first[20])  # solute spread downstream
        self.assertEqual(result["outlet"][-1]["concentration"], last[-1])


class TestPulse(unittest.TestCase):
    def test_pulse_and_decay(self):
        cfg = base_config(
            decay=0.05,
            inlet=[[0.0, 2.0], [1.0, 0.0]],
            output_times=[0.0, 1.0, 2.0, 6.0, 12.0],
        )
        result = simulate(cfg)
        outlet = result["outlet"]
        # Inlet pulse ends at t=1; retarded arrival at outlet needs
        # t > R*L/v = 20 s, so outlet stays at zero here.
        self.assertTrue(all(row["concentration"] == 0.0 for row in outlet))
        balance = result["mass_balance"]
        self.assertAlmostEqual(balance["cumulative_inflow"], 2.0 * 1.0 * 1.0, places=9)
        self.assertGreater(balance["cumulative_decay"], 0.0)
        self.assertLess(balance["final_inventory"], balance["cumulative_inflow"])

    def test_event_times_hit_exactly(self):
        cfg = base_config(inlet=[[0.0, 1.0], [0.3, 0.0]], output_times=[0.3, 0.7, 1.1])
        result = simulate(cfg)
        self.assertEqual([row["time"] for row in result["outlet"]], [0.3, 0.7, 1.1])


class TestMassBalance(unittest.TestCase):
    def test_budget_closes(self):
        cfg = base_config(
            dispersion=0.2,
            decay=0.1,
            inlet=[[0.0, 1.5], [0.8, 0.2], [2.0, 0.0]],
            initial=0.3,
            output_times=[0.0, 0.5, 1.5, 3.0, 8.0],
        )
        balance = simulate(cfg)["mass_balance"]
        lhs = balance["initial_inventory"] + balance["cumulative_inflow"]
        rhs = (
            balance["cumulative_outflow"]
            + balance["cumulative_decay"]
            + balance["final_inventory"]
        )
        self.assertAlmostEqual(lhs, rhs, places=9)
        self.assertLess(abs(balance["relative_residual"]), 1e-12)


class TestCli(unittest.TestCase):
    def test_cli_writes_outputs(self):
        cfg = base_config()
        with tempfile.TemporaryDirectory() as tmp:
            cfg_path = os.path.join(tmp, "config.json")
            with open(cfg_path, "w", encoding="utf-8") as fh:
                json.dump(cfg, fh)
            proc = subprocess.run(
                [sys.executable, "-m", "retarded_transport.cli", cfg_path],
                cwd=tmp,
                capture_output=True,
                text=True,
                env={**os.environ, "PYTHONPATH": os.getcwd()},
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            for name in ("profile.csv", "outlet.csv", "mass_balance.json"):
                self.assertTrue(os.path.exists(os.path.join(tmp, name)))
            with open(os.path.join(tmp, "mass_balance.json"), encoding="utf-8") as fh:
                balance = json.load(fh)
            self.assertLess(abs(balance["relative_residual"]), 1e-12)


if __name__ == "__main__":
    unittest.main()
