"""CLI entry point: python -m sam_coverage."""

from __future__ import annotations

import argparse
import sys

from .io import InputError
from .pipeline import run


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m sam_coverage",
        description=(
            "Per-target coverage QC from SAM alignments. Only truly "
            "aligned reference bases (CIGAR M/=/X) count as coverage; "
            "deletions and intron skips (N) do not."
        ),
    )
    parser.add_argument("--sam", required=True, help="input SAM file")
    parser.add_argument("--bed", required=True, help="BED4 target file "
                        "(0-based, half-open)")
    parser.add_argument("--config", required=True, help="JSON config "
                        "(e.g. {\"min_mapq\": 20})")
    parser.add_argument("--out-csv", required=True, help="per-target CSV output")
    parser.add_argument("--out-summary", required=True,
                        help="junction + filtering summary JSON output")
    args = parser.parse_args(argv)
    try:
        summary = run(args.sam, args.bed, args.config,
                      args.out_csv, args.out_summary)
    except InputError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    rec = summary["records"]
    print(f"kept {rec['kept']} of {rec['total']} records; "
          f"junctions: {len(summary['junctions'])}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
