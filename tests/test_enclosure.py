import math
import unittest

from enclosure_radiation.model import (
    Case, CaseSurface, Enclosure, Surface, ConfigError, load_config,
)
from enclosure_radiation.solver import (
    SIGMA, UnderconstrainedError, solve_case,
)


def make_enc(areas, eps, vf):
    surfaces = tuple(
        Surface(name=f"s{i}", area=a, emissivity=e)
        for i, (a, e) in enumerate(zip(areas, eps))
    )
    return Enclosure(surfaces=surfaces,
                     view_factors=tuple(tuple(r) for r in vf))


def make_case(temps):
    return Case(name="c", surfaces=tuple(
        CaseSurface(temperature=t) for t in temps))


class IsothermalTests(unittest.TestCase):
    def test_isothermal_zero_net(self):
        enc = make_enc([1.0, 2.0], [0.5, 0.9],
                       [[0.0, 1.0], [0.5, 0.5]])
        res = solve_case(enc, make_case([500.0, 500.0]))
        for s in res.surfaces:
            self.assertAlmostEqual(s.net_power, 0.0, places=6)
            self.assertAlmostEqual(s.radiation if False else s.radiosity,
                                   SIGMA * 500.0 ** 4, places=4)
        self.assertAlmostEqual(res.energy_residual, 0.0, places=6)


class ParallelPlateTests(unittest.TestCase):
    def test_two_plates_analytic(self):
        # Infinite parallel plates: q/A = sigma (T1^4 - T2^4) / (1/e1 + 1/e2 - 1)
        e1, e2, t1, t2 = 0.8, 0.4, 800.0, 300.0
        enc = make_enc([1.0, 1.0], [e1, e2], [[0.0, 1.0], [1.0, 0.0]])
        res = solve_case(enc, make_case([t1, t2]))
        expected = SIGMA * (t1 ** 4 - t2 ** 4) / (1 / e1 + 1 / e2 - 1)
        self.assertAlmostEqual(res.surfaces[0].net_power, expected, delta=1e-4)
        self.assertAlmostEqual(res.surfaces[1].net_power, -expected, delta=1e-4)
        self.assertAlmostEqual(res.exchange[0][1], expected, delta=1e-4)
        self.assertAlmostEqual(res.exchange[1][0], -expected, delta=1e-4)


class AdiabaticTests(unittest.TestCase):
    def test_reradiating_wall(self):
        # Two plates plus an adiabatic surface seeing both equally.
        vf = [
            [0.0, 0.5, 0.5],
            [0.5, 0.0, 0.5],
            [0.5, 0.5, 0.0],
        ]
        enc = make_enc([1.0, 1.0, 1.0], [0.9, 0.9, 0.3], vf)
        res = solve_case(enc, make_case([1000.0, 300.0, None]))
        ad = res.surfaces[2]
        self.assertAlmostEqual(ad.net_power, 0.0, places=6)
        # Symmetric view factors: adiabatic J equals mean of blackbody powers.
        t_expected = ((1000.0 ** 4 + 300.0 ** 4) / 2) ** 0.25
        self.assertAlmostEqual(ad.temperature_K, t_expected, delta=1e-6)
        self.assertAlmostEqual(res.energy_residual, 0.0, delta=1e-6)

    def test_underconstrained_component(self):
        enc = make_enc([1.0, 1.0], [0.5, 0.5], [[0.0, 1.0], [1.0, 0.0]])
        with self.assertRaises(UnderconstrainedError):
            solve_case(enc, make_case([None, None]))

    def test_disconnected_components_checked_independently(self):
        # s0<->s1 and s2<->s3 are two separate enclosures.
        vf = [
            [0.0, 1.0, 0.0, 0.0],
            [1.0, 0.0, 0.0, 0.0],
            [0.0, 0.0, 0.0, 1.0],
            [0.0, 0.0, 1.0, 0.0],
        ]
        enc = make_enc([1.0] * 4, [0.5] * 4, vf)
        with self.assertRaises(UnderconstrainedError):
            solve_case(enc, make_case([400.0, None, None, None]))
        res = solve_case(enc, make_case([400.0, None, 500.0, None]))
        self.assertAlmostEqual(res.surfaces[1].temperature_K, 400.0, places=6)
        self.assertAlmostEqual(res.surfaces[3].temperature_K, 500.0, places=6)
