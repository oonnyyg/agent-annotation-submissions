"""Rendering of solutions as JSON plan and per-item CSV."""

from __future__ import annotations

import csv
import io as _io
import json
from decimal import Decimal

from .core import BudgetSolution


def _num(value: Decimal) -> str:
    """Render a Decimal exactly, without exponent notation."""
    return format(value.normalize(), "f")


def plan_document(solutions: list[tuple[str, BudgetSolution]]) -> dict:
    plans = []
    for name, sol in solutions:
        plans.append({
            "name": name,
            "budget_cents": sol.budget_cents,
            "total_cost_cents": sol.total_cost_cents,
            "total_expected_profit_cents": _num(sol.total_expected_profit_cents),
            "items": [
                {
                    "id": item.item_id,
                    "quantity": metrics.quantity,
                    "purchase_cost_cents": metrics.quantity * item.cost_cents,
                    "expected_profit_cents": _num(metrics.expected_profit_cents),
                }
                for item, metrics in sol.choices
            ],
        })
    return {"plans": plans}


def plan_json(solutions: list[tuple[str, BudgetSolution]]) -> str:
    return json.dumps(plan_document(solutions), indent=2, ensure_ascii=False) + "\n"


CSV_FIELDS = [
    "budget", "item_id", "quantity",
    "expected_sales", "expected_leftover", "expected_shortage",
    "sales_revenue_cents", "salvage_revenue_cents",
    "purchase_cost_cents", "shortage_penalty_cents",
    "expected_profit_cents",
]


def items_csv(solutions: list[tuple[str, BudgetSolution]]) -> str:
    buffer = _io.StringIO()
    writer = csv.writer(buffer, lineterminator="\n")
    writer.writerow(CSV_FIELDS)
    for name, sol in solutions:
        for item, m in sol.choices:
            writer.writerow([
                name, item.item_id, m.quantity,
                _num(m.expected_sales), _num(m.expected_leftover),
                _num(m.expected_shortage), _num(m.sales_revenue_cents),
                _num(m.salvage_revenue_cents), _num(m.purchase_cost_cents),
                _num(m.shortage_penalty_cents), _num(m.expected_profit_cents),
            ])
    return buffer.getvalue()
