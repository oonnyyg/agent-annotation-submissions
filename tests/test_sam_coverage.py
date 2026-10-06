"""Tests for sam_coverage: soft clips, overlapping targets, introns."""

from __future__ import annotations

import json
import subprocess
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT))

from sam_coverage.coverage import compute_coverage  # noqa: E402
from sam_coverage.model import (  # noqa: E402
    Config,
    InputError,
    SamHeader,
    load_targets,
    parse_sam,
)


def write(tmp: Path, name: str, text: str) -> str:
    path = tmp / name
    path.write_text(text, encoding="utf-8")
    return str(path)


SAM_HEADER = "@HD\tVN:1.6\n@SQ\tSN:chr1\tLN:1000\n@SQ\tSN:chr2\tLN:500\n"


def make_record(qname, flag, rname, pos, mapq, cigar, seq):
    return "\t".join(
        [qname, str(flag), rname, str(pos), str(mapq), cigar, "*", "0", "0", seq, "*"]
    )


class CoverageTests(unittest.TestCase):
    def setUp(self):
        self.tmp = ROOT / "tests" / "_tmp"
        self.tmp.mkdir(exist_ok=True)
        self.header = SamHeader({"chr1": 1000, "chr2": 500})

    def tearDown(self):
        for path in self.tmp.glob("*"):
            path.unlink()
        self.tmp.rmdir()

    def run_pipeline(self, sam_text, bed_text, min_mapq=0):
        sam_path = write(self.tmp, "in.sam", sam_text)
        bed_path = write(self.tmp, "in.bed", bed_text)
        header, records = parse_sam(sam_path)
        targets = load_targets(bed_path, header)
        return compute_coverage(records, targets, header, min_mapq)

    def test_soft_clip_and_deletion_not_counted(self):
        # 5S then 5M: covers 5..9 only. 3D advances without coverage.
        rec = make_record("r1", 0, "chr1", 6, 60, "5S5M3D5M", "SSSSSAAAAACCCCC")
        result = self.run_pipeline(SAM_HEADER + rec + "\n", "chr1\t0\t20\tt1\n")
        entry = result.targets[0]
        # covered: 5-9 and 13-17 -> 10 bases at depth 1, 10 bases at 0
        self.assertEqual(entry.histogram, {0: 10, 1: 10})
        self.assertAlmostEqual(entry.mean_depth, 0.5)
        self.assertAlmostEqual(entry.fraction_covered_1x, 0.5)
        self.assertEqual(result.filters.kept, 1)

    def test_intron_n_not_counted_and_junction_reported(self):
        # 5M100N5M spanning an intron; N region must not count as coverage.
        rec = make_record("r1", 0, "chr1", 1, 60, "5M100N5M", "AAAAAAAAAA")
        result = self.run_pipeline(SAM_HEADER + rec + "\n", "chr1\t0\t120\tt1\n")
        entry = result.targets[0]
        self.assertEqual(entry.histogram, {0: 110, 1: 10})
        self.assertEqual(len(result.junctions), 1)
        junction = result.junctions[0]
        self.assertEqual((junction.rname, junction.start, junction.end), ("chr1", 5, 105))
        self.assertEqual(junction.count, 1)

    def test_overlapping_targets_independent(self):
        rec = make_record("r1", 0, "chr1", 11, 60, "10M", "AAAAAAAAAA")
        bed = "chr1\t0\t20\ttA\nchr1\t15\t30\ttB\n"
        result = self.run_pipeline(SAM_HEADER + rec + "\n", bed)
        t_a, t_b = result.targets
        self.assertEqual(t_a.histogram, {0: 10, 1: 10})
        self.assertEqual(t_b.histogram, {0: 10, 1: 5})

    def test_filter_order_and_counts(self):
        lines = [
            make_record("unmap", 4, "*", 0, 0, "*", "*"),
            make_record("sec", 256, "chr1", 1, 60, "10M", "AAAAAAAAAA"),
            make_record("supp", 2048, "chr1", 1, 60, "10M", "AAAAAAAAAA"),
            make_record("qcfail", 512, "chr1", 1, 60, "10M", "AAAAAAAAAA"),
            make_record("dup", 1024, "chr1", 1, 60, "10M", "AAAAAAAAAA"),
            make_record("lowq", 0, "chr1", 1, 10, "10M", "AAAAAAAAAA"),
            make_record("keep", 0, "chr1", 1, 60, "10M", "AAAAAAAAAA"),
        ]
        result = self.run_pipeline(
            SAM_HEADER + "\n".join(lines) + "\n", "chr1\t0\t20\tt1\n", min_mapq=20
        )
        counts = result.filters
        self.assertEqual(counts.total, 7)
        self.assertEqual(counts.unmapped, 1)
        self.assertEqual(counts.not_primary, 2)
        self.assertEqual(counts.qcfail_or_duplicate, 2)
        self.assertEqual(counts.low_mapq, 1)
        self.assertEqual(counts.kept, 1)

    def test_query_consumption_mismatch_rejected(self):
        rec = make_record("bad", 0, "chr1", 1, 60, "10M", "AAAAA")
        result = self.run_pipeline(SAM_HEADER + rec + "\n", "chr1\t0\t20\tt1\n")
        self.assertEqual(result.filters.invalid_record, 1)
        self.assertEqual(result.filters.kept, 0)

    def test_unknown_reference_and_out_of_bounds_rejected(self):
        rec1 = make_record("r1", 0, "chrX", 1, 60, "10M", "AAAAAAAAAA")
        rec2 = make_record("r2", 0, "chr1", 995, 60, "10M", "AAAAAAAAAA")
        result = self.run_pipeline(
            SAM_HEADER + rec1 + "\n" + rec2 + "\n", "chr1\t0\t20\tt1\n"
        )
        self.assertEqual(result.filters.invalid_record, 2)

    def test_target_validation(self):
        sam_path = write(self.tmp, "h.sam", SAM_HEADER)
        header, _ = parse_sam(sam_path)
        with self.assertRaises(InputError):
            load_targets(write(self.tmp, "b1.bed", "chrX\t0\t10\tt\n"), header)
        with self.assertRaises(InputError):
            load_targets(write(self.tmp, "b2.bed", "chr1\t0\t2000\tt\n"), header)
        with self.assertRaises(InputError):
            load_targets(
                write(self.tmp, "b3.bed", "chr1\t0\t10\tt\nchr1\t5\t15\tt\n"), header
            )

    def test_eq_and_x_count_as_coverage(self):
        rec = make_record("r1", 0, "chr1", 1, 60, "5=3X2=", "AAAAAAAAAA")
        result = self.run_pipeline(SAM_HEADER + rec + "\n", "chr1\t0\t10\tt1\n")
        self.assertEqual(result.targets[0].histogram, {0: 0, 1: 10})

    def test_paired_overlap_not_merged(self):
        # Two mates covering the same bases each contribute independently.
        rec1 = make_record("p", 99, "chr1", 1, 60, "10M", "AAAAAAAAAA")
        rec2 = make_record("p", 147, "chr1", 1, 60, "10M", "AAAAAAAAAA")
        result = self.run_pipeline(
            SAM_HEADER + rec1 + "\n" + rec2 + "\n", "chr1\t0\t10\tt1\n"
        )
        self.assertEqual(result.targets[0].histogram, {0: 0, 2: 10})


