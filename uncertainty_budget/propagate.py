"""First-order propagation of the full input covariance matrix.

For results y = f(x) with Jacobian J (rows = results, cols = inputs):
    Cov(y) = J Sigma J^T
The budget decomposes each result variance into per-input diagonal terms
J_ri^2 Sigma_ii and per-pair cross terms 2 J_ri J_rj Sigma_ij (i < j),
keeping signs so that correlated error sources may cancel.
"""

import math

from .errors import ExpressionError
from .expr import compile_expression


class ResultBudget:
    def __init__(self, name, value, sensitivities, variance, diag, cross):
        self.name = name
        self.value = value
        self.sensitivities = sensitivities
        self.variance = variance
        self.diag = diag
        self.cross = cross

    @property
    def std_uncertainty(self):
        return math.sqrt(max(self.variance, 0.0))


def evaluate_results(result_specs, quantities):
    """Evaluate each result expression and its gradient at the nominal point."""
    names = {q.name for q in quantities}
    env = {q.name: (q.nominal, i) for i, q in enumerate(quantities)}
    n = len(quantities)
    if not isinstance(result_specs, list) or not result_specs:
        raise ExpressionError("'results' must be a non-empty list")
    seen = set()
    evaluated = []
    for spec in result_specs:
        if not isinstance(spec, dict):
            raise ExpressionError(f"result entry must be an object, got {spec!r}")
        name = spec.get("name")
        if not isinstance(name, str) or not name.isidentifier():
            raise ExpressionError(f"result name must be an identifier, got {name!r}")
        if name in seen:
            raise ExpressionError(f"duplicate result name '{name}'", name)
        if name in names:
            raise ExpressionError(
                f"result name '{name}' collides with an input quantity", name)
        seen.add(name)
        node = compile_expression(spec.get("expression"), names, name)
        dual = node.eval(env, n)
        evaluated.append((name, dual))
    return evaluated


def propagate(evaluated, covariance):
    """Build budgets and the result covariance matrix."""
    n = len(covariance)
    budgets = []
    jacobian = []
    for name, dual in evaluated:
        grad = dual.grad
        jacobian.append(grad)
        diag = [grad[i] ** 2 * covariance[i][i] for i in range(n)]
        cross = []
        for i in range(n):
            for j in range(i + 1, n):
                cross.append((i, j, 2.0 * grad[i] * grad[j] * covariance[i][j]))
        variance = sum(diag) + sum(c for _, _, c in cross)
        budgets.append(ResultBudget(name, dual.value, grad, variance, diag, cross))
    m = len(evaluated)
    result_cov = [[0.0] * m for _ in range(m)]
    for r in range(m):
        for s in range(m):
            jr, js = jacobian[r], jacobian[s]
            total = 0.0
            for i in range(n):
                for j in range(n):
                    total += jr[i] * covariance[i][j] * js[j]
            result_cov[r][s] = total
    return budgets, result_cov


def result_correlations(budgets, result_cov):
    """Correlation coefficients between results; None when either is zero."""
    m = len(budgets)
    corr = [[None] * m for _ in range(m)]
    for r in range(m):
        for s in range(m):
            vr, vs = result_cov[r][r], result_cov[s][s]
            if vr <= 0.0 or vs <= 0.0:
                corr[r][s] = None
            else:
                corr[r][s] = result_cov[r][s] / math.sqrt(vr * vs)
    return corr
