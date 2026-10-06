import csv
import json
import tempfile
import unittest
from pathlib import Path

from wirefield.cli import main
from wirefield.model import Coil
from wirefield.solver import evaluate

SQUARE = ((1.0, 0.0, 0.0), (0.0, 1.0, 0.0), (-1.0, 0.0, 0.0), (0.0, -1.0, 0.0))


class SafetyDistanceTests(unittest.TestCase):
    def test_point_within_safe_distance_blocked(self):
        coils = [Coil("loop", SQUARE, 10.0)]
        result = evaluate(coils, [10.0], (1.0, 0.05, 0.0), 0.1)
        self.assertFalse(result.computable)
        self.assertEqual(result.blocking_coils, ("loop",))
        self.assertEqual(result.total, (0.0, 0.0, 0.0))  # no huge pseudo-value

    def test_other_points_still_computed(self):
        coils = [Coil("loop", SQUARE, 10.0)]
        far = evaluate(coils, [10.0], (0.0, 0.0, 5.0), 0.1)
        self.assertTrue(far.computable)
        self.assertGreater(far.total[2], 0.0)


class CliEndToEndTests(unittest.TestCase):
    def test_scenario_delta_and_reports(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            coils_path = tmp / "coils.json"
            coils_path.write_text(
                json.dumps(
                    {
                        "coils": [
                            {"name": "a", "current": 4.0, "vertices": [list(v) for v in SQUARE]},
                            {"name": "b", "current": 4.0, "vertices": [list(v) for v in SQUARE]},
                        ]
                    }
                ),
                encoding="utf-8",
            )
            probes_path = tmp / "probes.csv"
            probes_path.write_text(
                "name,px,py,pz,xx,xy,xz,yx,yy,yz,zx,zy,zz\n"
                "p1,0,0,3,1,0,0,0,1,0,0,0,1\n"
                "p2,0,0,3,0,1,0,-1,0,0,0,0,1\n",
                encoding="utf-8",
            )
            scen_path = tmp / "scen.json"
            scen_path.write_text(
                json.dumps({"scenarios": [{"name": "b_reversed", "currents": {"b": -4.0}}]}),
                encoding="utf-8",
            )
            json_out = tmp / "report.json"
            csv_out = tmp / "report.csv"
            rc = main(
                [
                    "--coils", str(coils_path),
                    "--probes", str(probes_path),
                    "--scenarios", str(scen_path),
                    "--safe-distance", "0.01",
                    "--json-out", str(json_out),
                    "--csv-out", str(csv_out),
                ]
            )
            self.assertEqual(rc, 0)
            report = json.loads(json_out.read_text(encoding="utf-8"))
            self.assertEqual([s["name"] for s in report["scenarios"]], ["baseline", "b_reversed"])

            base_p1 = report["scenarios"][0]["points"][0]
            rev_p1 = report["scenarios"][1]["points"][0]
            # Reversing coil b cancels the total field exactly.
            for component in rev_p1["total_world_T"]:
                self.assertAlmostEqual(component, 0.0, places=12)
            # Delta is a vector difference: b goes +4 -> -4, so delta = -2*B_b.
            delta = rev_p1["delta_vs_baseline"]["world_T"]
            b_base = base_p1["per_coil_world_T"]["b"]
            for d, bb in zip(delta, b_base):
                self.assertAlmostEqual(d, -2.0 * bb, places=12)
            # Rotated probe p2 keeps the same world magnitude.
            base_p2 = report["scenarios"][0]["points"][1]
            self.assertAlmostEqual(base_p1["magnitude_T"], base_p2["magnitude_T"], places=12)
            # Local delta magnitude equals world delta magnitude (rotation preserves norm).
            dl = rev_p1["delta_vs_baseline"]["local_T"]
            dw = rev_p1["delta_vs_baseline"]["world_T"]
            norm = lambda v: sum(c * c for c in v) ** 0.5
            self.assertAlmostEqual(norm(dl), norm(dw), places=12)

            rows = list(csv.DictReader(csv_out.read_text(encoding="utf-8").splitlines()))
            self.assertEqual(len(rows), 4)
            self.assertIn("delta_world_x_T", rows[1])


if __name__ == "__main__":
    unittest.main()
