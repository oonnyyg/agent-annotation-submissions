import contextlib
import io
import json
import tempfile
import unittest
from pathlib import Path

from green_ampt.cli import main


class TestCli(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.tmp = Path(self._tmp.name)
        (self.tmp / "soil.json").write_text(
            json.dumps({"ks": 10.0, "psi": 160.0, "delta_theta": 0.3}),
            encoding="utf-8",
        )
        (self.tmp / "rain.csv").write_text(
            "start,end,rate\n0,1,5\n1,3,40\n3,4,0\n", encoding="utf-8"
        )

    def tearDown(self):
        self._tmp.cleanup()

    def run_cli(self, *args):
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = main(list(args))
        return code, out.getvalue()

    def test_successful_run(self):
        code, out = self.run_cli(
            str(self.tmp / "rain.csv"), str(self.tmp / "soil.json")
        )
        self.assertEqual(code, 0)
        self.assertIn("ponding", out)
        self.assertIn("total runoff", out)
        self.assertIn("mass balance error : 0.000e+00", out)

    def test_bad_input_returns_error_code(self):
        (self.tmp / "bad.csv").write_text("start,end,rate\n0,1,-5\n", encoding="utf-8")
        code, _ = self.run_cli(str(self.tmp / "bad.csv"), str(self.tmp / "soil.json"))
        self.assertEqual(code, 1)

    def test_missing_file_returns_error_code(self):
        code, _ = self.run_cli(str(self.tmp / "nope.csv"), str(self.tmp / "soil.json"))
        self.assertEqual(code, 1)


if __name__ == "__main__":
    unittest.main()