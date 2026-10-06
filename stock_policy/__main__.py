"""CLI: python -m stock_policy INPUT [--json-out PATH] [--csv-out PATH]."""

from __future__ import annotations

import argparse
import json
import sys

from .core import InputError, solve_budget
from .io import parse_document
from .report import items_csv, plan_json


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m stock_policy",
        description="Optimal stocking under a shared purchase budget.",
    )
    parser.add_argument("input", help="path to the input JSON document")
    parser.add_argument("--json-out",
                        help="write the plan JSON here (default: stdout)")
    parser.add_argument("--csv-out",
                        help="write the per-item metrics CSV here")
    args = parser.parse_args(argv)

    try:
        with open(args.input, "r", encoding="utf-8-sig") as handle:
            doc = json.load(handle)
    except (OSError, json.JSONDecodeError) as exc:
        print(f"error: cannot read input: {exc}", file=sys.stderr)
        return 2

    try:
        items, budgets = parse_document(doc)
    except InputError as exc:
        print(f"error: invalid input: {exc}", file=sys.stderr)
        return 2

    solutions = [(name, solve_budget(items, amount)) for name, amount in budgets]
    plan_text = plan_json(solutions)
    csv_text = items_csv(solutions)

    if args.json_out:
        with open(args.json_out, "w", encoding="utf-8", newline="") as handle:
            handle.write(plan_text)
    else:
        sys.stdout.write(plan_text)
    if args.csv_out:
        with open(args.csv_out, "w", encoding="utf-8", newline="") as handle:
            handle.write(csv_text)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
