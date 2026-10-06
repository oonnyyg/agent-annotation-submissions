"""Command line entry: python -m spin_echo CONFIG [--csv PATH] [--summary PATH]."""

from __future__ import annotations

import argparse
import json
import os
import sys

from .config import load_experiment
from .errors import ConfigError
from .report import build_summary, write_csv
from .simulate import run


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m spin_echo",
        description="Local spin-echo simulator (no hardware, stdlib only).",
    )
    parser.add_argument("config", help="path to the JSON experiment configuration")
    parser.add_argument("--csv", default="signal.csv", help="CSV output path")
    parser.add_argument("--summary", default="summary.json", help="JSON summary path")
    args = parser.parse_args(argv)

    try:
        with open(args.config, "r", encoding="utf-8") as fh:
            data = json.load(fh)
    except OSError as exc:
        print(f"error: cannot read config: {exc}", file=sys.stderr)
        return 2
    except json.JSONDecodeError as exc:
        print(f"error: invalid JSON in {args.config}: {exc}", file=sys.stderr)
        return 2

    try:
        experiment = load_experiment(data)
    except ConfigError as exc:
        print(f"error: invalid configuration: {exc}", file=sys.stderr)
        return 2

    results = run(experiment)
    for path in (args.csv, args.summary):
        parent = os.path.dirname(os.path.abspath(path))
        os.makedirs(parent, exist_ok=True)
    write_csv(args.csv, experiment, results)
    summary = build_summary(experiment, results)
    with open(args.summary, "w", encoding="utf-8") as fh:
        json.dump(summary, fh, indent=2, allow_nan=False)
        fh.write("\n")
    print(f"wrote {args.csv} and {args.summary}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
