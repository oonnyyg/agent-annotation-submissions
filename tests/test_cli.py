import csv
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from route_sampler.io import load_routes, load_settings
from route_sampler.model import RouteInputError, WGS84


class CliTestBase(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.dir = Path(self.tmp.name)
        self.settings = self.dir / "settings.json"
        self.settings.write_text(json.dumps({
            "sample_spacing_m": 5000.0,
            "tolerance": 1e-12,
            "max_iterations": 200,
        }), encoding="utf-8")

    def tearDown(self):
        self.tmp.cleanup()

    def write_waypoints(self, rows):
        path = self.dir / "waypoints.csv"
        with open(path, "w", encoding="utf-8", newline="") as fh:
            writer = csv.writer(fh)
            writer.writerow(["route_id", "lat", "lon"])
            writer.writerows(rows)
        return path

    def run_cli(self, waypoints):
        out_dir = self.dir / "out"
        proc = subprocess.run(
            [sys.executable, "-m", "route_sampler", str(waypoints),
             str(self.settings), "--out-dir", str(out_dir)],
            capture_output=True, text=True,
        )
        return proc, out_dir


class TestCli(CliTestBase):
    def test_end_to_end_outputs(self):
        waypoints = self.write_waypoints([
            ["alpha", 0.0, 0.0],
            ["alpha", 0.0, 0.1],
            ["beta", 10.0, 20.0],
            ["beta", 10.0, 20.0],
        ])
        proc, out_dir = self.run_cli(waypoints)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        with open(out_dir / "segments.csv", encoding="utf-8") as fh:
            segments = list(csv.DictReader(fh))
        self.assertEqual(len(segments), 2)
        self.assertEqual(segments[1]["azimuth_deg"], "")
        with open(out_dir / "samples.csv", encoding="utf-8") as fh:
            samples = list(csv.DictReader(fh))
        beta = [s for s in samples if s["route_id"] == "beta"]
        self.assertEqual(len(beta), 1)
        diag = json.loads((out_dir / "diagnostics.json").read_text(encoding="utf-8"))
        self.assertEqual(diag["ellipsoid"]["a"], WGS84.a)
        alpha = next(r for r in diag["routes"] if r["route_id"] == "alpha")
        self.assertEqual(alpha["status"], "ok")
        self.assertGreater(alpha["total_distance_m"], 0.0)

    def test_failed_route_isolated(self):
        waypoints = self.write_waypoints([
            ["bad", 0.0, 0.0],
            ["bad", 0.5, 179.7],
            ["good", 0.0, 0.0],
            ["good", 0.0, 0.1],
        ])
        proc, out_dir = self.run_cli(waypoints)
        self.assertEqual(proc.returncode, 2)
        diag = json.loads((out_dir / "diagnostics.json").read_text(encoding="utf-8"))
        bad = next(r for r in diag["routes"] if r["route_id"] == "bad")
        self.assertEqual(bad["status"], "failed")
        self.assertIsNone(bad["total_distance_m"])
        self.assertEqual(bad["failed_segments"][0]["segment_index"], 0)
        good = next(r for r in diag["routes"] if r["route_id"] == "good")
        self.assertEqual(good["status"], "ok")
        with open(out_dir / "samples.csv", encoding="utf-8") as fh:
            samples = list(csv.DictReader(fh))
        self.assertTrue(all(s["route_id"] == "good" for s in samples))

    def test_invalid_waypoint_rejected_with_location(self):
        waypoints = self.write_waypoints([
            ["r", 0.0, 0.0],
            ["r", 90.0, 10.0],
        ])
        proc, _ = self.run_cli(waypoints)
        self.assertEqual(proc.returncode, 1)
        self.assertIn("route 'r' row 2", proc.stderr)


class TestValidation(unittest.TestCase):
    def test_non_finite_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "w.csv"
            path.write_text(
                "route_id,lat,lon\nr,0,0\nr,nan,5\n", encoding="utf-8"
            )
            with self.assertRaises(RouteInputError) as ctx:
                load_routes(path)
            self.assertEqual(ctx.exception.row, 2)

    def test_flattening_bounds(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "s.json"
            for bad_f in (0.01, -0.001):
                path.write_text(json.dumps({
                    "ellipsoid": {"a": 6378137.0, "f": bad_f},
                    "sample_spacing_m": 100.0,
                }), encoding="utf-8")
                with self.assertRaises(ValueError):
                    load_settings(path)


if __name__ == "__main__":
    unittest.main()
