"""Input schema parsing and geometric/physical validation."""

from __future__ import annotations

from dataclasses import dataclass, field


class InputError(ValueError):
    """Raised when the user supplied configuration is invalid."""


@dataclass
class Surface:
    name: str
    area: float
    emissivity: float


@dataclass
class Case:
    name: str
    # temperature in kelvin, or None for an adiabatic (zero net flux) surface
    temperatures: list[float | None] = field(default_factory=list)


@dataclass
class Enclosure:
    surfaces: list[Surface]
    view_factors: list[list[float]]
    cases: list[Case]
    tolerance: float


def _require(cond: bool, message: str) -> None:
    if not cond:
        raise InputError(message)


def load_enclosure(data: dict) -> Enclosure:
    _require(isinstance(data, dict), "top-level JSON value must be an object")

    raw_surfaces = data.get("surfaces")
    _require(isinstance(raw_surfaces, list) and len(raw_surfaces) >= 1,
             "'surfaces' must be a non-empty list")

    surfaces: list[Surface] = []
    names: set[str] = set()
    for i, raw in enumerate(raw_surfaces):
        _require(isinstance(raw, dict), f"surfaces[{i}] must be an object")
        name = raw.get("name")
        _require(isinstance(name, str) and name, f"surfaces[{i}].name must be a non-empty string")
        _require(name not in names, f"duplicate surface name: {name!r}")
        names.add(name)
        area = raw.get("area")
        _require(isinstance(area, (int, float)) and not isinstance(area, bool) and area > 0,
                 f"surface {name!r}: area must be a positive number")
        emissivity = raw.get("emissivity")
        _require(isinstance(emissivity, (int, float)) and not isinstance(emissivity, bool)
                 and 0.0 < emissivity <= 1.0,
                 f"surface {name!r}: emissivity must be in (0, 1]")
        surfaces.append(Surface(name=name, area=float(area), emissivity=float(emissivity)))

    n = len(surfaces)
    tol = data.get("tolerance", 1e-6)
    _require(isinstance(tol, (int, float)) and not isinstance(tol, bool) and tol > 0,
             "'tolerance' must be a positive number")
    tol = float(tol)

    vf = data.get("view_factors")
    _require(isinstance(vf, list) and len(vf) == n,
             f"'view_factors' must be a {n}x{n} matrix")
    matrix: list[list[float]] = []
    for i, row in enumerate(vf):
        _require(isinstance(row, list) and len(row) == n,
                 f"view_factors row {i} must have {n} entries")
        parsed = []
        for j, value in enumerate(row):
            _require(isinstance(value, (int, float)) and not isinstance(value, bool),
                     f"view_factors[{i}][{j}] must be a number")
            _require(value >= 0.0, f"view_factors[{i}][{j}] must be non-negative")
            parsed.append(float(value))
        matrix.append(parsed)

    # Row-sum (summation) rule: sum_j F_ij = 1 for a closed enclosure.
    for i, row in enumerate(matrix):
        s = sum(row)
        _require(abs(s - 1.0) <= tol,
                 f"view factor row {i} sums to {s}, not 1 (tolerance {tol}); "
                 "fix the matrix, it will not be renormalized automatically")

    # Reciprocity: A_i F_ij = A_j F_ji.
    for i in range(n):
        for j in range(i + 1, n):
            lhs = surfaces[i].area * matrix[i][j]
            rhs = surfaces[j].area * matrix[j][i]
            _require(abs(lhs - rhs) <= tol,
                     f"reciprocity violated between {surfaces[i].name!r} and "
                     f"{surfaces[j].name!r}: A_i*F_ij={lhs}, A_j*F_ji={rhs} "
                     f"(tolerance {tol})")

    raw_cases = data.get("cases")
    _require(isinstance(raw_cases, list) and len(raw_cases) >= 1,
             "'cases' must be a non-empty list")
    cases: list[Case] = []
    case_names: set[str] = set()
    for c, raw in enumerate(raw_cases):
        _require(isinstance(raw, dict), f"cases[{c}] must be an object")
        cname = raw.get("name", f"case-{c + 1}")
        _require(isinstance(cname, str) and cname, f"cases[{c}].name must be a non-empty string")
        _require(cname not in case_names, f"duplicate case name: {cname!r}")
        case_names.add(cname)
        raw_temps = raw.get("temperatures")
        _require(isinstance(raw_temps, dict),
                 f"case {cname!r}: 'temperatures' must map surface names to kelvin or null")
        _require(set(raw_temps) == names,
                 f"case {cname!r}: 'temperatures' must list every surface exactly once")
        temps: list[float | None] = []
        for surf in surfaces:
            value = raw_temps[surf.name]
            if value is None:
                temps.append(None)
            else:
                _require(isinstance(value, (int, float)) and not isinstance(value, bool)
                         and value > 0.0,
                         f"case {cname!r}, surface {surf.name!r}: "
                         "temperature must be a positive kelvin value or null (adiabatic)")
                temps.append(float(value))
        cases.append(Case(name=cname, temperatures=temps))

    return Enclosure(surfaces=surfaces, view_factors=matrix, cases=cases, tolerance=tol)
