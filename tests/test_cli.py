import json
import tempfile
import unittest

from binary_meta.cli import main

HEADER = "study_id,e_events,e_nonevents,c_events,c_nonevents\n"


def write_csv(text: str) -> str:
    tmp = tempfile.NamedTemporaryFile(
        "w", suffix=".csv", delete=False, encoding="utf-8", newline=""
    )
    tmp.write(text)
    tmp.close()
    return tmp.name


class CliTest(unittest.TestCase):
    def test_json_report_structure(self):
        path = write_csv(
            HEADER
            + "a,12,88,20,80\n"
            + "b,0,50,3,47\n"
            + "c,0,60,0,55\n"
            + "d,30,70,25,75\n"
        )
        out = tempfile.NamedTemporaryFile(suffix=".json", delete=False).name
        self.assertEqual(main([path, "--output", out]), 0)
        with open(out, encoding="utf-8") as handle:
            report = json.load(handle)
        self.assertEqual(report["n_included"], 3)
        self.assertEqual(report["n_excluded"], 1)
        self.assertEqual(report["excluded"][0]["study_id"], "c")
        self.assertIn("no events", report["excluded"][0]["reason"])
        ids = [s["study_id"] for s in report["studies"]]
        self.assertEqual(ids, ["a", "b", "d"])
        corrected = {s["study_id"]: s["continuity_correction"] for s in report["studies"]}
        self.assertEqual(corrected, {"a": False, "b": True, "d": False})
        self.assertEqual(len(report["leave_one_out"]), 3)
        self.assertIsNotNone(report["pooled"]["heterogeneity"]["tau2"])

    def test_csv_output_runs(self):
        path = write_csv(HEADER + "a,12,88,20,80\nb,8,42,15,35\n")
        out = tempfile.NamedTemporaryFile(suffix=".csv", delete=False).name
        self.assertEqual(main([path, "--format", "csv", "--output", out]), 0)
        with open(out, encoding="utf-8") as handle:
            text = handle.read()
        self.assertIn("pooled_random", text)
        self.assertIn("loo_random_omit_a", text)

    def test_invalid_input_exit_code(self):
        path = write_csv(HEADER + "a,1,2,3,4\na,1,2,3,4\n")
        self.assertEqual(main([path]), 2)

    def test_missing_file_exit_code(self):
        self.assertEqual(main(["does-not-exist.csv"]), 2)


if __name__ == "__main__":
    unittest.main()
