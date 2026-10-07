import csv
import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

SRC = Path(__file__).resolve().parent.parent / "src"


def run_cli(routes_csv: str, settings: dict, out_dir: Path):
    with tempfile.TemporaryDirectory() as tmp:
        tmp_path = Path(tmp)
        routes_path = tmp_path / "routes.csv"
        routes_path.write_text(routes_csv, encoding="utf-8")
        settings_path = tmp_path / "settings.json"
        settings_path.write_text(json.dumps(settings), encoding="utf-8")
        proc = subprocess.run(
            [sys.executable, "-m", "ellipsoidal_route_sampler",
             "--routes", str(routes_path),
             "--settings", str(settings_path),
             "--out-dir", str(out_dir)],
            capture_output=True, text=True,
            env={**os.environ, "PYTHONPATH": str(SRC)},
        )
    return proc


SETTINGS = {
    "ellipsoid": {"semi_major_axis": 6378137.0,
                  "flattening": 1 / 298.257223563},
    "sample_interval": 50000.0,
    "tolerance": 1e-12,
    "max_iterations": 200,
}


class TestCli(unittest.TestCase):
    def test_end_to_end_and_failed_route_isolated(self):
        routes = (
            "route_id,lat,lon\n"
            "ok,0.0,0.0\n"
            "ok,0.0,1.0\n"
            "bad,0.0,0.0\n"
            "bad,0.5,179.5\n"
        )
        settings = dict(SETTINGS, max_iterations=5)
        with tempfile.TemporaryDirectory() as tmp:
            proc = run_cli(routes, settings, Path(tmp))
            self.assertEqual(proc.returncode, 1, proc.stderr)
            diag = json.loads((Path(tmp) / "diagnostics.json").read_text())
            self.assertEqual(diag["routes"]["ok"]["status"], "ok")
            self.assertEqual(diag["routes"]["bad"]["status"], "failed")
            self.assertEqual(diag["routes"]["bad"]["failed_segment"], 0)
            with (Path(tmp) / "segments.csv").open() as handle:
                rows = list(csv.DictReader(handle))
            self.assertEqual({r["route_id"] for r in rows}, {"ok"})
            with (Path(tmp) / "samples.csv").open() as handle:
                samples = list(csv.DictReader(handle))
            self.assertTrue(samples)
            self.assertEqual(samples[-1]["lat"], "0.0000000000")
            self.assertEqual(samples[-1]["lon"], "1.0000000000")

    def test_invalid_input_rejected(self):
        routes = "route_id,lat,lon\nr,90.0,0.0\n"
        with tempfile.TemporaryDirectory() as tmp:
            proc = run_cli(routes, SETTINGS, Path(tmp))
            self.assertEqual(proc.returncode, 2)
            self.assertIn("row 1", proc.stderr)
            self.assertIn("'r'", proc.stderr)

    def test_invalid_settings_rejected(self):
        routes = "route_id,lat,lon\nr,1.0,2.0\n"
        bad = dict(SETTINGS, sample_interval=-5.0)
        with tempfile.TemporaryDirectory() as tmp:
            proc = run_cli(routes, bad, Path(tmp))
            self.assertEqual(proc.returncode, 2)
            self.assertIn("sample_interval", proc.stderr)


if __name__ == "__main__":
    unittest.main()
