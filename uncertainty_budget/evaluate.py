"""Forward-mode evaluation of expression ASTs with exact first derivatives.

Each evaluation returns the nominal value plus a dict of partial derivatives
with respect to the named inputs. Domain errors and non-differentiable or
infinite-derivative points raise EvaluationError.
"""

from __future__ import annotations

import math

from .errors import EvaluationError, ExpressionError
from .parser import Binary, Func, Num, Pow, Unary, Var


def _combine(a: dict, b: dict, fa: float, fb: float) -> dict:
    keys = a.keys() | b.keys()
    return {k: fa * a.get(k, 0.0) + fb * b.get(k, 0.0) for k in keys}


def evaluate(node, values: dict[str, float]) -> tuple[float, dict[str, float]]:
    """Return (value, partials) at the given nominal point."""
    if isinstance(node, Num):
        return node.value, {}
    if isinstance(node, Var):
        if node.name not in values:
            raise ExpressionError(f"undeclared quantity {node.name!r}", node.pos)
        return values[node.name], {node.name: 1.0}
    if isinstance(node, Unary):
        val, der = evaluate(node.operand, values)
        return -val, {k: -v for k, v in der.items()}
    if isinstance(node, Binary):
        lv, ld = evaluate(node.left, values)
        rv, rd = evaluate(node.right, values)
        if node.op == "+":
            return lv + rv, _combine(ld, rd, 1.0, 1.0)
        if node.op == "-":
            return lv - rv, _combine(ld, rd, 1.0, -1.0)
        if node.op == "*":
            return lv * rv, _combine(ld, rd, rv, lv)
        if rv == 0.0:
            raise EvaluationError("division by zero at the nominal point")
        return lv / rv, _combine(ld, rd, 1.0 / rv, -lv / (rv * rv))
    if isinstance(node, Pow):
        bv, bd = evaluate(node.base, values)
        n = node.exponent
        if n == 0:
            return 1.0, {k: 0.0 for k in bd}
        if bv == 0.0 and n < 0:
            raise EvaluationError("negative exponent of a zero base at the nominal point")
        value = bv ** n
        if bv == 0.0:
            factor = 1.0 if n == 1 else 0.0
        else:
            factor = n * bv ** (n - 1)
        return value, {k: factor * v for k, v in bd.items()}
    if isinstance(node, Func):
        av, ad = evaluate(node.arg, values)
        if node.name == "sqrt":
            if av < 0.0:
                raise EvaluationError("sqrt of a negative value at the nominal point")
            if av == 0.0:
                raise EvaluationError("sqrt is not differentiable at zero")
            value = math.sqrt(av)
            factor = 0.5 / value
        elif node.name == "exp":
            value = math.exp(av)
            if not math.isfinite(value):
                raise EvaluationError("exp overflow at the nominal point")
            factor = value
        else:  # ln
            if av <= 0.0:
                raise EvaluationError("ln of a non-positive value at the nominal point")
            value = math.log(av)
            factor = 1.0 / av
        return value, {k: factor * v for k, v in ad.items()}
    raise ExpressionError("unknown AST node")
