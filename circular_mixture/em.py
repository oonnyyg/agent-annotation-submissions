"""Weighted EM for von Mises mixtures with stable log-space computations."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .vm import (
    KAPPA_MAX,
    a1,
    circular_mean,
    kappa_from_r,
    log_density,
    norm_angle,
)

MIN_COMPONENT_WEIGHT = 1e-12


@dataclass
class Component:
    weight: float
    mu: float
    kappa: float


@dataclass
class FitResult:
    components: list[Component]
    responsibilities: list[list[float]]
    log_likelihood: float
    log_likelihood_history: list[float] = field(default_factory=list)
    iterations: int = 0
    converged: bool = False
    message: str = ""


def _logsumexp(values: list[float]) -> float:
    m = max(values)
    if m == -math.inf:
        return -math.inf
    return m + math.log(sum(math.exp(v - m) for v in values))


def e_step(
    angles: list[float], weights: list[float], components: list[Component]
) -> tuple[list[list[float]], float]:
    """Compute responsibilities and total weighted log-likelihood, in log space."""
    resp: list[list[float]] = []
    total_ll = 0.0
    for theta, w in zip(angles, weights):
        logs = [
            math.log(max(c.weight, MIN_COMPONENT_WEIGHT))
            + log_density(theta, c.mu, c.kappa)
            for c in components
        ]
        lse = _logsumexp(logs)
        total_ll += w * lse
        resp.append([math.exp(v - lse) for v in logs])
    return resp, total_ll


def m_step(
    angles: list[float], weights: list[float], resp: list[list[float]], k: int
) -> list[Component]:
    """Weighted M-step. Empty components are re-seeded at the previous mean
    with kappa 0 (uniform) and a floor weight, keeping them recoverable."""
    n_eff = [0.0] * k
    for r, w in zip(resp, weights):
        for j in range(k):
            n_eff[j] += w * r[j]
    total = sum(n_eff)
    components: list[Component] = []
    for j in range(k):
        if n_eff[j] <= MIN_COMPONENT_WEIGHT * total:
            # Degenerate/empty component: keep it alive as a uniform component.
            components.append(Component(MIN_COMPONENT_WEIGHT, 0.0, 0.0))
            continue
        comp_weights = [w * r[j] for w, r in zip(weights, resp)]
        mu, r_bar = circular_mean(angles, comp_weights)
        kappa = kappa_from_r(r_bar)
        components.append(Component(n_eff[j] / total, mu, kappa))
    # Re-normalize weights so they sum exactly to one.
    wsum = sum(c.weight for c in components)
    for c in components:
        c.weight /= wsum
    return components


def fit(
    angles: list[float],
    weights: list[float],
    initial: list[Component],
    tol: float = 1e-8,
    max_iter: int = 500,
) -> FitResult:
    """Run weighted EM. Convergence is declared only on log-likelihood
    stabilization; exhausting iterations is reported as NOT converged."""
    components = [Component(c.weight, norm_angle(c.mu), min(c.kappa, KAPPA_MAX))
                  for c in initial]
    history: list[float] = []
    prev_ll = -math.inf
    converged = False
    message = "maximum iterations reached without convergence"
    iterations = 0
    for iterations in range(1, max_iter + 1):
        resp, ll = e_step(angles, weights, components)
        if not math.isfinite(ll):
            return FitResult(components, resp, ll, history, iterations,
                             False, "numerical failure: non-finite log-likelihood")
        history.append(ll)
        components = m_step(angles, weights, resp, len(components))
        if abs(ll - prev_ll) <= tol * max(1.0, abs(ll)):
            converged = True
            message = "converged: log-likelihood stabilized"
            break
        prev_ll = ll
    # Final E-step with the final parameters for consistent outputs.
    resp, final_ll = e_step(angles, weights, components)
    history.append(final_ll)
    return FitResult(components, resp, final_ll, history, iterations,
                     converged, message)
