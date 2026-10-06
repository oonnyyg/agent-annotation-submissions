import math
import unittest

from wirefield.probe import Probe, ProbeDataError, _check_basis


class ProbeBasisTests(unittest.TestCase):
    def test_rotated_probe_preserves_magnitude(self):
        # Rotate world basis by 90 deg about z.
        probe = Probe(
            "rot",
            (0.0, 0.0, 0.0),
            axis_x=(0.0, 1.0, 0.0),
            axis_y=(-1.0, 0.0, 0.0),
            axis_z=(0.0, 0.0, 1.0),
        )
        world = (0.3, -0.4, 0.5)
        local = probe.to_local(world)
        world_norm = math.sqrt(sum(c * c for c in world))
        local_norm = math.sqrt(sum(c * c for c in local))
        self.assertAlmostEqual(world_norm, local_norm, places=15)
        # Local components are dot products with the axes.
        self.assertAlmostEqual(local[0], -0.4, places=15)
        self.assertAlmostEqual(local[1], -0.3, places=15)
        self.assertAlmostEqual(local[2], 0.5, places=15)

    def test_left_handed_basis_rejected(self):
        with self.assertRaises(ProbeDataError):
            _check_basis((1.0, 0.0, 0.0), (0.0, 1.0, 0.0), (0.0, 0.0, -1.0), "t")

    def test_non_unit_axis_rejected(self):
        with self.assertRaises(ProbeDataError):
            _check_basis((2.0, 0.0, 0.0), (0.0, 1.0, 0.0), (0.0, 0.0, 1.0), "t")

    def test_non_orthogonal_axes_rejected(self):
        with self.assertRaises(ProbeDataError):
            _check_basis((1.0, 0.0, 0.0), (1.0, 0.0, 0.0), (0.0, 0.0, 1.0), "t")


if __name__ == "__main__":
    unittest.main()
