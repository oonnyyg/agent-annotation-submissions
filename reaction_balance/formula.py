"""Chemical formula parsing: element symbols, integer subscripts, nested parens."""

from __future__ import annotations

ELEMENTS = frozenset(
    "H He Li Be B C N O F Ne Na Mg Al Si P S Cl Ar K Ca Sc Ti V Cr Mn Fe Co Ni "
    "Cu Zn Ga Ge As Se Br Kr Rb Sr Y Zr Nb Mo Tc Ru Rh Pd Ag Cd In Sn Sb Te I "
    "Xe Cs Ba La Ce Pr Nd Pm Sm Eu Gd Tb Dy Ho Er Tm Yb Lu Hf Ta W Re Os Ir Pt "
    "Au Hg Tl Pb Bi Po At Rn Fr Ra Ac Th Pa U Np Pu Am Cm Bk Cf Es Fm Md No Lr "
    "Rf Db Sg Bh Hs Mt Ds Rg Cn Nh Fl Mc Lv Ts Og".split()
)


class FormulaError(ValueError):
    pass


def parse_formula(text: str) -> dict[str, int]:
    """Parse a formula into {element: count}. Raises FormulaError on bad input."""
    if not isinstance(text, str) or not text:
        raise FormulaError("empty formula")
    counts, pos = _parse_group(text, 0)
    if pos != len(text):
        raise FormulaError(f"unexpected character at {pos} in {text!r}")
    if not counts:
        raise FormulaError(f"formula {text!r} contains no elements")
    return counts


def _parse_group(text: str, pos: int) -> tuple[dict[str, int], int]:
    counts: dict[str, int] = {}
    while pos < len(text):
        ch = text[pos]
        if ch == "(":
            sub, pos = _parse_group(text, pos + 1)
            if pos >= len(text) or text[pos] != ")":
                raise FormulaError(f"unclosed parenthesis in {text!r}")
            pos += 1
            mult, pos = _parse_number(text, pos)
            for el, n in sub.items():
                counts[el] = counts.get(el, 0) + n * mult
        elif ch == ")":
            return counts, pos
        elif ch.isupper():
            end = pos + 1
            if end < len(text) and text[end].islower():
                end += 1
            symbol = text[pos:end]
            if symbol not in ELEMENTS:
                raise FormulaError(f"unknown element {symbol!r} in {text!r}")
            pos = end
            num, pos = _parse_number(text, pos)
            counts[symbol] = counts.get(symbol, 0) + num
        else:
            raise FormulaError(f"invalid character {ch!r} in {text!r}")
    return counts, pos


def _parse_number(text: str, pos: int) -> tuple[int, int]:
    start = pos
    while pos < len(text) and text[pos].isdigit():
        pos += 1
    if pos == start:
        return 1, pos
    value = int(text[start:pos])
    if value < 1:
        raise FormulaError(f"subscript must be a positive integer in {text!r}")
    return value, pos
