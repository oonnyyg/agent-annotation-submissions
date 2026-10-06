"""Exact rational balancing of a single reaction via null-space analysis."""

from __future__ import annotations

from fractions import Fraction
from math import gcd

from .formula import parse_formula


class BalanceError(ValueError):
    """Raised when a reaction cannot be uniquely balanced."""


def _null_space(matrix):
    """Return a basis for the null space of matrix (rows x cols), exact."""
    rows = len(matrix)
    cols = len(matrix[0]) if rows else 0
    mat = [row[:] for row in matrix]
    pivot_cols = []
    r = 0
    for c in range(cols):
        pivot = next((i for i in range(r, rows) if mat[i][c] != 0), None)
        if pivot is None:
            continue
        mat[r], mat[pivot] = mat[pivot], mat[r]
        inv = 1 / mat[r][c]
        mat[r] = [x * inv for x in mat[r]]
        for i in range(rows):
            if i != r and mat[i][c] != 0:
                factor = mat[i][c]
                mat[i] = [a - factor * b for a, b in zip(mat[i], mat[r])]
        pivot_cols.append(c)
        r += 1
        if r == rows:
            break
    free = [c for c in range(cols) if c not in pivot_cols]
    basis = []
    for f in free:
        vec = [Fraction(0)] * cols
        vec[f] = Fraction(1)
        for rank, c in enumerate(pivot_cols):
            vec[c] = -mat[rank][f]
        basis.append(vec)
    return basis


def balance_reaction(reactants, products):
    """Balance one reaction. Returns {species: positive integer coefficient}.

    Raises BalanceError if no solution exists or the solution is not unique.
    """
    species = list(reactants) + list(products)
    if not reactants or not products:
        raise BalanceError("reaction needs at least one reactant and one product")
    if len(set(species)) != len(species):
        raise BalanceError("duplicate species in a single reaction")
    compositions = [parse_formula(s) for s in species]
    elements = sorted({el for comp in compositions for el in comp})
    matrix = []
    for el in elements:
        row = []
        for i, comp in enumerate(compositions):
            value = Fraction(comp.get(el, 0))
            row.append(value if i < len(reactants) else -value)
        matrix.append(row)
    basis = _null_space(matrix)
    if not basis:
        raise BalanceError("no solution: element conservation cannot be satisfied")
    if len(basis) > 1:
        raise BalanceError("solution is not unique (null space dimension > 1)")
    vec = basis[0]
    if any(v == 0 for v in vec):
        raise BalanceError("no solution with all coefficients nonzero")
    if all(v < 0 for v in vec):
        vec = [-v for v in vec]
    if not all(v > 0 for v in vec):
        raise BalanceError("no solution with all coefficients positive")
    den = 1
    for v in vec:
        den = den * v.denominator // gcd(den, v.denominator)
    ints = [int(v * den) for v in vec]
    g = 0
    for n in ints:
        g = gcd(g, n)
    ints = [n // g for n in ints]
    return {s: n for s, n in zip(species, ints)}
