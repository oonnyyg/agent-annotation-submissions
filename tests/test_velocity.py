import math
import unittest

from settling.config import FluidConfig
from settling.velocity import ConvergenceError
from settling.velocity import drag_coefficient
from settling.velocity import solve_terminal_velocity

WATER = FluidConfig(density=1000.0, viscosity=1.0e-3)
G = 9.81


def stokes_velocity(d, rho_p):
    return (rho_p - WATER.density) * G * d * d / (18.0 * WATER.viscosity)


class TerminalVelocityTest(unittest.TestCase):
    def test_fine_particle_matches_stokes_limit(self):
        d, rho_p = 2e-6, 2650.0
        res = solve_terminal_velocity(d, rho_p, WATER, G)
        self.assertLess(res.reynolds, 1e-3)
        self.assertAlmostEqual(
            res.velocity / stokes_velocity(d, rho_p), 1.0, delta=1e-3
        )
        self.assertLess(res.relative_residual, 1e-12)

    def test_force_balance_and_uniqueness(self):
        res = solve_terminal_velocity(2e-4, 2650.0, WATER, G)
        self.assertGreater(res.velocity, 0.0)
        self.assertLess(res.relative_residual, 1e-12)
        cd = drag_coefficient(res.reynolds)
        drag = 0.5 * WATER.density * res.velocity**2 * (
            math.pi * (2e-4) ** 2 / 4.0
        ) * cd
        weight = (2650.0 - WATER.density) * G * math.pi * (2e-4) ** 3 / 6.0
        self.assertAlmostEqual(drag / weight, 1.0, places=9)

    def test_coarse_particle_high_reynolds(self):
        res = solve_terminal_velocity(5e-3, 2650.0, WATER, G)
        self.assertGreater(res.reynolds, 10.0)
        self.assertLess(res.relative_residual, 1e-12)

    def test_non_convergence_raises(self):
        with self.assertRaises(ConvergenceError):
            solve_terminal_velocity(2e-4, 2650.0, WATER, G, max_iter=1)


if __name__ == "__main__":
    unittest.main()
