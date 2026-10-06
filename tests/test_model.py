import json
import tempfile
import unittest
from pathlib import Path

from wirefield.model import CoilDataError, load_coils


def _write(data: dict) -> Path:
    fh = tempfile.NamedTemporaryFile(
        "w", suffix=".json", delete=False, encoding="utf-8"
    )
    json.dump(data, fh)
    fh.close()
    return Path(fh.name)


class CoilValidationTests(unittest.TestCase):
    def test_valid_coil_closes_automatically(self):
        path = _write(
            {
                "coils": [
                    {
                        "name": "tri",
                        "current": 1.5,
                        "vertices": [[0, 0, 0], [1, 0, 0], [0, 1, 0]],
                    }
                ]
            }
        )
        coils = load_coils(path)
        self.assertEqual(len(coils), 1)
        self.assertEqual(len(coils[0].segments()), 3)  # last vertex links back to first
        self.assertEqual(coils[0].segments()[-1], ((0.0, 1.0, 0.0), (0.0, 0.0, 0.0)))

    def test_too_few_vertices_rejected(self):
        path = _write({"coils": [{"name": "c", "current": 1, "vertices": [[0, 0, 0], [1, 0, 0]]}]})
        with self.assertRaises(CoilDataError):
            load_coils(path)

    def test_zero_length_edge_rejected(self):
        path = _write(
            {"coils": [{"name": "c", "current": 1, "vertices": [[0, 0, 0], [0, 0, 0], [1, 0, 0]]}]}
        )
        with self.assertRaises(CoilDataError):
            load_coils(path)

    def test_non_finite_rejected(self):
        path = _write(
            {"coils": [{"name": "c", "current": 1, "vertices": [[0, 0, 0], [1, 0, 0], [0, 1, "nan"]]}]}
        )
        # JSON 'nan' parses to float nan in Python.
        with self.assertRaises(CoilDataError):
            load_coils(path)
        path = _write(
            {"coils": [{"name": "c", "current": "inf", "vertices": [[0, 0, 0], [1, 0, 0], [0, 1, 0]]}]}
        )
        with self.assertRaises(CoilDataError):
            load_coils(path)


if __name__ == "__main__":
    unittest.main()
