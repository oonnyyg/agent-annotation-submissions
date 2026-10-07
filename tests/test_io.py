import json
import tempfile
import unittest
from pathlib import Path

from green_ampt.io import load_segments, load_soil


def write(tmp: Path, name: str, text: str) -> Path:
    path = tmp / name
    path.write_text(text, encoding="utf-8")
    return path


class TestLoadSegments(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.tmp = Path(self._tmp.name)

    def tearDown(self):
        self._tmp.cleanup()

    def test_valid_csv(self):
        path = write(self.tmp, "rain.csv", "start,end,rate\n0,1,5\n1,2,0\n")
        segs = load_segments(path)
        self.assertEqual(len(segs), 2)
        self.assertEqual(segs[0].rate, 5.0)

    def test_negative_rate_rejected(self):
        path = write(self.tmp, "rain.csv", "start,end,rate\n0,1,-5\n")
        with self.assertRaises(ValueError):
            load_segments(path)

    def test_overlap_rejected(self):
        path = write(self.tmp, "rain.csv", "start,end,rate\n0,2,5\n1,3,5\n")
        with self.assertRaises(ValueError):
            load_segments(path)

    def test_gap_rejected(self):
        path = write(self.tmp, "rain.csv", "start,end,rate\n0,1,5\n2,3,5\n")
        with self.assertRaises(ValueError):
            load_segments(path)

    def test_zero_duration_rejected(self):
        path = write(self.tmp, "rain.csv", "start,end,rate\n0,0,5\n")
        with self.assertRaises(ValueError):
            load_segments(path)

    def test_empty_rejected(self):
        path = write(self.tmp, "rain.csv", "start,end,rate\n")
        with self.assertRaises(ValueError):
            load_segments(path)


class TestLoadSoil(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.tmp = Path(self._tmp.name)

    def tearDown(self):
        self._tmp.cleanup()

    def test_valid_soil(self):
        path = write(
            self.tmp, "soil.json",
            json.dumps({"ks": 10.0, "psi": 160.0, "delta_theta": 0.3}),
        )
        params = load_soil(path)
        self.assertEqual(params.ks, 10.0)

    def test_missing_key_rejected(self):
        path = write(self.tmp, "soil.json", json.dumps({"ks": 10.0, "psi": 160.0}))
        with self.assertRaises(ValueError):
            load_soil(path)

    def test_invalid_value_rejected(self):
        path = write(
            self.tmp, "soil.json",
            json.dumps({"ks": -1.0, "psi": 160.0, "delta_theta": 0.3}),
        )
        with self.assertRaises(ValueError):
            load_soil(path)


if __name__ == "__main__":
    unittest.main()