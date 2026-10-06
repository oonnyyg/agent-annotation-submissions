"""Command-line interface: python -m uncertainty_budget"""

from __future__ import annotations

import argparse
import csv
import json
import sys

from .errors import UncertaintyBudgetError
from .model import load_correlation, load_inputs, load_results
from .propagate import compute


def _fmt(x) -> str:
    if x is None:
        return ""
    return f"{x:.12g}"


def write_budget_csv(output: dict, path: str) -> None:
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow([
            "result", "term_type", "input_1", "input_2",
            "variance_contribution", "result_variance", "result_std_uncertainty",
        ])
        for res in output["results"]:
            for term in res["budget"]:
                inputs = term["inputs"]
                writer.writerow([
                    res["name"],
                    term["type"],
                    inputs[0],
                    inputs[1] if len(inputs) > 1 else "",
                    _fmt(term["contribution"]),
                    _fmt(res["variance"]),
                    _fmt(res["std_uncertainty"]),
                ])


def build_json_output(output: dict) -> dict:
    return {
        "inputs": output["inputs"],
        "input_correlation": output["correlation"],
        "results": [
            {
                "name": r["name"],
                "expression": r["expression"],
                "value": r["value"],
                "std_uncertainty": r["std_uncertainty"],
                "variance": r["variance"],
                "budget": r["budget"],
            }
            for r in output["results"]
        ],
        "result_correlation": output["result_correlation"],
    }


def main(argv=None) -> int:
    ap = argparse.ArgumentParser(
        prog="uncertainty_budget",
        description="First-order uncertainty budget with correlated inputs.",
    )
    ap.add_argument("spec", help="path to the JSON specification ('-' for stdin)")
    ap.add_argument("--json", dest="json_path", default=None,
                    help="write JSON results to this path (default: stdout)")
    ap.add_argument("--csv", dest="csv_path", default=None,
                    help="write the variance budget table as CSV to this path")
    args = ap.parse_args(argv)

    try:
        if args.spec == "-":
            spec = json.load(sys.stdin)
        else:
            with open(args.spec, "r", encoding="utf-8") as fh:
                spec = json.load(fh)
        if not isinstance(spec, dict):
            raise UncertaintyBudgetError("top-level JSON value must be an object")
        inputs = load_inputs(spec)
        corr = load_correlation(spec, len(inputs))
        results = load_results(spec, {x["name"] for x in inputs})
        output = compute(inputs, corr, results)
    except UncertaintyBudgetError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    except OSError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    except json.JSONDecodeError as exc:
        print(f"error: invalid JSON: {exc}", file=sys.stderr)
        return 2

    payload = json.dumps(build_json_output(output), indent=2, allow_nan=False)
    if args.json_path:
        with open(args.json_path, "w", encoding="utf-8") as fh:
            fh.write(payload + "\n")
    else:
        print(payload)
    if args.csv_path:
        write_budget_csv(output, args.csv_path)
    return 0
