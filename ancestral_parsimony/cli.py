"""Command-line interface for ancestral character reconstruction."""

from __future__ import annotations

import argparse
import sys

from .model import InputError, Problem, load_problem
from .sankoff import Reconstruction, reconstruct


def _format_number(value: float) -> str:
    return str(int(value)) if value == int(value) else repr(value)


def _render_assignments(problem: Problem, result: Reconstruction) -> list[str]:
    header = ["node", "kind", *problem.characters]
    rows = [header]
    for node_id in sorted(result.assignments):
        kind = "leaf" if node_id in problem.observations else "node"
        if node_id == problem.root.node_id:
            kind = "root"
        rows.append([node_id, kind, *result.assignments[node_id]])
    return _table(rows)


def _render_branch_costs(problem: Problem, result: Reconstruction) -> list[str]:
    header = ["parent", "child", *problem.characters, "total"]
    rows = [header]
    for branch in result.branch_costs:
        rows.append(
            [
                branch.parent_id,
                branch.child_id,
                *[_format_number(c) for c in branch.per_character],
                _format_number(branch.total),
            ]
        )
    return _table(rows)


def _render_summary(problem: Problem, result: Reconstruction) -> list[str]:
    lines = ["per-character scores:"]
    for name, score in zip(problem.characters, result.column_scores):
        lines.append(f"  {name}: {_format_number(score)}")
    lines.append(f"total score: {_format_number(result.total_score)}")
    return lines


def _table(rows: list[list[str]]) -> list[str]:
    widths = [max(len(row[i]) for row in rows) for i in range(len(rows[0]))]
    return [
        "  ".join(cell.ljust(widths[i]) for i, cell in enumerate(row)).rstrip()
        for row in rows
    ]


def _check_consistency(problem: Problem, result: Reconstruction) -> None:
    for column, score in enumerate(result.column_scores):
        branch_sum = sum(branch.per_character[column] for branch in result.branch_costs)
        if abs(branch_sum - score) > 1e-9:
            raise AssertionError(
                f"column {problem.characters[column]!r}: branch costs sum to "
                f"{branch_sum}, expected {score}"
            )
    if abs(sum(result.column_scores) - result.total_score) > 1e-9:
        raise AssertionError("column scores do not add up to the total score")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="ancestral-parsimony",
        description=(
            "Reconstruct ancestral discrete character states on a fixed rooted "
            "tree with Sankoff parsimony (minimum total change cost)."
        ),
    )
    parser.add_argument("tree", help="JSON file with states, costs, and tree")
    parser.add_argument("csv", help="CSV file with leaf observations ('?' = unknown)")
    args = parser.parse_args(argv)
    try:
        problem = load_problem(args.tree, args.csv)
        result = reconstruct(problem)
        _check_consistency(problem, result)
    except InputError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    print("node assignments:")
    print("\n".join(_render_assignments(problem, result)))
    print()
    print("branch change costs:")
    print("\n".join(_render_branch_costs(problem, result)))
    print()
    print("\n".join(_render_summary(problem, result)))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
