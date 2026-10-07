from __future__ import annotations

import csv
import json
import math
import tempfile
import unittest
from pathlib import Path

from persistence.cli import main
from persistence.core import InputError, analyze, load_points, load_scales
from persistence.reports import write_reports


def point(identifier: str, x: float, y: float):
    from persistence.core import Point

    return Point(identifier, x, y)


class PersistenceTests(unittest.TestCase):
    def test_equidistant_points_merge_as_one_batch(self):
        points = (
            point("c", 1.0, 0.0),
            point("a", -0.5, math.sqrt(3) / 2),
            point("b", -0.5, -math.sqrt(3) / 2),
        )

        result = analyze(points, (0.5, math.sqrt(3), 2.0))

        self.assertEqual(len(result.merges), 1)
        merge = result.merges[0]
        self.assertAlmostEqual(merge.distance, math.sqrt(3))
        self.assertEqual(merge.surviving_component, "a")
        self.assertEqual(merge.merged_components, ("b", "c"))
        self.assertEqual(merge.component_count_after, 1)
        self.assertEqual(result.intervals[0].death, None)
        self.assertAlmostEqual(result.intervals[1].death, math.sqrt(3))
        self.assertAlmostEqual(result.intervals[2].death, math.sqrt(3))

        below, exact, above = result.scale_groups
        self.assertEqual(below.component_count, 3)
        self.assertEqual(exact.component_count, 1)
        self.assertEqual(exact.groups[0][0], "a")
        self.assertEqual(exact.groups[0][1], ("a", "b", "c"))
        self.assertEqual(above.component_count, 1)

    def test_duplicate_coordinates_keep_zero_length_intervals(self):
        result = analyze(
            (point("z", 0.0, 0.0), point("a", 0.0, 0.0), point("m", 2.0, 0.0)),
            (0.0,),
        )

        zero_merge = result.merges[0]
        self.assertEqual(zero_merge.distance, 0.0)
        self.assertEqual(zero_merge.surviving_component, "a")
        self.assertEqual(zero_merge.merged_components, ("z",))
        zero_interval = next(interval for interval in result.intervals if interval.identifier == "z")
        self.assertEqual(zero_interval.birth, 0.0)
        self.assertEqual(zero_interval.death, 0.0)
        self.assertEqual(result.scale_groups[0].component_count, 2)

    def test_results_do_not_depend_on_input_row_order(self):
        ordered = (
            point("a", 0.0, 0.0),
            point("b", 1.0, 0.0),
            point("c", 2.0, 0.0),
        )
        shuffled = (ordered[2], ordered[0], ordered[1])

        first = analyze(ordered, (0.9, 1.0, 1.1))
        second = analyze(shuffled, (0.9, 1.0, 1.1))

        self.assertEqual(first.to_dict(), second.to_dict())
        exact = first.scale_groups[1]
        self.assertEqual(exact.component_count, 1)
        self.assertEqual(exact.groups[0][0], "a")

    def test_single_point_has_full_output(self):
        result = analyze((point("only", 5.0, -2.0),), (0.0, 10.0))

        self.assertEqual(len(result.intervals), 1)
        self.assertEqual(result.intervals[0].death, None)
        self.assertEqual(result.merges, ())
        self.assertEqual([group.component_count for group in result.scale_groups], [1, 1])
        self.assertEqual(result.scale_groups[0].groups[0][1], ("only",))

    def test_loader_rejects_invalid_inputs(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)
            duplicate = path / "duplicate.csv"
            duplicate.write_text("id,x,y\na,0,0\na,1,1\n", encoding="utf-8")
            with self.assertRaises(InputError):
                load_points(str(duplicate))

            invalid = path / "invalid.csv"
            invalid.write_text("id,x,y\na,nan,0\n", encoding="utf-8")
            with self.assertRaises(InputError):
                load_points(str(invalid))

            scales = path / "scales.json"
            scales.write_text('{"scales":[-1]}', encoding="utf-8")
            with self.assertRaises(InputError):
                load_scales(str(scales))

    def test_cli_writes_reports(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory)
            points_path = path / "points.csv"
            scales_path = path / "scales.json"
            output_path = path / "out"
            points_path.write_text("id,x,y\nc,1,0\na,0,0\n", encoding="utf-8")
            scales_path.write_text("[0.5, 1]", encoding="utf-8")

            exit_code = main(
                [str(points_path), str(scales_path), "--output-dir", str(output_path)]
            )

            self.assertEqual(exit_code, 0)
            payload = json.loads((output_path / "report.json").read_text(encoding="utf-8"))
            self.assertEqual(payload["scales"][1]["component_count"], 1)
            with (output_path / "scale_summary.csv").open(encoding="utf-8", newline="") as handle:
                rows = list(csv.DictReader(handle))
            self.assertEqual(rows[0]["component_count"], rows[0]["alive_interval_count"])
            self.assertEqual(rows[1]["component_count"], rows[1]["alive_interval_count"])


if __name__ == "__main__":
    unittest.main()
