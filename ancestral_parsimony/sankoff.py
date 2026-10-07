"""Sankoff dynamic programming and joint optimal state assignment."""

from __future__ import annotations

import math

from .data import UNKNOWN
from .tree import Tree


def _lex_order(states):
    """State indices sorted by lexicographic state name (tie-break order)."""
    return sorted(range(len(states)), key=lambda i: states[i])


def sankoff_column(tree: Tree, observations, states, matrix):
    """Run Sankoff DP for one trait column.

    observations: {leaf_id: state string or UNKNOWN}.
    Returns (assignment {node_id: state}, total_cost, branch_costs).
    """
    n = len(states)
    index = {s: i for i, s in enumerate(states)}
    lex = _lex_order(states)

    cost = {}
    for node in tree.postorder():
        if not tree.children[node]:
            obs = observations[node]
            if obs == UNKNOWN:
                cost[node] = [0.0] * n
            else:
                vec = [math.inf] * n
                vec[index[obs]] = 0.0
                cost[node] = vec
        else:
            vec = []
            for i in range(n):
                total = 0.0
                for child in tree.children[node]:
                    total += min(cost[child][j] + matrix[i][j] for j in range(n))
                vec.append(total)
            cost[node] = vec

    # Root: lexicographically smallest state among optima.
    root_state = min(lex, key=lambda i: cost[tree.root][i])
    total_cost = cost[tree.root][root_state]

    # Top-down joint backtrack: given the parent's chosen state, each child
    # takes the lexicographically smallest state attaining the constrained
    # minimum. This yields one jointly optimal assignment, not independent
    # per-node minima.
    assignment = {tree.root: root_state}
    for node in tree.preorder_children_sorted():
        parent_state = assignment[node]
        for child in sorted(tree.children[node]):
            best = min(
                lex,
                key=lambda j: cost[child][j] + matrix[parent_state][j],
            )
            assignment[child] = best

    branch_costs = {}
    for node in tree.preorder_children_sorted():
        for child in sorted(tree.children[node]):
            branch_costs[(node, child)] = matrix[assignment[node]][assignment[child]]

    # Integrity check: branch costs must re-sum to the DP optimum.
    assert abs(sum(branch_costs.values()) - total_cost) < 1e-9
    return (
        {node: states[i] for node, i in assignment.items()},
        total_cost,
        branch_costs,
    )
