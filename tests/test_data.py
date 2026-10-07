import tempfile
import unittest

from binary_meta.data import DataError, load_studies

HEADER = "study_id,e_events,e_nonevents,c_events,c_nonevents\n"


def write_csv(text: str) -> str:
    tmp = tempfile.NamedTemporaryFile(
        "w", suffix=".csv", delete=False, encoding="utf-8", newline=""
    )
    tmp.write(text)
    tmp.close()
    return tmp.name


class LoadStudiesTest(unittest.TestCase):
    def test_valid_rows(self):
        path = write_csv(HEADER + "s1,10,90,20,80\ns2,0,50,3,47\n")
        studies = load_studies(path)
        self.assertEqual([s.study_id for s in studies], ["s1", "s2"])
        self.assertEqual(studies[0].e_total, 100)

    def test_duplicate_study_id_reports_line(self):
        path = write_csv(HEADER + "s1,1,9,2,8\ns1,3,7,4,6\n")
        with self.assertRaisesRegex(DataError, r"line 3.*duplicate.*s1"):
            load_studies(path)

    def test_negative_count_reports_line(self):
        path = write_csv(HEADER + "s1,1,9,2,8\ns2,-1,10,2,8\n")
        with self.assertRaisesRegex(DataError, r"line 3.*negative"):
            load_studies(path)

    def test_non_integer_count(self):
        path = write_csv(HEADER + "s1,1.5,9,2,8\n")
        with self.assertRaisesRegex(DataError, r"line 2.*not an integer"):
            load_studies(path)

    def test_zero_group_total(self):
        path = write_csv(HEADER + "s1,0,0,2,8\n")
        with self.assertRaisesRegex(DataError, r"line 2.*experimental group total"):
            load_studies(path)

    def test_missing_column(self):
        path = write_csv("study_id,e_events,e_nonevents,c_events\ns1,1,9,2\n")
        with self.assertRaisesRegex(DataError, r"missing required columns.*c_nonevents"):
            load_studies(path)

    def test_empty_file(self):
        path = write_csv("")
        with self.assertRaisesRegex(DataError, "empty"):
            load_studies(path)


if __name__ == "__main__":
    unittest.main()
