"""Command line interface: python -m enclosure_radiation config.json."""

from __future__ import annotations

import argparse
import json
import sys

from .model import InputError, load_enclosure
from .report import details_json, surfaces_csv
from .solver import solve_case


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m enclosure_radiation",
        description="Gray diffuse enclosure radiation calculator.")
    parser.add_argument("config", help="JSON configuration file")
    parser.add_argument("--csv", default="surfaces.csv",
                        help="output CSV path (default: surfaces.csv)")
    parser.add_argument("--json", default="exchange_details.json",
                        help="output JSON path (default: exchange_details.json)")
    args = parser.parse_args(argv)

    try:
        with open(args.config, "r", encoding="utf-8") as handle:
            data = json.load(handle)
    except OSError as exc:
        print(f"error: cannot read {args.config}: {exc}", file=sys.stderr)
        return 2
    except json.JSONDecodeError as exc:
        print(f"error: invalid JSON in {args.config}: {exc}", file=sys.stderr)
        return 2

    try:
        enclosure = load_enclosure(data)
    except InputError as exc:
        print(f"error: invalid configuration: {exc}", file=sys.stderr)
        return 2

    solved = []
    failed = []
    for case in enclosure.cases:
        try:
            solved.append(solve_case(enclosure, case))
        except InputError as exc:
            failed.append({"name": case.name, "status": "failed",
                           "error": str(exc)})
            print(f"error: case {case.name!r} failed: {exc}", file=sys.stderr)

    with open(args.csv, "w", encoding="utf-8", newline="") as handle:
        handle.write(surfaces_csv(solved))
    with open(args.json, "w", encoding="utf-8") as handle:
        handle.write(details_json(solved, failed))

    print(f"wrote {args.csv} and {args.json} "
          f"({len(solved)} solved, {len(failed)} failed)")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
