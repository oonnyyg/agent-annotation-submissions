"""Command-line interface for the discrete mass transport tool."""

from __future__ import annotations

import argparse
import sys

from .model import InputError, load_problem
from .solver import solve


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="mass-transport",
        description=(
            "Move mass between two discrete distributions at minimum total "
            "cost and print the per-path transport plan."
        ),
    )
    parser.add_argument("sources", help="CSV of source locations (id,x,y,mass)")
    parser.add_argument("targets", help="CSV of target locations (id,x,y,mass)")
    parser.add_argument("config", help="JSON config (metric, disabled pairs)")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        problem = load_problem(args.sources, args.targets, args.config)
    except InputError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    except OSError as exc:
        print(f"error: cannot read input: {exc}", file=sys.stderr)
        return 2

    result = solve(problem)
    total = result.total_mass
    print(f"metric: {problem.metric}")
    print(f"total mass: {total}")
    if not result.feasible:
        print(
            "INFEASIBLE: the disabled paths prevent moving all mass; "
            "no complete transport plan exists."
        )
        return 1

    print("plan (source -> target: amount @ unit cost):")
    for entry in result.flows:
        print(
            f"  {entry.source} -> {entry.target}: "
            f"{entry.amount} @ {entry.unit_cost} = {entry.cost}"
        )
    print(f"total cost: {result.total_cost}")
    print(f"average cost per unit mass: {result.average_cost:.6f}")

    out_amount: dict[str, int] = {}
    in_amount: dict[str, int] = {}
    for entry in result.flows:
        out_amount[entry.source] = out_amount.get(entry.source, 0) + entry.amount
        in_amount[entry.target] = in_amount.get(entry.target, 0) + entry.amount
    for loc in problem.sources.locations:
        assert out_amount.get(loc.ident, 0) == loc.mass, loc.ident
    for loc in problem.targets.locations:
        assert in_amount.get(loc.ident, 0) == loc.mass, loc.ident
    print("conservation check: OK (every source and target matches its mass)")
    print(
        "tie-breaking: deterministic; among optimal plans the "
        "lexicographically smallest by (source id, target id) is chosen."
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
