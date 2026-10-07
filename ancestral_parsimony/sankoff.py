"""Sankoff dynamic programming with a jointly optimal backtrace."""

from __future__ import annotations

from dataclasses import dataclass

from .model import Node, Problem

INF = float("inf")


@dataclass
class BranchCost:
    parent_id: str
    child_id: str
    per_character: list[float]

    @property
    def total(self) -> float:
        return sum(self.per_character)


@dataclass
class Reconstruction:
    assignments: dict[str, list[str]]
    branch_costs: list[BranchCost]
    column_scores: list[float]
    total_score: float


def _postorder(node: Node) -> list[Node]:
    order: list[Node] = []

    def walk(current: Node) -> None:
        for child in current.children:
            walk(child)
        order.append(current)

    walk(node)
    return order


def _downpass(problem: Problem, column: int) -> dict[str, list[float]]:
    """Minimal subtree cost for every node and state, for one character."""
    n_states = len(problem.states)
    costs = problem.costs
    dp: dict[str, list[float]] = {}
    for node in _postorder(problem.root):
        if node.is_leaf:
            observed = problem.observations[node.node_id][column]
            if observed is None:
                dp[node.node_id] = [0.0] * n_states
            else:
                fixed = problem.states.index(observed)
                dp[node.node_id] = [
                    0.0 if state == fixed else INF for state in range(n_states)
                ]
        else:
            scores = []
            for state in range(n_states):
                total = 0.0
                for child in node.children:
                    child_dp = dp[child.node_id]
                    total += min(
                        costs[state][child_state] + child_dp[child_state]
                        for child_state in range(n_states)
                    )
                scores.append(total)
            dp[node.node_id] = scores
    return dp


def reconstruct(problem: Problem) -> Reconstruction:
    """Reconstruct one jointly optimal, deterministically tie-broken assignment."""
    states = problem.states
    # Tie-breaking compares state labels lexicographically, independent of the
    # order states or children happen to be listed in the input.
    tie_order = sorted(range(len(states)), key=lambda i: states[i])
    n_characters = len(problem.characters)
    dps = [_downpass(problem, column) for column in range(n_characters)]

    assignments: dict[str, list[str]] = {}

    def assign(node: Node, parent_states: list[int]) -> None:
        chosen: list[int] = []
        for column in range(n_characters):
            dp = dps[column][node.node_id]
            row = problem.costs[parent_states[column]]
            # min() keeps the first minimum, and tie_order is sorted by label,
            # so ties resolve to the lexicographically smallest state.
            best = min(tie_order, key=lambda s: row[s] + dp[s])
            chosen.append(best)
        assignments[node.node_id] = [states[s] for s in chosen]
        for child in sorted(node.children, key=lambda c: c.node_id):
            assign(child, chosen)

    # Root: lexicographically smallest optimal state per character.
    root_chosen: list[int] = []
    for column in range(n_characters):
        dp = dps[column][problem.root.node_id]
        best = min(tie_order, key=lambda s: dp[s])
        root_chosen.append(best)
    assignments[problem.root.node_id] = [states[s] for s in root_chosen]
    for child in sorted(problem.root.children, key=lambda c: c.node_id):
        assign(child, root_chosen)

    branch_costs = []

    def collect(node: Node) -> None:
        for child in sorted(node.children, key=lambda c: c.node_id):
            per_character = [
                problem.costs[states.index(assignments[node.node_id][col])][
                    states.index(assignments[child.node_id][col])
                ]
                for col in range(n_characters)
            ]
            branch_costs.append(BranchCost(node.node_id, child.node_id, per_character))
            collect(child)

    collect(problem.root)

    column_scores = [
        min(dps[column][problem.root.node_id]) for column in range(n_characters)
    ]
    return Reconstruction(
        assignments=assignments,
        branch_costs=branch_costs,
        column_scores=column_scores,
        total_score=sum(column_scores),
    )
