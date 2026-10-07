import csv
import json
import tempfile
import unittest

from ranked_set_enrichment.core import load_feature_scores, load_feature_sets


class LoadScoresTest(unittest.TestCase):
    def write_csv(self, rows):
        tmp = tempfile.NamedTemporaryFile("w", suffix=".csv", delete=False, newline="")
        writer = csv.writer(tmp)
        writer.writerow(["feature_id", "score"])
        writer.writerows(rows)
        tmp.close()
        return tmp.name

    def test_sorted_descending_with_id_tiebreak(self):
        path = self.write_csv([["b", 1.0], ["a", 1.0], ["c", 2.0], ["d", -1.0]])
        records = load_feature_scores(path)
        self.assertEqual([r.feature_id for r in records], ["c", "a", "b", "d"])

    def test_duplicate_id_rejected(self):
        path = self.write_csv([["a", 1.0], ["a", 2.0]])
        with self.assertRaises(ValueError):
            load_feature_scores(path)

    def test_non_finite_score_rejected(self):
        for bad in ["nan", "inf", "-inf", "abc"]:
            path = self.write_csv([["a", bad]])
            with self.assertRaises(ValueError, msg=bad):
                load_feature_scores(path)


class LoadSetsTest(unittest.TestCase):
    def write_json(self, payload):
        tmp = tempfile.NamedTemporaryFile("w", suffix=".json", delete=False, encoding="utf-8")
        json.dump(payload, tmp)
        tmp.close()
        return tmp.name

    def test_sets_list_shape(self):
        path = self.write_json({"sets": [{"name": "s1", "members": ["a", "b"]}]})
        sets = load_feature_sets(path)
        self.assertEqual(sets[0].name, "s1")
        self.assertEqual(sets[0].members, ["a", "b"])

    def test_mapping_shape(self):
        path = self.write_json({"s1": ["a"], "s2": ["b", "c"]})
        sets = load_feature_sets(path)
        self.assertEqual([s.name for s in sets], ["s1", "s2"])


if __name__ == "__main__":
    unittest.main()
