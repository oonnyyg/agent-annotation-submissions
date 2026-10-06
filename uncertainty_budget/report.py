"""Assemble the JSON report and the budget CSV."""

import csv
import io

from .errors import InputError
from .model import covariance_matrix, load_correlation, load_quantities
from .propagate import evaluate_results, propagate, result_correlations


def compute(spec):
    """Validate the input spec and compute the full uncertainty budget."""
    if not isinstance(spec, dict):
        raise InputError("top-level JSON value must be an object")
    quantities = load_quantities(spec.get("quantities"))
    correlation = load_correlation(spec.get("correlation"), len(quantities))
    covariance = covariance_matrix(quantities, correlation)
    evaluated = evaluate_results(spec.get("results"), quantities)
    budgets, result_cov = propagate(evaluated, covariance)
    corr = result_correlations(budgets, result_cov)
    return quantities, correlation, budgets, result_cov, corr


def report_dict(quantities, correlation, budgets, result_cov, corr):
    """JSON-serializable report: values, uncertainties, covariances."""
    results = []
    for r, budget in enumerate(budgets):
        results.append({
            "name": budget.name,
            "value": budget.value,
            "std_uncertainty": budget.std_uncertainty,
            "sensitivities": {
                q.name: budget.sensitivities[i]
                for i, q in enumerate(quantities)
            },
        })
    names = [b.name for b in budgets]
    correlations = {
        names[r]: {names[s]: corr[r][s] for s in range(len(names))}
        for r in range(len(names))
    }
    return {
        "results": results,
        "result_covariance": result_cov,
        "result_correlations": correlations,
        "input_correlation": correlation,
    }


def budget_csv(quantities, budgets):
    """CSV text: diagonal and cross contributions per result.

    Cross terms keep their sign; the 'sum' row equals the propagated
    variance exactly (no absolute values, no normalization).
    """
    out = io.StringIO()
    writer = csv.writer(out, lineterminator="\n")
    writer.writerow([
        "result", "term", "input_i", "input_j",
        "contribution_to_variance",
    ])
    for budget in budgets:
        total = 0.0
        for i, q in enumerate(quantities):
            writer.writerow([budget.name, "diagonal", q.name, "", budget.diag[i]])
            total += budget.diag[i]
        for i, j, contribution in budget.cross:
            writer.writerow([
                budget.name, "cross",
                quantities[i].name, quantities[j].name, contribution,
            ])
            total += contribution
        writer.writerow([budget.name, "sum", "", "", total])
        writer.writerow([budget.name, "propagated_variance", "", "",
                         budget.variance])
    return out.getvalue()
