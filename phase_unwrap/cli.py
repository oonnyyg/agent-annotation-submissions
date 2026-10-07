"""Command line interface for quality-guided phase unwrapping."""

from __future__ import annotations

import argparse
import csv
import json
import math
import sys
from dataclasses import asdict

from .core import UnwrapResult, unwrap
from .grid import load_grids


def _write_float_grid(path: str, grid: list[list[float]]) -> None:
    with open(path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        for row in grid:
            writer.writerow(
                ["" if math.isnan(v) else repr(v) for v in row]
            )


def _write_int_grid(path: str, grid: list[list[int]]) -> None:
    with open(path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        for row in grid:
            writer.writerow(row)


def _diagnostics(result: UnwrapResult) -> dict:
    return {
        "rows": result.rows,
        "cols": result.cols,
        "num_regions": len(result.roots),
        "roots": [{"region": i, "row": r, "col": c}
                  for i, (r, c) in enumerate(result.roots)],
        "num_conflicts": len(result.conflicts),
        "consistent": not result.conflicts,
        "conflicts": [asdict(c) for c in result.conflicts],
        "note": (
            "Forest construction succeeding does not imply all adjacency "
            "constraints are satisfiable; see 'conflicts'. Absolute cycle "
            "counts between different regions are undetermined."
        ),
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="phase-unwrap",
        description="Quality-guided 2D phase unwrapping (standard library).",
    )
    parser.add_argument("--phase", required=True, help="wrapped phase CSV")
    parser.add_argument("--quality", required=True, help="quality CSV")
    parser.add_argument("--mask", required=True,
                        help="mask CSV (0 = valid, non-zero = masked)")
    parser.add_argument("--out-unwrapped", default="unwrapped.csv")
    parser.add_argument("--out-regions", default="regions.csv")
    parser.add_argument("--out-diagnostics", default="diagnostics.json")
    args = parser.parse_args(argv)

    try:
        grid = load_grids(args.phase, args.quality, args.mask)
    except (ValueError, OSError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    result = unwrap(grid)
    _write_float_grid(args.out_unwrapped, result.unwrapped)
    _write_int_grid(args.out_regions, result.region)
    with open(args.out_diagnostics, "w", encoding="utf-8") as handle:
        json.dump(_diagnostics(result), handle, indent=2)
        handle.write("\n")

    print(f"regions: {len(result.roots)}, "
          f"conflicts: {len(result.conflicts)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
