"""Core model: exact expected profit per stock level and budgeted combination."""

from __future__ import annotations

from dataclasses import dataclass
from decimal import Decimal


class InputError(ValueError):
    """Raised when the input document is invalid."""


@dataclass(frozen=True)
class Item:
    item_id: str
    price_cents: int
    cost_cents: int
    salvage_cents: int
    shortage_penalty_cents: int
    demand: tuple[tuple[int, Decimal], ...]

    @property
    def max_demand(self) -> int:
        return max(q for q, _ in self.demand)


@dataclass(frozen=True)
class Metrics:
    quantity: int
    expected_sales: Decimal
    expected_leftover: Decimal
    expected_shortage: Decimal
    sales_revenue_cents: Decimal
    salvage_revenue_cents: Decimal
    purchase_cost_cents: Decimal
    shortage_penalty_cents: Decimal
    expected_profit_cents: Decimal


def expected_metrics(item: Item, quantity: int) -> Metrics:
    """Exact expected metrics for stocking ``quantity`` units of ``item``."""
    if quantity < 0:
        raise ValueError("quantity must be non-negative")
    sales = Decimal(0)
    leftover = Decimal(0)
    shortage = Decimal(0)
    for demand_qty, prob in item.demand:
        sold = min(quantity, demand_qty)
        sales += prob * sold
        leftover += prob * (quantity - sold)
        shortage += prob * (demand_qty - sold)
    sales_revenue = sales * item.price_cents
    salvage_revenue = leftover * item.salvage_cents
    purchase_cost = Decimal(quantity) * item.cost_cents
    penalty = shortage * item.shortage_penalty_cents
    profit = sales_revenue + salvage_revenue - purchase_cost - penalty
    return Metrics(
        quantity=quantity,
        expected_sales=sales,
        expected_leftover=leftover,
        expected_shortage=shortage,
        sales_revenue_cents=sales_revenue,
        salvage_revenue_cents=salvage_revenue,
        purchase_cost_cents=purchase_cost,
        shortage_penalty_cents=penalty,
        expected_profit_cents=profit,
    )


def item_options(item: Item) -> list[Metrics]:
    """Expected metrics for stock levels 0..max demand (higher is dominated)."""
    return [expected_metrics(item, q) for q in range(item.max_demand + 1)]


@dataclass(frozen=True)
class BudgetSolution:
    budget_cents: int
    total_cost_cents: int
    total_expected_profit_cents: Decimal
    # (item, chosen metrics) pairs ordered by item id
    choices: tuple[tuple[Item, Metrics], ...]


def _better_state(a: tuple[Decimal, tuple[int, ...]],
                  b: tuple[Decimal, tuple[int, ...]]) -> tuple:
    """Pick the better DP state: higher profit, then lex-smaller vector."""
    if a[0] != b[0]:
        return a if a[0] > b[0] else b
    return a if a[1] <= b[1] else b


def solve_budget(items: list[Item], budget_cents: int) -> BudgetSolution:
    """Global optimum: max total expected profit with cost <= budget.

    Exact multiple-choice knapsack DP over total purchase cost. Ties on
    profit prefer less capital tied up, then the lexicographically smaller
    quantity vector ordered by item id. Result is independent of the input
    order of items.
    """
    if budget_cents < 0:
        raise InputError("budget_cents must be non-negative")
    ordered = sorted(items, key=lambda it: it.item_id)
    options = [item_options(it) for it in ordered]
    # states: total cost -> (profit, quantity vector)
    states: dict[int, tuple[Decimal, tuple[int, ...]]] = {0: (Decimal(0), ())}
    for idx, opts in enumerate(options):
        nxt: dict[int, tuple[Decimal, tuple[int, ...]]] = {}
        for cost, (profit, vec) in states.items():
            for opt in opts:
                new_cost = cost + opt.quantity * ordered[idx].cost_cents
                if new_cost > budget_cents:
                    continue
                cand = (profit + opt.expected_profit_cents, vec + (opt.quantity,))
                prev = nxt.get(new_cost)
                if prev is None:
                    nxt[new_cost] = cand
                else:
                    nxt[new_cost] = _better_state(prev, cand)
        states = nxt
    best_cost = 0
    best = states[0]
    for cost, state in states.items():
        if state[0] != best[0]:
            if state[0] > best[0]:
                best_cost, best = cost, state
        elif cost < best_cost or (cost == best_cost and state[1] < best[1]):
            best_cost, best = cost, state
    choices = tuple(
        (it, options[i][q]) for i, (it, q) in enumerate(zip(ordered, best[1]))
    )
    return BudgetSolution(
        budget_cents=budget_cents,
        total_cost_cents=best_cost,
        total_expected_profit_cents=best[0],
        choices=choices,
    )
