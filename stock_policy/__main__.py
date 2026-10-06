"""Command line entry point: python -m stock_policy INPUT [--json PATH] [--csv PATH]."""

from __future__ import annotations

import argparse
import json
import sys

from .errors import InputError
from .model import parse_problem
from .report import solutions_to_csv, solutions_to_json
from .solver import solve


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="stock_policy",
        description="Optimal stocking under a shared purchasing budget.",
    )
    parser.add_argument("input", help="path to the problem JSON file")
    parser.add_argument("--json", dest="json_path",
                        help="write the optimal-stocking JSON here (default: stdout)")
    parser.add_argument("--csv", dest="csv_path",
                        help="write the per-item profit CSV here")
    args = parser.parse_args(argv)

    try:
        with open(args.input, "r", encoding="utf-8") as fh:
            doc = json.load(fh)
        problem = parse_problem(doc)
    except OSError as exc:
        print(f"error: cannot read input: {exc}", file=sys.stderr)
        return 2
    except json.JSONDecodeError as exc:
        print(f"error: invalid JSON: {exc}", file=sys.stderr)
        return 2
    except InputError as exc:
        print(f"error: invalid input: {exc}", file=sys.stderr)
        return 2

    solutions = [solve(problem.items, budget) for budget in problem.budgets]

    json_text = solutions_to_json(solutions)
    if args.json_path:
        with open(args.json_path, "w", encoding="utf-8") as fh:
            fh.write(json_text)
    else:
        sys.stdout.write(json_text)
    if args.csv_path:
        with open(args.csv_path, "w", encoding="utf-8", newline="") as fh:
            fh.write(solutions_to_csv(solutions))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
