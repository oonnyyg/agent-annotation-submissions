import tempfile
import unittest
from pathlib import Path

from seqaudit.errors import ObservationError
from seqaudit.observations import load_observations


def write_csv(text):
    tmp = tempfile.NamedTemporaryFile(
        "w", suffix=".csv", delete=False, encoding="utf-8")
    tmp.write(text)
    tmp.close()
    return Path(tmp.name)


class ObservationTests(unittest.TestCase):
    def test_grouping_and_ordering(self):
        path = write_csv(
            "sequence_id,position,value\n"
            "a,2,1\na,1,0\nb,1,1\n")
        grouped = load_observations(path)
        self.assertEqual([o.position for o in grouped["a"]], [1, 2])
        self.assertEqual(len(grouped["b"]), 1)

    def test_duplicate_position_rejected(self):
        path = write_csv(
            "sequence_id,position,value\n"
            "a,1,0\na,1,1\n")
        with self.assertRaises(ObservationError):
            load_observations(path)

    def test_bad_value_rejected(self):
        path = write_csv("sequence_id,position,value\na,1,2\n")
        with self.assertRaises(ObservationError):
            load_observations(path)

    def test_missing_columns_rejected(self):
        path = write_csv("sequence_id,value\na,1\n")
        with self.assertRaises(ObservationError):
            load_observations(path)


if __name__ == "__main__":
    unittest.main()
