"""Expected profit evaluation for a single item and stock level."""

from __future__ import annotations

from dataclasses import dataclass
from decimal import Decimal

from .model import Item


@dataclass(frozen=True)
class Option:
    quantity: int
    expected_sales: Decimal  # units
    expected_leftover: Decimal  # units
    expected_shortage: Decimal  # units
    expected_profit: Decimal  # cents


def evaluate(item: Item, quantity: int) -> Option:
    sales = Decimal(0)
    leftover = Decimal(0)
    shortage = Decimal(0)
    for demand, prob in item.demand:
        sold = min(quantity, demand)
        sales += prob * sold
        leftover += prob * (quantity - sold)
        shortage += prob * (demand - sold)
    profit = (
        sales * item.price
        + leftover * item.salvage
        - Decimal(quantity) * item.cost
        - shortage * item.shortage_penalty
    )
    return Option(quantity, sales, leftover, shortage, profit)


def options_for(item: Item) -> list[Option]:
    """All stock levels from 0 up to the item's maximum demand."""
    return [evaluate(item, q) for q in range(item.max_demand + 1)]
