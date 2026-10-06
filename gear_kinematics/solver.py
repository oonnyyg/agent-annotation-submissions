"""Linear speed-constraint system: build and solve.

All shaft speeds share one global sign convention (positive = the chosen
positive rotation direction). Each constraint is a linear equation in the
shaft speeds; the system is solved with Gaussian elimination (stdlib only).
"""

from __future__ import annotations

from dataclasses import dataclass, field

from .model import Case, GearTrain

TOL = 1e-9


@dataclass
class Row:
    coeffs: dict[int, float]
    rhs: float
    sources: frozenset


def build_rows(train: GearTrain, case: Case) -> list:
    index = {name: i for i, name in enumerate(train.shafts)}
    rows = []
    for conn in train.connections:
        src = conn.label
        if conn.kind == "rigid":
            a, b = (index[s] for s in conn.shafts)
            rows.append(Row({a: 1.0, b: -1.0}, 0.0, frozenset({src})))
        elif conn.kind == "external_mesh":
            a, b = (index[s] for s in conn.shafts)
            z1, z2 = conn.teeth
            rows.append(Row({a: float(z1), b: float(z2)}, 0.0, frozenset({src})))
        elif conn.kind == "internal_mesh":
            a, b = (index[s] for s in conn.shafts)
            z1, z2 = conn.teeth
            rows.append(Row({a: float(z1), b: -float(z2)}, 0.0, frozenset({src})))
        elif conn.kind == "planetary":
            s, r, c = (index[x] for x in conn.shafts)
            z_sun, z_ring = conn.teeth
            # Willis: (w_sun - w_carrier)*z_sun + (w_ring - w_carrier)*z_ring = 0
            rows.append(Row(
                {s: float(z_sun), r: float(z_ring), c: -float(z_sun + z_ring)},
                0.0, frozenset({src}),
            ))
    for shaft, rpm in case.speeds.items():
        rows.append(Row({index[shaft]: 1.0}, float(rpm),
                        frozenset({f"case '{case.name}': speed of '{shaft}'"})))
    for shaft in case.fixed:
        rows.append(Row({index[shaft]: 1.0}, 0.0,
                        frozenset({f"case '{case.name}': fixed '{shaft}'"})))
    return rows


@dataclass
class Solution:
    status: str  # "unique" | "underdetermined" | "conflicting"
    speeds: dict = None
    free_shafts: list = field(default_factory=list)
    conflict_sources: list = field(default_factory=list)


def solve(train: GearTrain, case: Case) -> Solution:
    n = len(train.shafts)
    rows = build_rows(train, case)
    matrix = [[r.coeffs.get(j, 0.0) for j in range(n)] for r in rows]
    rhs = [r.rhs for r in rows]
    sources = [set(r.sources) for r in rows]

    pivot_cols = []
    pivot_rows = []
    row = 0
    for col in range(n):
        best, best_val = -1, TOL
        for r in range(row, len(matrix)):
            if abs(matrix[r][col]) > best_val:
                best, best_val = r, abs(matrix[r][col])
        if best < 0:
            continue
        matrix[row], matrix[best] = matrix[best], matrix[row]
        rhs[row], rhs[best] = rhs[best], rhs[row]
        sources[row], sources[best] = sources[best], sources[row]
        pivot = matrix[row][col]
        for r in range(len(matrix)):
            if r == row:
                continue
            factor = matrix[r][col] / pivot
            if abs(factor) < TOL:
                continue
            for j in range(n):
                matrix[r][j] -= factor * matrix[row][j]
            rhs[r] -= factor * rhs[row]
            sources[r] |= sources[row]
        pivot_cols.append(col)
        pivot_rows.append(row)
        row += 1

    conflict = set()
    for r in range(len(matrix)):
        if all(abs(matrix[r][j]) < TOL for j in range(n)) and abs(rhs[r]) > TOL:
            conflict |= sources[r]
    if conflict:
        return Solution("conflicting", conflict_sources=sorted(conflict))

    free = [j for j in range(n) if j not in pivot_cols]
    if free:
        return Solution("underdetermined",
                        free_shafts=[train.shafts[j] for j in free])

    speeds = {}
    for col, r in zip(pivot_cols, pivot_rows):
        value = rhs[r] / matrix[r][col]
        speeds[train.shafts[col]] = value if value else 0.0
    return Solution("unique", speeds=speeds)


def residuals(train: GearTrain, case: Case, speeds: dict) -> list:
    """Residual of every constraint (kinematic and case) for a solution."""
    names = list(train.shafts)
    out = []
    for row in build_rows(train, case):
        value = sum(c * speeds[names[j]] for j, c in row.coeffs.items()) - row.rhs
        out.append({"sources": sorted(row.sources), "residual": value})
    return out
