"""Command line interface: python -m uncertainty_budget INPUT [options]."""

import argparse
import json
import sys

from .errors import BudgetError
from .report import budget_csv, compute, report_dict


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="python -m uncertainty_budget",
        description=(
            "First-order uncertainty budget with full input correlations. "
            "Reads a JSON spec, writes a JSON report and a budget CSV."
        ),
    )
    parser.add_argument("input",
                        help="path to the input JSON spec ('-' for stdin)")
    parser.add_argument("--json", dest="json_out", default=None,
                        help="output JSON path (default: stdout)")
    parser.add_argument("--csv", dest="csv_out", default=None,
                        help="output budget CSV path "
                             "(default: <json>.budget.csv when --json is set)")
    args = parser.parse_args(argv)

    try:
        if args.input == "-":
            spec = json.load(sys.stdin)
        else:
            with open(args.input, "r", encoding="utf-8") as fh:
                spec = json.load(fh)
    except (OSError, json.JSONDecodeError) as exc:
        print(f"error: cannot read input JSON: {exc}", file=sys.stderr)
        return 2

    try:
        quantities, correlation, budgets, result_cov, corr = compute(spec)
    except BudgetError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    report = report_dict(quantities, correlation, budgets, result_cov, corr)
    json_text = json.dumps(report, indent=2) + "\n"
    csv_text = budget_csv(quantities, budgets)

    if args.json_out:
        with open(args.json_out, "w", encoding="utf-8") as fh:
            fh.write(json_text)
    else:
        sys.stdout.write(json_text)

    csv_out = args.csv_out
    if csv_out is None and args.json_out:
        csv_out = args.json_out + ".budget.csv"
    if csv_out:
        with open(csv_out, "w", encoding="utf-8", newline="") as fh:
            fh.write(csv_text)
    elif args.json_out is None:
        sys.stdout.write(csv_text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
