import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from sam_coverage.core import CoverageAccumulator, RecordFilter, walk_cigar
from sam_coverage.io import InputError, SamRecord, Target, load_targets, parse_sam
from sam_coverage.pipeline import run


def rec(cigar, pos=1, flag=0, mapq=60, seq=None, rname="chr1"):
    if seq is None:
        # derive query length from CIGAR
        import re
        seq = "A" * sum(
            int(n) for n, op in re.findall(r"(\d+)([MIDNSHP=X])", cigar)
            if op in "MIS=X"
        )
    return SamRecord(qname="q", flag=flag, rname=rname, pos=pos,
                     mapq=mapq, cigar=cigar, seq=seq, line_no=1)


class WalkCigarTest(unittest.TestCase):
    REFS = {"chr1": 1000}

    def test_soft_clip_does_not_extend_coverage(self):
        aligned, skipped = walk_cigar(rec("5S10M5S", pos=11), self.REFS)
        self.assertEqual(aligned, [(10, 20)])
        self.assertEqual(skipped, [])

    def test_deletion_advances_reference_without_coverage(self):
        aligned, skipped = walk_cigar(rec("5M3D5M", pos=1), self.REFS)
        self.assertEqual(aligned, [(0, 5), (8, 13)])
        self.assertEqual(skipped, [])

    def test_intron_skip_recorded_as_junction_not_coverage(self):
        aligned, skipped = walk_cigar(rec("5M100N5M", pos=1), self.REFS)
        self.assertEqual(aligned, [(0, 5), (105, 110)])
        self.assertEqual(skipped, [(5, 105)])

    def test_eq_and_x_count_as_coverage(self):
        aligned, _ = walk_cigar(rec("3=2X", pos=1), self.REFS)
        self.assertEqual(aligned, [(0, 3), (3, 5)])

    def test_query_consumption_mismatch_rejected(self):
        bad = rec("10M", seq="AAAAA")
        with self.assertRaises(InputError):
            walk_cigar(bad, self.REFS)

    def test_out_of_bounds_rejected(self):
        with self.assertRaises(InputError):
            walk_cigar(rec("20M", pos=995), self.REFS)

    def test_insert_and_hard_clip(self):
        aligned, _ = walk_cigar(rec("2H4M2I4M", pos=1), self.REFS)
        self.assertEqual(aligned, [(0, 4), (4, 8)])


class FilterTest(unittest.TestCase):
    REFS = {"chr1": 1000}

    def check(self, flag, mapq=60, min_mapq=0):
        f = RecordFilter(min_mapq, self.REFS)
        return f.check(rec("10M", flag=flag, mapq=mapq))

    def test_flag_filtering(self):
        self.assertEqual(self.check(0x4), "unmapped")
        self.assertEqual(self.check(0x100), "not_primary")
        self.assertEqual(self.check(0x800), "not_primary")
        self.assertEqual(self.check(0x200), "qc_fail")
        self.assertEqual(self.check(0x400), "duplicate")
        self.assertIsNone(self.check(0x10))  # reverse strand kept as-is

    def test_mapq_threshold(self):
        self.assertEqual(self.check(0, mapq=10, min_mapq=20), "low_mapq")
        self.assertIsNone(self.check(0, mapq=20, min_mapq=20))

    def test_unknown_reference(self):
        f = RecordFilter(0, self.REFS)
        self.assertEqual(f.check(rec("10M", rname="chrX")), "unknown_reference")


class AccumulatorTest(unittest.TestCase):
    def test_overlapping_targets_independent(self):
        t1 = Target("a", "chr1", 0, 10)
        t2 = Target("b", "chr1", 5, 15)
        a1, a2 = CoverageAccumulator(t1), CoverageAccumulator(t2)
        blocks = [(4, 8)]
        a1.add_blocks(blocks)
        a2.add_blocks(blocks)
        s1, s2 = a1.summarize(), a2.summarize()
        self.assertEqual(s1["histogram"], {0: 6, 1: 4})
        self.assertEqual(s2["histogram"], {0: 7, 1: 3})
        self.assertAlmostEqual(s1["mean_depth"], 0.4)
        self.assertAlmostEqual(s2["covered_fraction"], 0.3)

    def test_zero_depth_counted(self):
        t = Target("z", "chr1", 0, 10)
        s = CoverageAccumulator(t).summarize()
        self.assertEqual(s["histogram"], {0: 10})
        self.assertEqual(s["mean_depth"], 0.0)


