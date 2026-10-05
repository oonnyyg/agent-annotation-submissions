import math
import unittest

from solute_transport import TransportConfig, run_simulation


def make_cfg(**overrides):
    base = dict(
        length=10.0,
        cells=20,
        velocity=1.0,
        dispersion=0.0,
        retardation=1.0,
        decay=0.0,
        inlet=[(0.0, 1.0)],
        output_times=[5.0, 10.0],
    )
    base.update(overrides)
    return TransportConfig.from_dict(base)


class TestValidation(unittest.TestCase):
    def test_rejects_bad_parameters(self):
        with self.assertRaises(ValueError):
            make_cfg(length=0.0)
        with self.assertRaises(ValueError):
            make_cfg(cells=1)
        with self.assertRaises(ValueError):
            make_cfg(velocity=-1.0)
        with self.assertRaises(ValueError):
            make_cfg(retardation=0.5)
        with self.assertRaises(ValueError):
            make_cfg(decay=-0.1)
        with self.assertRaises(ValueError):
            make_cfg(inlet=[(1.0, 1.0)])
        with self.assertRaises(ValueError):
            make_cfg(inlet=[(0.0, -1.0)])
        with self.assertRaises(ValueError):
            make_cfg(output_times=[2.0, 1.0])
        with self.assertRaises(ValueError):
            make_cfg(initial=[0.0])


class TestPureAdvection(unittest.TestCase):
    def test_front_travels_at_retarded_velocity(self):
        cfg = make_cfg(retardation=2.0, output_times=[4.0])
        result = run_simulation(cfg)
        profile = result["profiles"][-1]["concentration"]
        # Front should be near x = v*t/R = 2 m -> cell index ~4 of 20.
        front = max(i for i, c in enumerate(profile) if c > 0.5)
        self.assertTrue(2 <= front <= 6)
        self.assertEqual(profile[-1], 0.0)

    def test_mass_balance_closes(self):
        cfg = make_cfg(retardation=2.0, decay=0.1, output_times=[8.0])
        mb = run_simulation(cfg)["mass_balance"]
        self.assertAlmostEqual(mb["relative_residual"], 0.0, places=12)


class TestZeroVelocityDiffusion(unittest.TestCase):
    def test_diffusion_spreads_and_conserves(self):
        cfg = make_cfg(velocity=0.0, dispersion=0.5,
                       initial=[1.0] * 5 + [0.0] * 15,
                       output_times=[2.0])
        result = run_simulation(cfg)
        profile = result["profiles"][-1]["concentration"]
        self.assertGreater(profile[10], 0.0)  # spread beyond initial block
        mb = result["mass_balance"]
        self.assertAlmostEqual(mb["stored"], mb["initial"], places=9)
        self.assertEqual(mb["inflow"], 0.0)
        self.assertEqual(mb["outflow"], 0.0)

    def test_decay_removes_mass(self):
        cfg = make_cfg(velocity=0.0, dispersion=0.0, decay=0.5,
                       retardation=3.0, initial=[2.0] * 20,
                       output_times=[1.0])
        result = run_simulation(cfg)
        mb = result["mass_balance"]
        self.assertGreater(mb["decayed"], 0.0)
        self.assertAlmostEqual(mb["relative_residual"], 0.0, places=12)
        # Explicit decay under-estimates exp(-lambda t) slightly.
        expected = 2.0 * math.exp(-0.5)
        self.assertAlmostEqual(result["profiles"][-1]["concentration"][0],
                               expected, delta=0.01)


class TestInletPulse(unittest.TestCase):
    def test_pulse_breakthrough_and_balance(self):
        cfg = make_cfg(
            velocity=1.0, dispersion=0.2, retardation=1.5, decay=0.05,
            inlet=[(0.0, 1.0), (2.0, 0.0)],
            output_times=[0.0, 2.0, 4.0, 6.0, 8.0, 10.0, 12.0],
        )
        result = run_simulation(cfg)
        outlet = result["outlet"]
        self.assertEqual([row["time"] for row in outlet],
                         [0.0, 2.0, 4.0, 6.0, 8.0, 10.0, 12.0])
        # Pulse reaches the outlet around t = R*L/v = 15 s, so the
        # outlet stays clean early and mass is stored inside.
        self.assertEqual(outlet[0]["concentration"], 0.0)
        mb = result["mass_balance"]
        self.assertGreater(mb["inflow"], 0.0)
        self.assertGreater(mb["stored"], 0.0)
        self.assertAlmostEqual(mb["relative_residual"], 0.0, places=9)
        for entry in mb["history"]:
            self.assertAlmostEqual(entry["residual"], 0.0, places=9)
        for snap in result["profiles"]:
            for c in snap["concentration"]:
                self.assertGreaterEqual(c, 0.0)

    def test_landing_on_inlet_breakpoint(self):
        # Output time coincides with the inlet step; no averaging across it.
        cfg = make_cfg(inlet=[(0.0, 1.0), (3.0, 0.0)],
                       output_times=[3.0, 6.0])
        result = run_simulation(cfg)
        mb = result["mass_balance"]
        # Inflow must equal exactly v * 1.0 * 3 s (unit area).
        self.assertAlmostEqual(mb["inflow"], 3.0, places=12)


if __name__ == "__main__":
    unittest.main()
