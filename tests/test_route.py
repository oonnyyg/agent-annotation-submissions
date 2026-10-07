import math
import unittest

from ellipsoidal_route_sampler.route import (
    Waypoint,
    compute_segments,
    sample_route,
)
from ellipsoidal_route_sampler.vincenty import WGS84

TOL, MAX_ITER = 1e-12, 200


def make_route(coords, route_id="r"):
    waypoints = [Waypoint(lat=lat, lon=lon, row=i + 1)
                 for i, (lat, lon) in enumerate(coords)]
    result = compute_segments(route_id, waypoints, WGS84, TOL, MAX_ITER)
    return result, waypoints


class TestSegments(unittest.TestCase):
    def test_coincident_waypoints_add_no_distance(self):
        result, _ = make_route([(30.0, 120.0), (30.0, 120.0),
                                (31.0, 120.0)])
        self.assertTrue(result.ok)
        self.assertEqual(result.segments[0].distance, 0.0)
        self.assertTrue(math.isnan(result.segments[0].initial_bearing))
        self.assertEqual(result.total_distance,
                         result.segments[1].distance)

    def test_failed_route_reports_segment(self):
        waypoints = [Waypoint(0.0, 0.0, 1), Waypoint(0.5, 179.5, 2)]
        result = compute_segments("bad", waypoints, WGS84, TOL, 3)
        self.assertFalse(result.ok)
        self.assertEqual(result.failed_segment, 0)
        self.assertIn("segment 0", result.error)
        self.assertTrue(math.isnan(result.total_distance))


class TestSampling(unittest.TestCase):
    def test_remainder_carries_across_segments(self):
        # Two segments along the equator; step crosses the join.
        result, waypoints = make_route(
            [(0.0, 0.0), (0.0, 1.0), (0.0, 2.0)])
        step = result.total_distance / 4.0
        samples = sample_route(result, waypoints, WGS84, step, TOL, MAX_ITER)
        cumulatives = [s.cumulative for s in samples]
        # 0, step, 2*step, 3*step, then endpoint at total (4*step)
        self.assertEqual(len(samples), 5)
        for i in range(4):
            self.assertAlmostEqual(cumulatives[i], step * i, places=6)
        self.assertAlmostEqual(cumulatives[-1], result.total_distance)
        # boundary sample at the shared waypoint belongs to later segment
        boundary = samples[2]
        self.assertAlmostEqual(boundary.cumulative,
                               result.segments[0].cumulative_end, places=6)
        self.assertEqual(boundary.segment_index, 1)

    def test_endpoint_not_duplicated(self):
        result, waypoints = make_route([(0.0, 0.0), (0.0, 1.0)])
        step = result.total_distance / 2.0
        samples = sample_route(result, waypoints, WGS84, step, TOL, MAX_ITER)
        # samples at 0 and total/2, endpoint appended once at total
        self.assertEqual(len(samples), 3)
        self.assertEqual(samples[-1].cumulative, result.total_distance)
        self.assertEqual(samples[-1].segment_index, 0)

    def test_all_coincident_yields_single_sample(self):
        result, waypoints = make_route([(30.0, 120.0)] * 3)
        self.assertTrue(result.ok)
        self.assertEqual(result.total_distance, 0.0)
        samples = sample_route(result, waypoints, WGS84, 1000.0,
                               TOL, MAX_ITER)
        self.assertEqual(len(samples), 1)
        self.assertEqual(samples[0].lat, 30.0)

    def test_single_waypoint(self):
        result, waypoints = make_route([(30.0, 120.0)])
        samples = sample_route(result, waypoints, WGS84, 1000.0,
                               TOL, MAX_ITER)
        self.assertEqual(len(samples), 1)


if __name__ == "__main__":
    unittest.main()
