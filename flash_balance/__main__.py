"""Command line interface: python -m flash_balance."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from .cascade import run_cascade
from .errors import FlashBalanceError
from .io import parse_config, render_streams_csv, render_summary_json


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="flash_balance",
        description="Two-stage flash material balance with fixed K-values.",
    )
    parser.add_argument("config", help="path to the JSON configuration file")
    parser.add_argument("--streams", help="output path for the streams CSV")
    parser.add_argument("--summary", help="output path for the summary JSON")
    args = parser.parse_args(argv)

    try:
        text = Path(args.config).read_text(encoding="utf-8")
        feeds, cases, tolerance = parse_config(text)
        results = {}
        for case in cases:
            stage_k = [s["k_values"] for s in case["stages"]]
            results[case["name"]] = run_cascade(feeds, stage_k, tolerance)
        streams_csv = render_streams_csv(results)
        summary_json = render_summary_json(results)
    except FlashBalanceError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    except OSError as exc:
        print(f"error: cannot read configuration: {exc}", file=sys.stderr)
        return 1

    if args.streams:
        Path(args.streams).write_text(streams_csv, encoding="utf-8")
    else:
        sys.stdout.write(streams_csv)
    if args.summary:
        Path(args.summary).write_text(summary_json, encoding="utf-8")
    elif args.streams:
        sys.stdout.write(summary_json)
    return 0


if __name__ == "__main__":
    sys.exit(main())
