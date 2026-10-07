"""Command-line interface for the discrete mass transport tool."""
from __future__ import annotations

import argparse
import sys

from .io import load_problem
from .model import Problem, TransportError
from .solver import InfeasibleError, Solution, solve


def _conservation_report(problem: Problem, solution: Solution) -> list[str]:
    sent = {s.site_id: 0 for s in problem.sources}
    received = {t.site_id: 0 for t in problem.targets}
    for alloc in solution.allocations:
        sent[alloc.source_id] += alloc.amount
        received[alloc.target_id] += alloc.amount
    lines = ["conservation check:"]
    ok = True
    for site in problem.sources:
        match = sent[site.site_id] == site.mass
        ok &= match
        lines.append(
            f"  source {site.site_id}: sent {sent[site.site_id]} / required {site.mass}"
            + ("" if match else "  MISMATCH")
        )
    for site in problem.targets:
        match = received[site.site_id] == site.mass
        ok &= match
        lines.append(
            f"  target {site.site_id}: received {received[site.site_id]} / required {site.mass}"
            + ("" if match else "  MISMATCH")
        )
    lines.append("  status: " + ("OK" if ok else "FAILED"))
    return lines


def format_report(problem: Problem, solution: Solution) -> str:
    lines = [
        f"cost metric: {problem.cost_metric}",
        f"total mass: {solution.total_mass}",
        "transport plan (source -> target: amount @ unit cost):",
    ]
    if solution.allocations:
        for alloc in solution.allocations:
            lines.append(
                f"  {alloc.source_id} -> {alloc.target_id}: "
                f"{alloc.amount} @ {alloc.unit_cost} = {alloc.cost}"
            )
    else:
        lines.append("  (no routes carry mass)")
    lines.append(f"total cost: {solution.total_cost}")
    lines.append(f"average cost per unit mass: {solution.average_cost:.6f}")
    lines.append("tie-break rule: among equally optimal plans, routes from "
                 "earlier-listed sources to earlier-listed targets are preferred")
    lines.extend(_conservation_report(problem, solution))
    return "\n".join(lines)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="mass-transport",
        description="Compute a minimum-cost transport plan between two discrete distributions.",
    )
    parser.add_argument("sources", help="CSV file with columns id,x,y,mass")
    parser.add_argument("targets", help="CSV file with columns id,x,y,mass")
    parser.add_argument("config", help="JSON config: cost_metric, disabled pairs")
    args = parser.parse_args(argv)

    try:
        problem = load_problem(args.sources, args.targets, args.config)
    except TransportError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    try:
        solution = solve(problem)
    except InfeasibleError as exc:
        print(f"infeasible: {exc}", file=sys.stderr)
        return 1
    print(format_report(problem, solution))
    return 0


if __name__ == "__main__":
    sys.exit(main())
