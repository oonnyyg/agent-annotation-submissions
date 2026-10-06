"""Input loading and validation."""

from __future__ import annotations

from dataclasses import dataclass
from decimal import Decimal, InvalidOperation
from typing import Any

from .errors import InputError


@dataclass(frozen=True)
class Item:
    item_id: str
    price: int  # cents
    cost: int  # cents, > 0
    salvage: int  # cents, 0 <= salvage <= cost
    shortage_penalty: int  # cents per unmet unit, >= 0
    demand: tuple[tuple[int, Decimal], ...]  # (quantity, probability), merged, sorted

    @property
    def max_demand(self) -> int:
        return self.demand[-1][0]


@dataclass(frozen=True)
class Problem:
    items: tuple[Item, ...]
    budgets: tuple[int, ...]


def _require(cond: bool, msg: str) -> None:
    if not cond:
        raise InputError(msg)


def _int_field(obj: dict[str, Any], name: str, ctx: str) -> int:
    value = obj.get(name)
    _require(isinstance(value, int) and not isinstance(value, bool),
             f"{ctx}: '{name}' must be an integer")
    return value


def _parse_probability(value: Any, ctx: str) -> Decimal:
    _require(isinstance(value, str), f"{ctx}: probability must be a decimal string")
    try:
        p = Decimal(value)
    except InvalidOperation:
        raise InputError(f"{ctx}: invalid decimal string {value!r}") from None
    _require(p.is_finite() and p >= 0, f"{ctx}: probability must be finite and >= 0")
    return p


def _parse_item(raw: Any, index: int) -> Item:
    ctx = f"items[{index}]"
    _require(isinstance(raw, dict), f"{ctx}: must be an object")
    item_id = raw.get("id")
    _require(isinstance(item_id, str) and item_id, f"{ctx}: 'id' must be a non-empty string")

    price = _int_field(raw, "price", ctx)
    cost = _int_field(raw, "cost", ctx)
    salvage = _int_field(raw, "salvage", ctx)
    penalty = _int_field(raw, "shortage_penalty", ctx)
    _require(cost > 0, f"{ctx}: cost must be positive")
    _require(price >= cost, f"{ctx}: price must be >= cost")
    _require(0 <= salvage <= cost, f"{ctx}: salvage must be between 0 and cost")
    _require(penalty >= 0, f"{ctx}: shortage_penalty must be >= 0")

    entries = raw.get("demand")
    _require(isinstance(entries, list) and entries, f"{ctx}: 'demand' must be a non-empty list")
    merged: dict[int, Decimal] = {}
    for j, entry in enumerate(entries):
        ectx = f"{ctx}.demand[{j}]"
        _require(isinstance(entry, dict), f"{ectx}: must be an object")
        qty = _int_field(entry, "quantity", ectx)
        _require(qty >= 0, f"{ectx}: quantity must be >= 0")
        prob = _parse_probability(entry.get("probability"), ectx)
        merged[qty] = merged.get(qty, Decimal(0)) + prob
    _require(sum(merged.values()) == 1, f"{ctx}: demand probabilities must sum to exactly 1")
    demand = tuple(sorted(merged.items()))
    return Item(item_id, price, cost, salvage, penalty, demand)


def parse_problem(doc: Any) -> Problem:
    _require(isinstance(doc, dict), "top level must be an object")
    raw_items = doc.get("items")
    _require(isinstance(raw_items, list) and raw_items, "'items' must be a non-empty list")
    items = tuple(_parse_item(raw, i) for i, raw in enumerate(raw_items))
    ids = [it.item_id for it in items]
    _require(len(set(ids)) == len(ids), "item ids must be unique")

    raw_budgets = doc.get("budgets")
    _require(isinstance(raw_budgets, list) and raw_budgets, "'budgets' must be a non-empty list")
    budgets: list[int] = []
    for i, b in enumerate(raw_budgets):
        _require(isinstance(b, int) and not isinstance(b, bool) and b >= 0,
                 f"budgets[{i}]: must be a non-negative integer")
        budgets.append(b)
    return Problem(items, tuple(budgets))
