"""Input model and strict validation for enclosure radiation problems."""

from __future__ import annotations

import json
import math
from dataclasses import dataclass, field


class ConfigError(ValueError):
    """Raised when the input configuration is invalid."""


@dataclass(frozen=True)
class Surface:
    name: str
    area: float          # m^2, > 0
    emissivity: float    # 0 < eps <= 1


@dataclass(frozen=True)
class CaseSurface:
    temperature: float | None  # K, or None when adiabatic


@dataclass(frozen=True)
class Case:
    name: str
    surfaces: tuple[CaseSurface, ...]


@dataclass(frozen=True)
class Enclosure:
    surfaces: tuple[Surface, ...]
    view_factors: tuple[tuple[float, ...], ...]  # F[i][j], from i to j
    row_sum_tol: float = 1e-9
    reciprocity_tol: float = 1e-9
    cases: tuple[Case, ...] = field(default=())


def _require(cond: bool, msg: str) -> None:
    if not cond:
        raise ConfigError(msg)


def _parse_surfaces(raw: object) -> tuple[Surface, ...]:
    _require(isinstance(raw, list) and raw, "'surfaces' must be a non-empty list")
    out = []
    names = set()
    for i, item in enumerate(raw):
        _require(isinstance(item, dict), f"surfaces[{i}] must be an object")
        name = item.get("name", f"s{i}")
        _require(isinstance(name, str) and name not in names,
                 f"surface name {name!r} missing or duplicated")
        names.add(name)
        area = item.get("area")
        _require(isinstance(area, (int, float)) and math.isfinite(area) and area > 0,
                 f"surface {name!r}: area must be a positive finite number")
        eps = item.get("emissivity")
        _require(isinstance(eps, (int, float)) and 0.0 < eps <= 1.0,
                 f"surface {name!r}: emissivity must be in (0, 1]")
        out.append(Surface(name=name, area=float(area), emissivity=float(eps)))
    return tuple(out)


def _parse_view_factors(raw: object, n: int,
                        row_sum_tol: float,
                        reciprocity_tol: float,
                        areas: tuple[float, ...]) -> tuple[tuple[float, ...], ...]:
    _require(isinstance(raw, list) and len(raw) == n,
             "'view_factors' must be a square matrix matching surfaces")
    matrix = []
    for i, row in enumerate(raw):
        _require(isinstance(row, list) and len(row) == n,
                 f"view_factors[{i}] must have {n} entries")
        vals = []
        for j, v in enumerate(row):
            _require(isinstance(v, (int, float)) and math.isfinite(v) and v >= 0.0,
                     f"view_factors[{i}][{j}] must be a non-negative finite number")
            vals.append(float(v))
        matrix.append(tuple(vals))
    for i in range(n):
        s = sum(matrix[i])
        _require(abs(s - 1.0) <= row_sum_tol,
                 f"view factor row {i} sums to {s!r}, outside tolerance {row_sum_tol}")
    for i in range(n):
        for j in range(i + 1, n):
            lhs = areas[i] * matrix[i][j]
            rhs = areas[j] * matrix[j][i]
            _require(abs(lhs - rhs) <= reciprocity_tol,
                     f"reciprocity violated for ({i},{j}): "
                     f"A{i}*F={lhs!r} vs A{j}*F={rhs!r} (tol {reciprocity_tol})")
    return tuple(matrix)


def _parse_cases(raw: object, n: int) -> tuple[Case, ...]:
    _require(isinstance(raw, list) and raw, "'cases' must be a non-empty list")
    out = []
    names = set()
    for c, item in enumerate(raw):
        _require(isinstance(item, dict), f"cases[{c}] must be an object")
        name = item.get("name", f"case{c}")
        _require(isinstance(name, str) and name not in names,
                 f"case name {name!r} missing or duplicated")
        names.add(name)
        specs = item.get("surfaces")
        _require(isinstance(specs, list) and len(specs) == n,
                 f"case {name!r}: 'surfaces' must list one entry per surface")
        cs = []
        for i, spec in enumerate(specs):
            _require(isinstance(spec, dict),
                     f"case {name!r}: surfaces[{i}] must be an object")
            has_t = "temperature_K" in spec
            has_a = bool(spec.get("adiabatic", False))
            _require(has_t != has_a,
                     f"case {name!r} surface {i}: specify exactly one of "
                     f"'temperature_K' or 'adiabatic'")
            if has_t:
                t = spec["temperature_K"]
                _require(isinstance(t, (int, float)) and math.isfinite(t) and t > 0,
                         f"case {name!r} surface {i}: temperature_K must be > 0")
                cs.append(CaseSurface(temperature=float(t)))
            else:
                cs.append(CaseSurface(temperature=None))
        out.append(Case(name=name, surfaces=tuple(cs)))
    return tuple(out)


def load_config(path: str) -> Enclosure:
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    _require(isinstance(data, dict), "top-level JSON must be an object")
    surfaces = _parse_surfaces(data.get("surfaces"))
    tol = data.get("tolerances", {})
    _require(isinstance(tol, dict), "'tolerances' must be an object")
    row_sum_tol = float(tol.get("row_sum", 1e-9))
    reciprocity_tol = float(tol.get("reciprocity", 1e-9))
    _require(row_sum_tol >= 0 and reciprocity_tol >= 0,
             "tolerances must be non-negative")
    areas = tuple(s.area for s in surfaces)
    vf = _parse_view_factors(data.get("view_factors"), len(surfaces),
                             row_sum_tol, reciprocity_tol, areas)
    cases = _parse_cases(data.get("cases"), len(surfaces))
    return Enclosure(surfaces=surfaces, view_factors=vf,
                     row_sum_tol=row_sum_tol,
                     reciprocity_tol=reciprocity_tol, cases=cases)
