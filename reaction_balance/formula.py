"""Parse chemical formulas into element counts.

Supported syntax: element symbols (uppercase letter followed by zero or
more lowercase letters), positive integer subscripts, and nested
parentheses. Ions, hydrate dots and isotopes are not supported.
"""

from __future__ import annotations

# Standard element symbols (periodic table, atomic numbers 1-118).
ELEMENTS: frozenset[str] = frozenset(
    "H He Li Be B C N O F Ne Na Mg Al Si P S Cl Ar K Ca Sc Ti V Cr Mn Fe Co "
    "Ni Cu Zn Ga Ge As Se Br Kr Rb Sr Y Zr Nb Mo Tc Ru Rh Pd Ag Cd In Sn Sb "
    "Te I Xe Cs Ba La Ce Pr Nd Pm Sm Eu Gd Tb Dy Ho Er Tm Yb Lu Hf Ta W Re "
    "Os Ir Pt Au Hg Tl Pb Bi Po At Rn Fr Ra Ac Th Pa U Np Pu Am Cm Bk Cf Es "
    "Fm Md No Lr Rf Db Sg Bh Hs Mt Ds Rg Cn Nh Fl Mc Lv Ts Og".split()
)


class FormulaError(ValueError):
    """Raised when a chemical formula is invalid."""


def parse_formula(formula: str) -> dict[str, int]:
    """Parse a formula string into a mapping of element -> count."""
    if not isinstance(formula, str) or not formula:
        raise FormulaError(f"empty or invalid formula: {formula!r}")
    counts, pos = _parse_group(formula, 0)
    if pos != len(formula):
        raise FormulaError(f"unexpected character at position {pos} in {formula!r}")
    if not counts:
        raise FormulaError(f"formula has no elements: {formula!r}")
    return counts


def _parse_group(text: str, pos: int) -> tuple[dict[str, int], int]:
    counts: dict[str, int] = {}
    while pos < len(text):
        ch = text[pos]
        if ch == "(":
            inner, pos = _parse_group(text, pos + 1)
            if pos >= len(text) or text[pos] != ")":
                raise FormulaError(f"unbalanced '(' in {text!r}")
            pos += 1
            mult, pos = _parse_number(text, pos)
            for elem, num in inner.items():
                counts[elem] = counts.get(elem, 0) + num * mult
        elif ch == ")":
            return counts, pos
        elif ch.isupper():
            pos += 1
            start = pos
            while pos < len(text) and text[pos].islower():
                pos += 1
            symbol = text[start - 1:pos]
            if symbol not in ELEMENTS:
                raise FormulaError(f"unknown element symbol: {symbol!r}")
            mult, pos = _parse_number(text, pos)
            counts[symbol] = counts.get(symbol, 0) + mult
        elif ch.isdigit():
            raise FormulaError(f"unexpected digit at position {pos} in {text!r}")
        else:
            raise FormulaError(f"unsupported character {ch!r} in formula {text!r}")
    return counts, pos


def _parse_number(text: str, pos: int) -> tuple[int, int]:
    start = pos
    while pos < len(text) and text[pos].isdigit():
        pos += 1
    if pos == start:
        return 1, pos
    value = int(text[start:pos])
    if value <= 0:
        raise FormulaError(f"subscript must be a positive integer in {text!r}")
    return value, pos