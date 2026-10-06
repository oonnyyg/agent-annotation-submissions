"""CLI entry point: python -m allele_drift <input.json> [--outdir DIR]."""

from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from .model import run_scenario
from .output import write_scenario_csv, build_summary


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m allele_drift",
        description="Exact finite haploid allele drift with selection and two-way mutation.",
    )
    parser.add_argument("input", help="Path to JSON config with a 'scenarios' list.")
    parser.add_argument(
        "--outdir",
        default=None,
        help="Output directory (default: alongside the input file).",
    )
    args = parser.parse_args(argv)

    input_path = Path(args.input)
    try:
        config = json.loads(input_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        print(f"error: cannot read config: {exc}", file=sys.stderr)
        return 2

    scenarios = config.get("scenarios")
    if not isinstance(scenarios, list) or not scenarios:
        print("error: config must contain a non-empty 'scenarios' list", file=sys.stderr)
        return 2

    outdir = Path(args.outdir) if args.outdir else input_path.parent
    outdir.mkdir(parents=True, exist_ok=True)

    summaries = []
    for index, raw in enumerate(scenarios):
        try:
            result = run_scenario(raw)
        except ValueError as exc:
            print(f"error: scenario {index}: {exc}", file=sys.stderr)
            return 2
        csv_path = outdir / f"{result.name}_generations.csv"
        write_scenario_csv(csv_path, result)
        summaries.append(build_summary(result, csv_path))
        print(f"wrote {csv_path}")

    summary_path = outdir / "summary.json"
    summary_path.write_text(
        json.dumps({"scenarios": summaries}, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )
    print(f"wrote {summary_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
