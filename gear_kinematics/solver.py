"""Solve the linear speed-constraint system with plain Gaussian elimination.

The system is A x = b over the axis speeds (rpm). The solver reports one of:
  - "unique":          exactly one solution
  - "underdetermined": infinitely many solutions (reports remaining DOF)
  - "conflicting":     inconsistent equations (reports an irreducible set of
                       constraint sources that cannot hold simultaneously)
"""

from __future__ import annotations

TOL = 1e-9


def _rref(matrix, rhs, ncols, tol=TOL):
    """In-place RREF; returns (pivot_columns, inconsistent_row_or_None)."""
    rows = len(matrix)
    pivots = []
    r = 0
    for c in range(ncols):
        if r >= rows:
            break
        best = max(range(r, rows), key=lambda i: abs(matrix[i][c]))
        if abs(matrix[best][c]) <= tol:
            continue
        matrix[r], matrix[best] = matrix[best], matrix[r]
        rhs[r], rhs[best] = rhs[best], rhs[r]
        piv = matrix[r][c]
        matrix[r] = [v / piv for v in matrix[r]]
        rhs[r] /= piv
        for i in range(rows):
            if i != r and abs(matrix[i][c]) > tol:
                f = matrix[i][c]
                matrix[i] = [a - f * b for a, b in zip(matrix[i], matrix[r])]
                rhs[i] -= f * rhs[r]
        pivots.append(c)
        r += 1
    inconsistent = None
    for i in range(rows):
        if all(abs(v) <= tol for v in matrix[i]) and abs(rhs[i]) > tol:
            inconsistent = i
            break
    return pivots, inconsistent


def _is_consistent(rows, ncols):
    mat = [list(coef) for coef, _ in rows]
    vec = [b for _, b in rows]
    _, bad = _rref(mat, vec, ncols)
    return bad is None


def _irreducible_conflict(rows, ncols):
    """Greedily shrink to a minimal inconsistent subset of rows."""
    keep = list(range(len(rows)))
    changed = True
    while changed:
        changed = False
        for i in list(keep):
            trial = [rows[k] for k in keep if k != i]
            if trial and not _is_consistent(trial, ncols):
                keep.remove(i)
                changed = True
    return [rows[k] for k in keep]


def solve(constraints, axes):
    """Solve constraints (model.Constraint list) over the given axis names."""
    index = {a: i for i, a in enumerate(axes)}
    n = len(axes)
    rows = []
    for con in constraints:
        coef = [0.0] * n
        for axis, c in con.coeffs.items():
            coef[index[axis]] = c
        rows.append((coef, con.rhs, con))

    mat = [list(coef) for coef, _, _ in rows]
    vec = [b for _, b, _ in rows]
    pivots, bad = _rref(mat, vec, n)

    if bad is not None:
        conflict_rows = _irreducible_conflict(
            [(coef, b) for coef, b, _ in rows], n)
        sources = []
        for coef, b in conflict_rows:
            for c2, b2, con in rows:
                if c2 == coef and b2 == b and con.source not in sources:
                    sources.append(con.source)
                    break
        return {"status": "conflicting", "conflict_sources": sources}

    rank = len(pivots)
    free_cols = [c for c in range(n) if c not in pivots]

    # One particular solution (free variables = 0); pivot rows are unit in RREF.
    particular = [0.0] * n
    for row, col in enumerate(pivots):
        particular[col] = vec[row]

    # Null-space basis vectors, one per free column.
    basis = []
    for fc in free_cols:
        v = [0.0] * n
        v[fc] = 1.0
        for row, col in enumerate(pivots):
            v[col] = -mat[row][fc]
        basis.append(v)

    determined = {}
    for i, axis in enumerate(axes):
        if all(abs(v[i]) <= TOL for v in basis):
            determined[axis] = particular[i]

    result = {
        "status": "unique" if not free_cols else "underdetermined",
        "determined": determined,
        "free_axes": [axes[c] for c in free_cols],
        "dof": len(free_cols),
    }
    if not free_cols:
        result["solution"] = particular
    return result


def residuals(constraints, axes, solution):
    """Per-constraint residual |A x - b| for a candidate solution."""
    index = {a: i for i, a in enumerate(axes)}
    out = []
    for con in constraints:
        lhs = sum(c * solution[index[a]] for a, c in con.coeffs.items())
        out.append({"source": con.source, "kind": con.kind,
                    "description": con.description, "residual": lhs - con.rhs})
    return out
