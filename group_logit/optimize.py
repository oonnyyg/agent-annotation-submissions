"""Preprocessing and a stable Newton (IRLS) logistic regression solver."""

import math

from .errors import GroupLogitError


def fit_scaler(rows):
    """Population mean/std over the given rows only. Constant columns get
    scale 1.0 so they remain zero after standardization."""
    n = len(rows)
    p = len(rows[0])
    means = [0.0] * p
    for row in rows:
        for j, value in enumerate(row):
            means[j] += value
    means = [m / n for m in means]
    scales = [0.0] * p
    for row in rows:
        for j, value in enumerate(row):
            diff = value - means[j]
            scales[j] += diff * diff
    scales = [math.sqrt(s / n) for s in scales]
    scales = [s if s > 0.0 else 1.0 for s in scales]
    return means, scales


def apply_scaler(rows, means, scales):
    return [[(value - means[j]) / scales[j] for j, value in enumerate(row)]
            for row in rows]


def _sigmoid(z):
    if z >= 0.0:
        return 1.0 / (1.0 + math.exp(-z))
    ez = math.exp(z)
    return ez / (1.0 + ez)


def log_loss(scores, labels):
    """Mean binary cross-entropy, stable for large |z|."""
    total = 0.0
    for z, y in zip(scores, labels):
        # log(1 + exp(z)) - y*z, computed without overflow
        total += max(z, 0.0) + math.log1p(math.exp(-abs(z))) - y * z
    return total / len(labels)


def _solve(matrix, vector):
    """Dense Gaussian elimination with partial pivoting."""
    n = len(vector)
    aug = [row[:] + [vector[i]] for i, row in enumerate(matrix)]
    for col in range(n):
        pivot = max(range(col, n), key=lambda r: abs(aug[r][col]))
        if abs(aug[pivot][col]) < 1e-14:
            aug[col][col] += 1e-10  # tiny ridge to keep the system solvable
            pivot = col
        aug[col], aug[pivot] = aug[pivot], aug[col]
        inv = 1.0 / aug[col][col]
        for r in range(col + 1, n):
            factor = aug[r][col] * inv
            if factor == 0.0:
                continue
            for c in range(col, n + 1):
                aug[r][c] -= factor * aug[col][c]
    out = [0.0] * n
    for i in range(n - 1, -1, -1):
        acc = aug[i][n]
        for c in range(i + 1, n):
            acc -= aug[i][c] * out[c]
        pivot = aug[i][i]
        if abs(pivot) < 1e-14:
            pivot = 1e-10 if pivot >= 0 else -1e-10
        out[i] = acc / pivot
    return out


def fit_logistic(features, labels, lam, max_iter, tol):
    """Minimize mean cross-entropy + lam * ||w||^2 / 2 (intercept unpenalized)
    via Newton's method. Returns (weights, intercept, info). Raises if the
    iteration does not converge within max_iter."""
    n = len(features)
    p = len(features[0])
    w = [0.0] * p
    b = 0.0

    def objective(wv, bv):
        scores = [bv + sum(wv[j] * x[j] for j in range(p)) for x in features]
        return log_loss(scores, labels) + 0.5 * lam * sum(v * v for v in wv)

    obj = objective(w, b)
    converged = False
    iterations = 0
    grad_norm = math.inf
    for iterations in range(1, max_iter + 1):
        grad_w = [lam * wj for wj in w]
        grad_b = 0.0
        h_w = [[0.0] * p for _ in range(p)]
        h_wb = [0.0] * p
        h_b = 0.0
        for x, y in zip(features, labels):
            z = b
            for j in range(p):
                z += w[j] * x[j]
            mu = _sigmoid(z)
            resid = mu - y
            grad_b += resid
            for j in range(p):
                grad_w[j] += resid * x[j]
            weight = max(mu * (1.0 - mu), 1e-12)
            h_b += weight
            for j in range(p):
                wx = weight * x[j]
                h_wb[j] += wx
                row = h_w[j]
                for k in range(j, p):
                    row[k] += wx * x[k]
        inv_n = 1.0 / n
        grad_w = [g * inv_n for g in grad_w]
        grad_b *= inv_n
        # Newton system for [w, b]
        size = p + 1
        hess = [[0.0] * size for _ in range(size)]
        for j in range(p):
            for k in range(j, p):
                value = h_w[j][k] * inv_n
                hess[j][k] = value
                hess[k][j] = value
            hess[j][j] += lam
            hess[j][p] = hess[p][j] = h_wb[j] * inv_n
        hess[p][p] = h_b * inv_n
        grad = grad_w + [grad_b]
        grad_norm = math.sqrt(sum(g * g for g in grad))
        if grad_norm <= tol:
            converged = True
            break
        step = _solve(hess, grad)
        # Backtracking Armijo line search on the directional derivative:
        # never accept an objective increase.
        gtd = sum(g * s for g, s in zip(grad, step))
        if gtd < 0.0:  # not a descent direction; fall back to steepest descent
            step = grad[:]
            gtd = sum(g * g for g in grad)
        t = 1.0
        new_w = w
        new_b = b
        new_obj = obj
        for _ in range(60):
            new_w = [w[j] - t * step[j] for j in range(p)]
            new_b = b - t * step[p]
            new_obj = objective(new_w, new_b)
            if new_obj <= obj - 1e-4 * t * gtd:
                break
            t *= 0.5
        else:
            raise GroupLogitError(
                f"line search failed to decrease the objective (lambda={lam})")
        step_norm = t * math.sqrt(sum(s * s for s in step))
        w, b, obj = new_w, new_b, new_obj
        if step_norm <= tol * (1.0 + math.sqrt(sum(v * v for v in w) + b * b)):
            converged = True
            break
    if not converged:
        raise GroupLogitError(
            f"optimizer failed to converge within {max_iter} iterations "
            f"(lambda={lam})")
    info = {"iterations": iterations, "converged": True,
            "objective": obj, "gradient_norm": grad_norm}
    return w, b, info
