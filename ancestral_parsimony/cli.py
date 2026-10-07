"""Command line interface for ancestral character reconstruction."""

from __future__ import annotations

import argparse
import sys

from .data import load_costs, load_traits, validate_observations
from .errors import InputError
from .sankoff import sankoff_column
from .tree import load_tree


def _fmt(value):
    return f"{value:g}" if isinstance(value, float) else str(value)


def run(argv=None):
    parser = argparse.ArgumentParser(
        prog="ancestral-parsimony",
        description=(
            "Reconstruct ancestral character states on a fixed rooted tree "
            "by minimizing total transition cost (Sankoff parsimony)."
        ),
    )
    parser.add_argument("--tree", required=True, help="rooted tree JSON file")
    parser.add_argument("--traits", required=True, help="leaf trait CSV file")
    parser.add_argument("--costs", required=True, help="states and cost matrix JSON file")
    args = parser.parse_args(argv)

    try:
        tree = load_tree(args.tree)
        columns, traits = load_traits(args.traits, tree.leaves)
        states, matrix = load_costs(args.costs)
        validate_observations(traits, columns, states)
    except InputError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    assignments = {}
    column_totals = {}
    branch_costs = {}
    for col_idx, col in enumerate(columns):
        observations = {leaf: traits[leaf][col_idx] for leaf in tree.leaves}
        assignment, total, bcosts = sankoff_column(tree, observations, states, matrix)
        assignments[col] = assignment
        column_totals[col] = total
        branch_costs[col] = bcosts

    nodes = tree.preorder_children_sorted()

    print("Node state table:")
    print(",".join(["node"] + columns))
    for node in nodes:
        values = [assignments[col][node] for col in columns]
        print(",".join([str(node)] + values))

    print()
    print("Branch change costs (parent,child):")
    print(",".join(["parent", "child"] + columns + ["total"]))
    for node in nodes:
        for child in sorted(tree.children[node]):
            per_col = [branch_costs[col][(node, child)] for col in columns]
            row = [str(node), str(child)] + [_fmt(c) for c in per_col]
            row.append(_fmt(sum(per_col)))
            print(",".join(row))

    print()
    print("Score summary:")
    for col in columns:
        print(f"{col}: {_fmt(column_totals[col])}")
    print(f"total: {_fmt(sum(column_totals.values()))}")
    return 0


def main():
    sys.exit(run())


if __name__ == "__main__":
    main()