SAM_TEXT = """\
@HD\tVN:1.6\tSO:unknown
@SQ\tSN:chr1\tLN:200
r1\t0\tchr1\t1\t60\t10M\t*\t0\t0\tAAAAAAAAAA\t*
r2\t0\tchr1\t6\t60\t5M50N5M\t*\t0\t0\tAAAAAAAAAA\t*
r3\t0\tchr1\t11\t60\t3S7M\t*\t0\t0\tAAAAAAAAAA\t*
r4\t0\tchr1\t21\t10\t10M\t*\t0\t0\tAAAAAAAAAA\t*
r5\t4\tchr1\t31\t60\t10M\t*\t0\t0\tAAAAAAAAAA\t*
r6\t256\tchr1\t41\t60\t10M\t*\t0\t0\tAAAAAAAAAA\t*
r7\t1024\tchr1\t51\t60\t10M\t*\t0\t0\tAAAAAAAAAA\t*
r8\t0\tchr9\t1\t60\t10M\t*\t0\t0\tAAAAAAAAAA\t*
"""

BED_TEXT = "chr1\t0\t20\tt1\nchr1\t10\t30\tt2\n"


class PipelineTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        d = Path(self.tmp.name)
        (d / "in.sam").write_text(SAM_TEXT, encoding="utf-8")
        (d / "t.bed").write_text(BED_TEXT, encoding="utf-8")
        (d / "cfg.json").write_text(json.dumps({"min_mapq": 20}),
                                    encoding="utf-8")
        self.dir = d

    def tearDown(self):
        self.tmp.cleanup()

    def run_pipeline(self):
        d = self.dir
        summary = run(str(d / "in.sam"), str(d / "t.bed"), str(d / "cfg.json"),
                      str(d / "out.csv"), str(d / "summary.json"))
        csv_text = (d / "out.csv").read_text(encoding="utf-8")
        return summary, csv_text

    def test_end_to_end(self):
        summary, csv_text = self.run_pipeline()
        recs = summary["records"]
        self.assertEqual(recs["total"], 8)
        self.assertEqual(recs["kept"], 3)
        self.assertEqual(recs["filtered"]["low_mapq"], 1)
        self.assertEqual(recs["filtered"]["unmapped"], 1)
        self.assertEqual(recs["filtered"]["not_primary"], 1)
        self.assertEqual(recs["filtered"]["duplicate"], 1)
        self.assertEqual(recs["filtered"]["unknown_reference"], 1)
        # junction from r2's 50N skip only
        self.assertEqual(
            summary["junctions"],
            [{"rname": "chr1", "start": 10, "end": 60, "count": 1}],
        )
        lines = csv_text.strip().splitlines()
        self.assertEqual(lines[0].split(","),
                         ["target", "rname", "start", "end", "length",
                          "mean_depth", "covered_fraction", "depth_histogram"])
        # t1 [0,20): r1 covers 0-10, r2 covers 5-10 (N gap not covered),
        # r3 soft-clipped to 10-17. depth: 0-5:1, 5-10:2, 10-17:1, 17-20:0
        t1 = lines[1].split(",")
        self.assertEqual(t1[0], "t1")
        self.assertEqual(t1[7], "0:3;1:12;2:5")
        self.assertAlmostEqual(float(t1[5]), (1*5 + 2*5 + 1*7) / 20)
        self.assertAlmostEqual(float(t1[6]), 17 / 20)
        # t2 [10,30): only r3's 10-17 block; r2's 55-60 is outside
        t2 = lines[2].split(",")
        self.assertEqual(t2[7], "0:13;1:7")

    def test_cli_module_invocation(self):
        d = self.dir
        proc = subprocess.run(
            [sys.executable, "-m", "sam_coverage",
             "--sam", str(d / "in.sam"), "--bed", str(d / "t.bed"),
             "--config", str(d / "cfg.json"),
             "--out-csv", str(d / "o.csv"),
             "--out-summary", str(d / "o.json")],
            capture_output=True, text=True,
            cwd=Path(__file__).resolve().parent.parent,
        )
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertTrue((d / "o.csv").exists())


class TargetValidationTest(unittest.TestCase):
    REFS = {"chr1": 100}

    def _load(self, text):
        with tempfile.NamedTemporaryFile(
                "w", suffix=".bed", delete=False, encoding="utf-8") as fh:
            fh.write(text)
            path = fh.name
        try:
            return load_targets(path, self.REFS)
        finally:
            Path(path).unlink()

    def test_duplicate_name_rejected(self):
        with self.assertRaises(InputError):
            self._load("chr1\t0\t10\ta\nchr1\t5\t15\ta\n")

    def test_unknown_reference_rejected(self):
        with self.assertRaises(InputError):
            self._load("chrZ\t0\t10\ta\n")

    def test_out_of_bounds_rejected(self):
        with self.assertRaises(InputError):
            self._load("chr1\t90\t101\ta\n")

    def test_overlapping_targets_allowed(self):
        targets = self._load("chr1\t0\t10\ta\nchr1\t5\t15\tb\n")
        self.assertEqual(len(targets), 2)


if __name__ == "__main__":
    unittest.main()
