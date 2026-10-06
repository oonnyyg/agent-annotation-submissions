"""Global optimization over a shared purchasing budget."""

from __future__ import annotations

from dataclasses import dataclass
from decimal import Decimal

from .model import Item
from .profit import Option, options_for


@dataclass(frozen=True)
class Solution:
    budget: int
    total_cost: int
    expected_profit: Decimal
    choices: tuple[tuple[Item, Option], ...]  # ordered by item id


def _better(a: tuple[Decimal, tuple[int, ...]], cost_a: int,
            b: tuple[Decimal, tuple[int, ...]], cost_b: int) -> bool:
    """True if candidate a (profit, quantities at cost_a) beats b."""
    if a[0] != b[0]:
        return a[0] > b[0]
    if cost_a != cost_b:
        return cost_a < cost_b
    return a[1] < b[1]


def solve(items: tuple[Item, ...], budget: int) -> Solution:
    ordered = sorted(items, key=lambda it: it.item_id)
    per_item = [options_for(it) for it in ordered]

    # dp[cost] = (profit, quantities tuple in sorted-id order)
    dp: dict[int, tuple[Decimal, tuple[int, ...]]] = {0: (Decimal(0), ())}
    for item, options in zip(ordered, per_item):
        nxt: dict[int, tuple[Decimal, tuple[int, ...]]] = {}
        for cost, (profit, qtys) in dp.items():
            for opt in options:
                new_cost = cost + opt.quantity * item.cost
                if new_cost > budget:
                    continue
                new_profit = profit + opt.expected_profit
                new_qtys = qtys + (opt.quantity,)
                old = nxt.get(new_cost)
                if old is None or _better((new_profit, new_qtys), new_cost, old, new_cost):
                    nxt[new_cost] = (new_profit, new_qtys)
        dp = nxt

    best_cost = 0
    best = dp[0]
    for cost, state in dp.items():
        if cost <= budget and _better(state, cost, best, best_cost):
            best, best_cost = state, cost

    choices = tuple(
        (item, options[qty])
        for item, options, qty in zip(ordered, per_item, best[1])
    )
    return Solution(budget, best_cost, best[0], choices)
