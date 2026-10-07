import math
import unittest

from impact.engine import (Disk, MultiContactError, SimConfig, SimError,
                           Simulation)


def make_cfg(**kw):
    base = dict(width=10.0, height=10.0, restitution=1.0, friction=0.0,
                t_end=5.0, sample_dt=0.5)
    base.update(kw)
    return SimConfig(**base)


def run(cfg, disks):
    return Simulation(cfg, disks).run()


class WallBounceTest(unittest.TestCase):
    def test_elastic_wall_bounce(self):
        sim = run(make_cfg(), [Disk(5.0, 5.0, 2.0, 0.0, 0.0, 1.0, 1.0)])
        self.assertTrue(sim.completed)
        self.assertEqual(len(sim.events), 1)
        ev = sim.events[0]
        self.assertAlmostEqual(ev["time"], 2.0)  # (10-1-5)/2
        d = sim.disks[0]
        self.assertAlmostEqual(d.vx, -2.0)
        self.assertAlmostEqual(d.omega, 0.0)
        self.assertAlmostEqual(ev["ke_delta"], 0.0, places=9)

    def test_wall_friction_kills_spin_and_slip(self):
        sim = run(make_cfg(friction=0.5),
                  [Disk(5.0, 5.0, 2.0, 1.0, 3.0, 1.0, 1.0)])
        d = sim.disks[0]
        vt = d.vy + d.omega * d.radius
        self.assertAlmostEqual(vt, 0.0, places=9)
        self.assertAlmostEqual(d.vx, -2.0)

    def test_inelastic_wall_loses_energy(self):
        sim = run(make_cfg(restitution=0.5),
                  [Disk(5.0, 5.0, 2.0, 0.0, 0.0, 1.0, 1.0)])
        self.assertLess(sim.events[0]["ke_delta"], 0.0)
        self.assertAlmostEqual(sim.disks[0].vx, -1.0)


class DiskDiskTest(unittest.TestCase):
    def test_head_on_elastic_equal_mass(self):
        disks = [Disk(3.0, 5.0, 1.0, 0.0, 0.0, 1.0, 1.0),
                 Disk(7.0, 5.0, -1.0, 0.0, 0.0, 1.0, 1.0)]
        sim = run(make_cfg(t_end=1.5), disks)
        self.assertEqual(len(sim.events), 1)
        self.assertAlmostEqual(sim.disks[0].vx, -1.0)
        self.assertAlmostEqual(sim.disks[1].vx, 1.0)
        self.assertAlmostEqual(sim.events[0]["ke_delta"], 0.0, places=9)
        self.assertAlmostEqual(ev_momentum_check(sim), 0.0, places=9)

    def test_eccentric_collision_exchanges_spin(self):
        disks = [Disk(4.0, 4.0, 2.0, 0.0, 0.0, 1.0, 1.0),
                 Disk(6.0, 5.5, 0.0, 0.0, 0.0, 1.0, 1.0)]
        sim = run(make_cfg(friction=0.5, t_end=0.6, sample_dt=1.0), disks)
        self.assertEqual(len(sim.events), 1)
        ev = sim.events[0]
        self.assertAlmostEqual(ev["momentum_check"], 0.0, places=9)
        self.assertLess(ev["ke_delta"], 0.0)  # friction dissipates
        self.assertNotAlmostEqual(sim.disks[0].omega, 0.0)
        self.assertNotAlmostEqual(sim.disks[1].omega, 0.0)
        nx, ny = ev["contacts"][0]["normal"]
        rvn = ((sim.disks[1].vx - sim.disks[0].vx) * nx +
               (sim.disks[1].vy - sim.disks[0].vy) * ny)
        self.assertGreater(rvn, 0.0)
        jn, jt = ev["contacts"][0]["jn"], ev["contacts"][0]["jt"]
        tx, ty = -ny, nx
        jx, jy = jn * nx + jt * tx, jn * ny + jt * ty
        for disk in sim.disks:
            # offset x impulse = (+/- r n) x (-/+ J): same sign for both
            torque = -disk.radius * (nx * jy - ny * jx)
            self.assertAlmostEqual(disk.inertia * disk.omega, torque,
                                   places=9)

    def test_grazing_contact_not_triggered(self):
        disks = [Disk(2.0, 4.0, 1.0, 0.0, 0.0, 1.0, 1.0),
                 Disk(2.0, 6.0, 1.0, 0.0, 0.0, 1.0, 1.0)]
        sim = run(make_cfg(t_end=1.0), disks)
        self.assertTrue(sim.completed)
        self.assertEqual(len(sim.events), 0)

    def test_frictionless_elastic_conserves_energy(self):
        disks = [Disk(4.0, 4.0, 2.0, 0.0, 5.0, 1.0, 1.0),
                 Disk(6.0, 5.5, 0.0, 0.0, -3.0, 1.0, 1.0)]
        sim = run(make_cfg(t_end=0.6, sample_dt=1.0), disks)
        self.assertEqual(len(sim.events), 1)
        ev = sim.events[0]
        self.assertAlmostEqual(ev["ke_delta"], 0.0, places=9)
        self.assertAlmostEqual(ev["momentum_check"], 0.0, places=9)
        # frictionless: no tangential impulse, spin unchanged
        self.assertAlmostEqual(ev["contacts"][0]["jt"], 0.0)
        self.assertAlmostEqual(sim.disks[0].omega, 5.0)


def ev_momentum_check(sim):
    return sim.events[0]["momentum_check"]


class ValidationTest(unittest.TestCase):
    def test_overlap_rejected(self):
        disks = [Disk(3.0, 5.0, 0.0, 0.0, 0.0, 1.0, 1.0),
                 Disk(4.5, 5.0, 0.0, 0.0, 0.0, 1.0, 1.0)]
        with self.assertRaises(SimError):
            Simulation(make_cfg(), disks)

    def test_out_of_bounds_rejected(self):
        with self.assertRaises(SimError):
            Simulation(make_cfg(), [Disk(0.5, 5.0, 0, 0, 0, 1.0, 1.0)])

    def test_bad_restitution_rejected(self):
        with self.assertRaises(SimError):
            Simulation(make_cfg(restitution=1.5),
                       [Disk(5.0, 5.0, 0, 0, 0, 1.0, 1.0)])

    def test_multi_contact_aborts(self):
        disks = [Disk(3.5, 5.0, 1.0, 0.0, 0.0, 1.0, 1.0),
                 Disk(6.0, 5.0, 0.0, 0.0, 0.0, 1.0, 1.0),
                 Disk(8.5, 5.0, -1.0, 0.0, 0.0, 1.0, 1.0)]
        with self.assertRaises(MultiContactError) as ctx:
            run(make_cfg(), disks)
        self.assertEqual(len(ctx.exception.contacts), 2)

    def test_event_limit_incomplete(self):
        cfg = make_cfg(restitution=1.0, max_events=2, t_end=100.0,
                       sample_dt=10.0)
        sim = run(cfg, [Disk(5.0, 5.0, 3.0, 2.0, 0.0, 1.0, 1.0)])
        self.assertFalse(sim.completed)
        self.assertEqual(len(sim.events), 2)


class SamplingTest(unittest.TestCase):
    def test_sample_at_event_uses_post_state(self):
        sim = run(make_cfg(t_end=4.0, sample_dt=1.0),
                  [Disk(5.0, 5.0, 2.0, 0.0, 0.0, 1.0, 1.0)])
        at2 = [s for s in sim.samples if abs(s["time"] - 2.0) < 1e-9][0]
        self.assertLess(at2["disks"][0]["vx"], 0.0)


if __name__ == "__main__":
    unittest.main()
