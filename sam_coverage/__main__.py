"""Command line interface: python -m sam_coverage."""

from __future__ import annotations

import argparse
import csv
import json
import sys

from .coverage import compute_coverage
from .model import InputError, load_config, load_targets, parse_sam


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="python -m sam_coverage",
        description=(
            "Compute target-region coverage QC from a SAM file, counting "
            "only truly aligned reference bases (M/=/X)."
        ),
    )
    parser.add_argument("--sam", required=True, help="input SAM file")
    parser.add_argument("--bed", required=True, help="BED target regions (0-based half-open)")
    parser.add_argument("--config", required=True, help="JSON config (e.g. min_mapq)")
    parser.add_argument("--csv", required=True, help="output per-target CSV path")
    parser.add_argument("--json", required=True, help="output summary JSON path")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        config = load_config(args.config)
        header, records = parse_sam(args.sam)
        targets = load_targets(args.bed, header)
        result = compute_coverage(records, targets, header, config.min_mapq)
    except (InputError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    with open(args.csv, "w", encoding="utf-8", newline="") as handle:
        writer = csv.writer(handle)
        writer.writerow(
            ["target_id", "rname", "start", "end", "length",
             "mean_depth", "fraction_covered_1x", "depth_histogram"]
        )
        for entry in result.targets:
            target = entry.target
            writer.writerow([
                target.target_id,
                target.rname,
                target.start,
                target.end,
                target.end - target.start,
                f"{entry.mean_depth:.6f}",
                f"{entry.fraction_covered_1x:.6f}",
                json.dumps({str(k): v for k, v in entry.histogram.items()}),
            ])

    summary = {
        "filters": {
            "total_records": result.filters.total,
            "skipped_unmapped": result.filters.unmapped,
            "skipped_not_primary": result.filters.not_primary,
            "skipped_qcfail_or_duplicate": result.filters.qcfail_or_duplicate,
            "skipped_low_mapq": result.filters.low_mapq,
            "skipped_invalid_record": result.filters.invalid_record,
            "kept_records": result.filters.kept,
        },
        "junctions": [
            {"rname": j.rname, "start": j.start, "end": j.end, "count": j.count}
            for j in result.junctions
        ],
    }
    with open(args.json, "w", encoding="utf-8") as handle:
        json.dump(summary, handle, indent=2)
        handle.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
