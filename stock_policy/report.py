"""JSON and CSV rendering of solutions."""

from __future__ import annotations

import csv
import io
import json
from decimal import Decimal

from .solver import Solution


def _num(d: Decimal) -> str:
    return format(d.normalize(), "f")


def solutions_to_json(solutions: list[Solution]) -> str:
    doc = {"budgets": []}
    for sol in solutions:
        entry = {
            "budget": sol.budget,
            "total_cost": sol.total_cost,
            "expected_profit": _num(sol.expected_profit),
            "items": [
                {
                    "id": item.item_id,
                    "quantity": opt.quantity,
                    "expected_sales": _num(opt.expected_sales),
                    "expected_leftover": _num(opt.expected_leftover),
                    "expected_shortage": _num(opt.expected_shortage),
                    "expected_profit": _num(opt.expected_profit),
                }
                for item, opt in sol.choices
            ],
        }
        doc["budgets"].append(entry)
    return json.dumps(doc, indent=2, ensure_ascii=False) + "\n"


def solutions_to_csv(solutions: list[Solution]) -> str:
    buf = io.StringIO()
    writer = csv.writer(buf, lineterminator="\n")
    writer.writerow([
        "budget", "item_id", "quantity", "expected_sales",
        "expected_leftover", "expected_shortage", "expected_profit",
    ])
    for sol in solutions:
        for item, opt in sol.choices:
            writer.writerow([
                sol.budget, item.item_id, opt.quantity,
                _num(opt.expected_sales), _num(opt.expected_leftover),
                _num(opt.expected_shortage), _num(opt.expected_profit),
            ])
    return buf.getvalue()
