"""Parsing and validation of the input JSON document."""

from __future__ import annotations

from decimal import Decimal, InvalidOperation

from .core import InputError, Item


def _require(cond: bool, message: str) -> None:
    if not cond:
        raise InputError(message)


def _parse_int(value, field: str) -> int:
    _require(isinstance(value, int) and not isinstance(value, bool),
             f"{field} must be an integer")
    return value


def _parse_probability(value, field: str) -> Decimal:
    _require(isinstance(value, str), f"{field} must be a decimal string")
    try:
        prob = Decimal(value)
    except InvalidOperation:
        raise InputError(f"{field} is not a valid decimal: {value!r}")
    _require(prob.is_finite(), f"{field} must be finite")
    _require(prob >= 0, f"{field} must be non-negative")
    return prob


def _parse_demand(raw, item_id: str) -> tuple[tuple[int, Decimal], ...]:
    """Accept a list of entries or an object mapping quantity strings to
    probability strings. Duplicate quantities are merged by summing."""
    entries: list[tuple[int, Decimal]] = []
    if isinstance(raw, dict):
        for key, value in raw.items():
            _require(isinstance(key, str) and key.isdigit(),
                     f"item {item_id}: demand key must be a quantity string")
            prob = _parse_probability(value, f"item {item_id}: probability")
            entries.append((int(key), prob))
    elif isinstance(raw, list):
        for pos, entry in enumerate(raw):
            _require(isinstance(entry, dict),
                     f"item {item_id}: demand entry {pos} must be an object")
            qty = _parse_int(entry.get("quantity"),
                             f"item {item_id}: demand quantity")
            prob = _parse_probability(entry.get("probability"),
                                      f"item {item_id}: probability")
            entries.append((qty, prob))
    else:
        raise InputError(f"item {item_id}: demand must be an object or list")
    _require(len(entries) > 0, f"item {item_id}: demand must not be empty")
    merged: dict[int, Decimal] = {}
    for qty, prob in entries:
        _require(qty >= 0, f"item {item_id}: demand quantity must be >= 0")
        merged[qty] = merged.get(qty, Decimal(0)) + prob
    total = sum(merged.values(), Decimal(0))
    _require(total == 1,
             f"item {item_id}: probabilities must sum to exactly 1, got {total}")
    return tuple(sorted(merged.items()))


def parse_document(doc) -> tuple[list[Item], list[tuple[str, int]]]:
    """Validate the whole document; raise InputError before any solving."""
    _require(isinstance(doc, dict), "document must be a JSON object")
    raw_items = doc.get("items")
    _require(isinstance(raw_items, list) and raw_items,
             "items must be a non-empty list")
    items: list[Item] = []
    seen_ids: set[str] = set()
    for pos, raw in enumerate(raw_items):
        _require(isinstance(raw, dict), f"item {pos} must be an object")
        item_id = raw.get("id")
        _require(isinstance(item_id, str) and item_id,
                 f"item {pos}: id must be a non-empty string")
        _require(item_id not in seen_ids, f"duplicate item id: {item_id}")
        seen_ids.add(item_id)
        price = _parse_int(raw.get("price_cents"), f"item {item_id}: price_cents")
        cost = _parse_int(raw.get("cost_cents"), f"item {item_id}: cost_cents")
        salvage = _parse_int(raw.get("salvage_cents"),
                             f"item {item_id}: salvage_cents")
        penalty = _parse_int(raw.get("shortage_penalty_cents"),
                             f"item {item_id}: shortage_penalty_cents")
        _require(cost > 0, f"item {item_id}: cost must be positive")
        _require(price >= cost, f"item {item_id}: price must be >= cost")
        _require(0 <= salvage <= cost,
                 f"item {item_id}: salvage must be between 0 and cost")
        _require(penalty >= 0,
                 f"item {item_id}: shortage penalty must be non-negative")
        _require("demand" in raw, f"item {item_id}: missing demand")
        demand = _parse_demand(raw["demand"], item_id)
        items.append(Item(item_id, price, cost, salvage, penalty, demand))
    raw_budgets = doc.get("budgets")
    _require(isinstance(raw_budgets, list) and raw_budgets,
             "budgets must be a non-empty list")
    budgets: list[tuple[str, int]] = []
    for pos, entry in enumerate(raw_budgets):
        if isinstance(entry, dict):
            name = entry.get("name", f"budget_{pos}")
            _require(isinstance(name, str), f"budget {pos}: name must be a string")
            amount = _parse_int(entry.get("budget_cents"),
                                f"budget {name}: budget_cents")
        else:
            name = f"budget_{pos}"
            amount = _parse_int(entry, f"budget {pos}")
        _require(amount >= 0, f"budget {name}: budget must be non-negative")
        budgets.append((name, amount))
    return items, budgets
