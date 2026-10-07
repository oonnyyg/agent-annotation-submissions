import unittest

from route_sampler.model import WGS84, Waypoint, compute_segments
from route_sampler.sampler import sample_route

TOL, MAX_ITER = 1e-12, 200


def make_route(route_id, coords):
    waypoints = [Waypoint(lat=lat, lon=lon, row=i + 1)
                 for i, (lat, lon) in enumerate(coords)]
    return compute_segments(route_id, waypoints, WGS84, TOL, MAX_ITER)


class TestSampling(unittest.TestCase):
    def test_cross_segment_remainder(self):
        route = make_route("r", [(0.0, 0.0), (0.0, 0.09), (0.0, 0.18)])
        total = route.total_distance
        spacing = total * 0.75
        samples = sample_route(route, spacing, WGS84, TOL, MAX_ITER)
        self.assertEqual(len(samples), 3)
        self.assertAlmostEqual(samples[0].cumulative, 0.0)
        self.assertAlmostEqual(samples[1].cumulative, spacing)
        self.assertAlmostEqual(samples[2].cumulative, total)
        self.assertEqual(samples[0].segment_index, 0)
        self.assertEqual(samples[1].segment_index, 1)
        self.assertEqual(samples[2].segment_index, 1)
        self.assertAlmostEqual(samples[1].lat, 0.0, places=6)
        self.assertGreater(samples[1].lon, 0.09)

    def test_boundary_attribution_fixed(self):
        route = make_route("r", [(0.0, 0.0), (0.0, 0.1), (0.0, 0.2)])
        first = route.segments[0].distance
        samples = sample_route(route, first, WGS84, TOL, MAX_ITER)
        boundary = samples[1]
        self.assertAlmostEqual(boundary.cumulative, first)
        self.assertEqual(boundary.segment_index, 0)
        self.assertEqual(samples[-1].segment_index, 1)

    def test_endpoint_not_duplicated(self):
        route = make_route("r", [(0.0, 0.0), (0.0, 0.1)])
        total = route.total_distance
        samples = sample_route(route, total / 2.0, WGS84, TOL, MAX_ITER)
        self.assertEqual(len(samples), 3)
        self.assertAlmostEqual(samples[-1].cumulative, total)
        self.assertEqual(len({s.cumulative for s in samples}), 3)

    def test_coincident_waypoints_no_mileage(self):
        route = make_route("r", [(10.0, 20.0), (10.0, 20.0), (10.0, 21.0)])
        self.assertEqual(route.segments[0].distance, 0.0)
        self.assertIsNone(route.segments[0].azimuth)
        self.assertEqual(route.segments[0].cumulative_end, 0.0)
        self.assertGreater(route.segments[1].distance, 0.0)

    def test_all_coincident_single_sample(self):
        route = make_route("r", [(10.0, 20.0)] * 3)
        samples = sample_route(route, 100.0, WGS84, TOL, MAX_ITER)
        self.assertEqual(len(samples), 1)
        self.assertEqual(samples[0].lat, 10.0)
        self.assertIsNone(samples[0].segment_index)

    def test_failed_route_not_sampled(self):
        route = make_route("r", [(0.0, 0.0), (0.5, 179.7)])
        self.assertFalse(route.ok)
        self.assertIsNone(route.total_distance)
        self.assertEqual(route.failures[0].segment_index, 0)
        self.assertEqual(route.failures[0].start_row, 1)
        self.assertEqual(route.failures[0].end_row, 2)


if __name__ == "__main__":
    unittest.main()
