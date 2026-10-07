"""Weighted EM for von Mises mixtures with numerically stable bookkeeping."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .errors import FitError
from .vmm import KAPPA_MAX, TWO_PI, a1_inverse, circular_mean, log_bessel_i0

LOG_TWO_PI = math.log(TWO_PI)
MIN_COMPONENT_WEIGHT = 1e-12


@dataclass
class Component:
    weight: float
    mu: float
    kappa: float


@dataclass
class FitResult:
    components: list[Component]
    log_likelihood: float
    log_likelihood_history: list[float] = field(default_factory=list)
    iterations: int = 0
    converged: bool = False
    message: str = ""


def _log_component_density(theta: float, mu: float, kappa: float) -> float:
    """log of the von Mises pdf; kappa == 0 is the uniform density 1/(2*pi)."""
    if kappa == 0.0:
        return -LOG_TWO_PI
    return kappa * math.cos(theta - mu) - LOG_TWO_PI - log_bessel_i0(kappa)


def _e_step(
    angles: list[float], weights: list[float], components: list[Component]
) -> tuple[list[list[float]], float]:
    """Responsibilities via log-sum-exp; returns (resp, weighted log-lik)."""
    k = len(components)
    resp = [[0.0] * k for _ in angles]
    total_ll = 0.0
    for i, theta in enumerate(angles):
        logs = [
            (
                math.log(comp.weight)
                + _log_component_density(theta, comp.mu, comp.kappa)
                if comp.weight > 0.0
                else -math.inf
            )
            for comp in components
        ]
        m = max(logs)
        if m == -math.inf:
            raise FitError("all components have zero weight")
        exps = [math.exp(v - m) for v in logs]
        s = sum(exps)
        if not (s > 0.0) or not math.isfinite(s):
            raise FitError("responsibility normalization failed numerically")
        total_ll += weights[i] * (m + math.log(s))
        row = resp[i]
        for j in range(k):
            row[j] = exps[j] / s
    return resp, total_ll


def _m_step(
    angles: list[float], weights: list[float], resp: list[list[float]]
) -> list[Component]:
    k = len(resp[0])
    new_components: list[Component] = []
    w_total = sum(weights)
    for j in range(k):
        comp_weights = [weights[i] * resp[i][j] for i in range(len(angles))]
        n_eff = sum(comp_weights)
        if n_eff <= MIN_COMPONENT_WEIGHT:
            # Empty component: keep it at zero weight with a uniform density.
            # It stays identifiable in the output instead of being hidden.
            new_components.append(Component(weight=0.0, mu=0.0, kappa=0.0))
            continue
        mu, r = circular_mean(angles, comp_weights)
        kappa = min(a1_inverse(r), KAPPA_MAX)
        new_components.append(Component(weight=n_eff / w_total, mu=mu, kappa=kappa))
    s = sum(c.weight for c in new_components)
    if s <= 0.0:
        raise FitError("all components became empty during M-step")
    for c in new_components:
        c.weight /= s
    return new_components


def fit(
    angles: list[float],
    weights: list[float],
    components: list[Component],
    tol: float = 1e-8,
    max_iter: int = 500,
) -> FitResult:
    """Run weighted EM. Exhausting max_iter is reported as NOT converged."""
    history: list[float] = []
    resp, ll = _e_step(angles, weights, components)
    history.append(ll)
    converged = False
    iterations = 0
    for iterations in range(1, max_iter + 1):
        components = _m_step(angles, weights, resp)
        resp, ll_new = _e_step(angles, weights, components)
        history.append(ll_new)
        if not math.isfinite(ll_new):
            raise FitError("log-likelihood became non-finite")
        if abs(ll_new - ll) <= tol * max(1.0, abs(ll)):
            ll = ll_new
            converged = True
            break
        ll = ll_new
    message = (
        "converged"
        if converged
        else f"not converged: iteration limit {max_iter} reached"
    )
    return FitResult(
        components=components,
        log_likelihood=ll,
        log_likelihood_history=history,
        iterations=iterations,
        converged=converged,
        message=message,
    )


def final_responsibilities(
    angles: list[float], weights: list[float], components: list[Component]
) -> tuple[list[list[float]], float]:
    """Recompute responsibilities and total log-likelihood with final params."""
    return _e_step(angles, weights, components)
