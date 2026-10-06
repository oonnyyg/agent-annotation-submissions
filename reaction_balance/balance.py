"""Balance chemical equations with exact rational arithmetic.

Builds the element-conservation matrix A (elements x species) where
reactant columns are positive and product columns are negative, then
computes the null space over the rationals. An equation is uniquely
balanceable only when the null space has dimension exactly one and the
basis vector can be scaled so every coefficient is strictly positive.
"""

from __future__ import annotations

from fractions import Fraction
from math import gcd

from .formula import parse_formula


class BalanceError(ValueError):
    """Raised when an equation cannot be uniquely balanced."""


class NoSolutionError(BalanceError):
    """No positive balancing exists (null space has no all-positive vector)."""


class NotUniqueError(BalanceError):
    """Balancing is not unique (null space dimension greater than one)."""


def balance_equation(
    reactants: list[str], products: list[str]
) -> dict[str, int]:
    """Return coprime positive integer coefficients for each species.

    Raises BalanceError subclasses when no unique positive solution exists.
    """
    species = list(reactants) + list(products)
    if not species:
        raise BalanceError("equation has no species")
    seen: set[str] = set()
    for name in species:
        if name in seen:
            raise BalanceError(f"duplicate species in equation: {name!r}")
        seen.add(name)

    compositions = [parse_formula(name) for name in species]
    elements = sorted({e for comp in compositions for e in comp})
    # Matrix rows: elements; columns: species. Reactants positive,
    # products negative so that A * coefficients = 0 expresses balance.
    matrix = [
        [
            Fraction(comp.get(elem, 0))
            * (1 if idx < len(reactants) else -1)
            for idx, comp in enumerate(compositions)
        ]
        for elem in elements
    ]
    basis = _null_space(matrix, len(species))
    if len(basis) == 0:
        raise NoSolutionError(
            "no non-trivial balancing exists for this equation"
        )
    if len(basis) > 1:
        raise NotUniqueError(
            f"balancing is not unique: null space dimension is {len(basis)}"
        )
    vector = basis[0]
    if all(v > 0 for v in vector):
        coeffs = vector
    elif all(v < 0 for v in vector):
        coeffs = [-v for v in vector]
    else:
        raise NoSolutionError(
            "no balancing with all-positive coefficients exists"
        )
    denominators = 1
    for value in coeffs:
        denominators = denominators * value.denominator // gcd(
            denominators, value.denominator
        )
    integers = [int(value * denominators) for value in coeffs]
    common = 0
    for value in integers:
        common = gcd(common, value)
    integers = [value // common for value in integers]
    return dict(zip(species, integers))


def _rref(matrix: list[list[Fraction]], ncols: int) -> tuple[list[list[Fraction]], list[int]]:
    """Reduced row echelon form over the rationals; returns (rref, pivots)."""
    rows = [row[:] for row in matrix]
    pivots: list[int] = []
    r = 0
    for c in range(ncols):
        pivot = None
        for i in range(r, len(rows)):
            if rows[i][c] != 0:
                pivot = i
                break
        if pivot is None:
            continue
        rows[r], rows[pivot] = rows[pivot], rows[r]
        scale = rows[r][c]
        rows[r] = [v / scale for v in rows[r]]
        for i in range(len(rows)):
            if i != r and rows[i][c] != 0:
                factor = rows[i][c]
                rows[i] = [a - factor * b for a, b in zip(rows[i], rows[r])]
        pivots.append(c)
        r += 1
        if r == len(rows):
            break
    return rows, pivots


def _null_space(matrix: list[list[Fraction]], ncols: int) -> list[list[Fraction]]:
    """Basis of the null space of the matrix (over the rationals)."""
    rref, pivots = _rref(matrix, ncols)
    free = [c for c in range(ncols) if c not in pivots]
    basis: list[list[Fraction]] = []
    for free_col in free:
        vector = [Fraction(0)] * ncols
        vector[free_col] = Fraction(1)
        for row_idx, pivot_col in enumerate(pivots):
            vector[pivot_col] = -rref[row_idx][free_col]
        basis.append(vector)
    return basis