class CliTests(unittest.TestCase):
    def test_end_to_end(self):
        tmp = ROOT / "tests" / "_tmp_cli"
        tmp.mkdir(exist_ok=True)
        try:
            sam = tmp / "in.sam"
            sam.write_text(
                SAM_HEADER
                + make_record("r1", 0, "chr1", 1, 60, "5M5N5M", "AAAAAAAAAA")
                + "\n",
                encoding="utf-8",
            )
            bed = tmp / "in.bed"
            bed.write_text("chr1\t0\t15\tt1\n", encoding="utf-8")
            cfg = tmp / "cfg.json"
            cfg.write_text(json.dumps({"min_mapq": 0}), encoding="utf-8")
            out_csv = tmp / "out.csv"
            out_json = tmp / "out.json"
            proc = subprocess.run(
                [sys.executable, "-m", "sam_coverage", "--sam", str(sam),
                 "--bed", str(bed), "--config", str(cfg),
                 "--csv", str(out_csv), "--json", str(out_json)],
                cwd=ROOT, capture_output=True, text=True,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            csv_text = out_csv.read_text(encoding="utf-8")
            self.assertIn("t1", csv_text)
            summary = json.loads(out_json.read_text(encoding="utf-8"))
            self.assertEqual(summary["filters"]["kept_records"], 1)
            self.assertEqual(
                summary["junctions"],
                [{"rname": "chr1", "start": 5, "end": 10, "count": 1}],
            )
        finally:
            for path in tmp.glob("*"):
                path.unlink()
            tmp.rmdir()


if __name__ == "__main__":
    unittest.main()
