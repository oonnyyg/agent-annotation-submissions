"""Logistic regression fitting via Newton's method (IRLS)."""

from __future__ import annotations

import math
from dataclasses import dataclass

from .errors import UserError


def sigmoid(score: float) -> float:
    """Numerically stable logistic sigmoid; never overflows."""
    if score >= 0.0:
        return 1.0 / (1.0 + math.exp(-score))
    exp_score = math.exp(score)
    return exp_score / (1.0 + exp_score)


def log_loss_single(label: int, score: float) -> float:
    """Stable -log p(y|score) for a single sample; handles huge scores."""
    # log(1 + exp(score)) - label * score, computed without overflow.
    softplus = math.log1p(math.exp(-abs(score))) + max(score, 0.0)
    return softplus - label * score


def average_log_loss(labels: list[int], scores: list[float]) -> float:
    if not labels:
        raise UserError("cannot compute log loss on an empty set")
    return sum(log_loss_single(y, s) for y, s in zip(labels, scores)) / len(labels)


def scores_for(rows: list[list[float]], coefficients: list[float], intercept: float) -> list[float]:
    return [intercept + sum(c * x for c, x in zip(coefficients, row)) for row in rows]


@dataclass
class FitResult:
    intercept: float
    coefficients: list[float]
    iterations: int
    converged: bool
    final_objective: float


def _solve_linear(matrix: list[list[float]], rhs: list[float]) -> list[float] | None:
    """Solve matrix @ x = rhs by Gaussian elimination with partial pivoting."""
    n = len(rhs)
    aug = [row[:] + [rhs[i]] for i, row in enumerate(matrix)]
    for col in range(n):
        pivot = max(range(col, n), key=lambda r: abs(aug[r][col]))
        if abs(aug[pivot][col]) < 1e-14:
            return None
        aug[col], aug[pivot] = aug[pivot], aug[col]
        pivot_value = aug[col][col]
        for r in range(col + 1, n):
            factor = aug[r][col] / pivot_value
            if factor == 0.0:
                continue
            for c in range(col, n + 1):
                aug[r][c] -= factor * aug[col][c]
    x = [0.0] * n
    for i in range(n - 1, -1, -1):
        total = aug[i][n] - sum(aug[i][j] * x[j] for j in range(i + 1, n))
        x[i] = total / aug[i][i]
    return x


def _objective(labels, probabilities, coefficients, lam):
    n = len(labels)
    loss = sum(
        -math.log(max(p, 1e-300)) if y == 1 else -math.log(max(1.0 - p, 1e-300))
        for y, p in zip(labels, probabilities)
    ) / n
    penalty = 0.5 * lam * sum(c * c for c in coefficients)
    return loss + penalty


def fit_logistic(
    rows: list[list[float]],
    labels: list[int],
    lam: float,
    max_iterations: int,
    tolerance: float,
) -> FitResult:
    """Minimize mean binary cross-entropy + 0.5*lam*||w||^2 (intercept unpenalized)."""
    n = len(rows)
    if n == 0:
        raise UserError("cannot fit on an empty training set")
    if not any(labels) or all(labels):
        raise UserError("training set must contain both classes 0 and 1")
    p = len(rows[0])
    beta = [0.0] * (p + 1)  # beta[0] is the intercept

    converged = False
    iterations = 0
    probabilities = [0.5] * n
    for iteration in range(1, max_iterations + 1):
        iterations = iteration
        scores = [beta[0] + sum(beta[j + 1] * row[j] for j in range(p)) for row in rows]
        probabilities = [sigmoid(s) for s in scores]
        gradient = [0.0] * (p + 1)
        for row, y, prob in zip(rows, labels, probabilities):
            diff = prob - y
            gradient[0] += diff
            for j in range(p):
                gradient[j + 1] += diff * row[j]
        gradient = [g / n for g in gradient]
        for j in range(p):
            gradient[j + 1] += lam * beta[j + 1]

        grad_norm = max(abs(g) for g in gradient)
        if grad_norm < tolerance:
            converged = True
            break

        hessian = [[0.0] * (p + 1) for _ in range(p + 1)]
        for row, prob in zip(rows, probabilities):
            w = prob * (1.0 - prob)
            if w == 0.0:
                continue
            hessian[0][0] += w
            for j in range(p):
                hessian[0][j + 1] += w * row[j]
                for k in range(j, p):
                    hessian[j + 1][k + 1] += w * row[j] * row[k]
        for j in range(p + 1):
            for k in range(j):
                hessian[j][k] = hessian[k][j]
        for j in range(p + 1):
            for k in range(p + 1):
                hessian[j][k] /= n
        for j in range(p):
            hessian[j + 1][j + 1] += lam

        step = _solve_linear(hessian, gradient)
        if step is None:
            raise UserError("Hessian is singular; cannot continue optimization")
        # Backtracking line search to guarantee objective decrease.
        current = _objective(labels, probabilities, beta[1:], lam)
        step_scale = 1.0
        accepted = False
        for _ in range(50):
            candidate = [b - step_scale * s for b, s in zip(beta, step)]
            cand_scores = [candidate[0] + sum(candidate[j + 1] * row[j] for j in range(p)) for row in rows]
            cand_probs = [sigmoid(s) for s in cand_scores]
            if _objective(labels, cand_probs, candidate[1:], lam) <= current:
                beta = candidate
                probabilities = cand_probs
                accepted = True
                break
            step_scale *= 0.5
        if not accepted:
            raise UserError("line search failed to decrease the objective")

    final = _objective(labels, probabilities, beta[1:], lam)
    return FitResult(
        intercept=beta[0],
        coefficients=beta[1:],
        iterations=iterations,
        converged=converged,
        final_objective=final,
    )
