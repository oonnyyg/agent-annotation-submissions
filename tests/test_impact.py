import math
import unittest

from impact.core import Disk, SimConfig, SimError, run


def cfg(**kw):
    base = dict(width=10.0, height=10.0, restitution=1.0, friction=0.0,
                t_end=1.0, dt_sample=0.5)
    base.update(kw)
    return SimConfig(**base)


class TestWallBounce(unittest.TestCase):
    def test_elastic_wall_bounce(self):
        d = Disk(x=1.0, y=5.0, vx=-2.0, vy=0.0, omega=0.0,
                 mass=1.0, radius=0.5)
        sim = run([d], cfg(t_end=1.0))
        self.assertTrue(sim.completed)
        self.assertEqual(len(sim.events), 1)
        ev = sim.events[0]
        self.assertAlmostEqual(ev["t"], 0.25)
        self.assertEqual(ev["contacts"][0]["wall"], "left")
        self.assertAlmostEqual(d.vx, 2.0)
        self.assertAlmostEqual(d.x, 2.0)  # 0.5 + 2*0.75

    def test_spin_from_friction_wall(self):
        # Disk sliding along the wall: friction at contact creates spin.
        d = Disk(x=2.0, y=5.0, vx=-1.0, vy=1.0, omega=0.0,
                 mass=1.0, radius=1.0)
        sim = run([d], cfg(t_end=1.5, friction=0.5))
        self.assertEqual(len(sim.events), 1)
        self.assertGreater(d.omega, 0.0)  # spin picked up from friction
        self.assertGreater(d.vx, 0.0)
        c = sim.events[0]["contacts"][0]
        self.assertLessEqual(abs(c["impulse_tangent"]),
                             0.5 * c["impulse_normal"] + 1e-12)


class TestDiskDisk(unittest.TestCase):
    def test_head_on_elastic_equal_mass(self):
        a = Disk(x=2.0, y=5.0, vx=1.0, vy=0.0, omega=0.0,
                 mass=1.0, radius=1.0)
        b = Disk(x=6.0, y=5.0, vx=-1.0, vy=0.0, omega=0.0,
                 mass=1.0, radius=1.0)
        sim = run([a, b], cfg(t_end=1.5))
        self.assertAlmostEqual(sim.events[0]["t"], 1.0)
        self.assertAlmostEqual(a.vx, -1.0)
        self.assertAlmostEqual(b.vx, 1.0)
        diag = sim.diagnostics()
        self.assertAlmostEqual(diag["kinetic_energy_final"],
                               diag["kinetic_energy_initial"], places=9)
        self.assertAlmostEqual(
            diag["total_linear_momentum"][0],
            diag["initial_linear_momentum"][0], places=9)

    def test_eccentric_collision_exchanges_spin(self):
        # Offset (eccentric) impact: translation and spin couple.
        a = Disk(x=2.0, y=5.0, vx=2.0, vy=0.0, omega=0.0,
                 mass=1.0, radius=1.0)
        b = Disk(x=5.0, y=5.8, vx=0.0, vy=0.0, omega=0.0,
                 mass=1.0, radius=1.0)
        sim = run([a, b], cfg(t_end=2.0, friction=0.3))
        self.assertEqual(len(sim.events), 1)
        self.assertNotAlmostEqual(a.omega, 0.0)
        self.assertNotAlmostEqual(b.omega, 0.0)
        diag = sim.diagnostics()
        # Energy must not increase; momentum conserved (no walls hit yet).
        self.assertLessEqual(diag["kinetic_energy_final"],
                             diag["kinetic_energy_initial"] + 1e-9)
        for k in (0, 1):
            self.assertAlmostEqual(
                diag["total_linear_momentum"][k],
                diag["initial_linear_momentum"][k], places=9)

    def test_perfect_elastic_frictionless_ke_conserved(self):
        a = Disk(x=2.0, y=4.0, vx=1.5, vy=0.3, omega=2.0,
                 mass=1.0, radius=1.0)
        b = Disk(x=6.0, y=4.5, vx=-1.0, vy=-0.2, omega=-1.0,
                 mass=2.0, radius=1.0)
        sim = run([a, b], cfg(t_end=0.6))
        diag = sim.diagnostics()
        self.assertAlmostEqual(diag["kinetic_energy_final"],
                               diag["kinetic_energy_initial"], places=9)

    def test_grazing_not_retriggered(self):
        # Touching but moving apart: no event should fire.
        a = Disk(x=3.0, y=5.0, vx=-1.0, vy=0.0, omega=0.0,
                 mass=1.0, radius=1.0)
        b = Disk(x=5.0, y=5.0, vx=1.0, vy=0.0, omega=0.0,
                 mass=1.0, radius=1.0)
        sim = run([a, b], cfg(t_end=1.0))
        self.assertEqual(len(sim.events), 0)
        self.assertTrue(sim.completed)


class TestValidationAndLimits(unittest.TestCase):
    def test_reject_overlap(self):
        a = Disk(x=3.0, y=5.0, vx=0.0, vy=0.0, omega=0.0,
                 mass=1.0, radius=1.0)
        b = Disk(x=4.5, y=5.0, vx=0.0, vy=0.0, omega=0.0,
                 mass=1.0, radius=1.0)
        with self.assertRaises(SimError):
            run([a, b], cfg())

    def test_reject_out_of_bounds(self):
        a = Disk(x=0.2, y=5.0, vx=0.0, vy=0.0, omega=0.0,
                 mass=1.0, radius=1.0)
        with self.assertRaises(SimError):
            run([a], cfg())

    def test_reject_bad_coefficients(self):
        a = Disk(x=5.0, y=5.0, vx=0.0, vy=0.0, omega=0.0,
                 mass=1.0, radius=1.0)
        with self.assertRaises(SimError):
            run([a], cfg(restitution=1.5))
        with self.assertRaises(SimError):
            run([a], cfg(friction=-0.1))

    def test_event_limit_not_complete(self):
        d = Disk(x=5.0, y=5.0, vx=3.0, vy=2.0, omega=0.0,
                 mass=1.0, radius=0.5)
        sim = run([d], cfg(t_end=100.0, max_events=5))
        self.assertFalse(sim.completed)
        self.assertIn("event limit", sim.stopped_reason)

    def test_simultaneous_shared_disk_stops(self):
        # Disk 0 hits disks 1 and 2 at the same instant.
        a = Disk(x=5.0, y=5.0, vx=1.0, vy=0.0, omega=0.0,
                 mass=1.0, radius=1.0)
        b = Disk(x=7.0, y=6.0, vx=0.0, vy=0.0, omega=0.0,
                 mass=1.0, radius=1.0)
        c = Disk(x=7.0, y=4.0, vx=0.0, vy=0.0, omega=0.0,
                 mass=1.0, radius=1.0)
        with self.assertRaises(SimError) as ctx:
            run([a, b, c], cfg(t_end=2.0))
        self.assertIn("share disk", str(ctx.exception))


if __name__ == "__main__":
    unittest.main()
