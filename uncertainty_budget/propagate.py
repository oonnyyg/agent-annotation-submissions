"""First-order uncertainty propagation through the Jacobian."""

from __future__ import annotations

import math

from .evaluate import evaluate
from .parser import parse


def covariance_matrix(stds: list[float], corr: list[list[float]]) -> list[list[float]]:
    n = len(stds)
    return [[corr[i][j] * stds[i] * stds[j] for j in range(n)] for i in range(n)]


def compute(spec_inputs, corr, spec_results):
    """Run the full propagation. Returns a results dict ready for JSON output."""
    names = [x["name"] for x in spec_inputs]
    values = {x["name"]: x["value"] for x in spec_inputs}
    stds = [x["std_uncertainty"] for x in spec_inputs]
    cov = covariance_matrix(stds, corr)
    n = len(names)

    jacobian = []  # rows: per result, dict name -> partial
    nominals = []
    for res in spec_results:
        ast = parse(res["expression"])
        value, partials = evaluate(ast, values)
        nominals.append(value)
        jacobian.append([partials.get(name, 0.0) for name in names])

    m = len(spec_results)
    out_cov = [[0.0] * m for _ in range(m)]
    for r in range(m):
        for s in range(m):
            total = 0.0
            for i in range(n):
                for j in range(n):
                    total += jacobian[r][i] * cov[i][j] * jacobian[s][j]
            out_cov[r][s] = total

    results_out = []
    for r, res in enumerate(spec_results):
        var = out_cov[r][r]
        # Guard against tiny negative roundoff for exactly-cancelling budgets.
        std = math.sqrt(var) if var > 0.0 else 0.0
        budget = []
        for i in range(n):
            budget.append({
                "type": "diagonal",
                "inputs": [names[i]],
                "contribution": jacobian[r][i] ** 2 * cov[i][i],
            })
        for i in range(n):
            for j in range(i + 1, n):
                # Both (i,j) and (j,i) cross terms combined; sign preserved.
                budget.append({
                    "type": "cross",
                    "inputs": [names[i], names[j]],
                    "contribution": 2.0 * jacobian[r][i] * cov[i][j] * jacobian[r][j],
                })
        results_out.append({
            "name": res["name"],
            "expression": res["expression"],
            "value": nominals[r],
            "std_uncertainty": std,
            "variance": var,
            "budget": budget,
        })

    correlations = []
    for r in range(m):
        row = []
        for s in range(m):
            denom = math.sqrt(out_cov[r][r] * out_cov[s][s])
            if denom == 0.0:
                row.append(None)
            else:
                row.append(max(-1.0, min(1.0, out_cov[r][s] / denom)))
        correlations.append(row)

    return {
        "inputs": [
            {"name": x["name"], "value": x["value"], "std_uncertainty": x["std_uncertainty"]}
            for x in spec_inputs
        ],
        "correlation": corr,
        "results": results_out,
        "result_correlation": correlations,
    }
