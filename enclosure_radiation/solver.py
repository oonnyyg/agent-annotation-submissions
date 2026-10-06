"""Radiosity solver for gray diffuse enclosures (pure standard library)."""

from __future__ import annotations

from dataclasses import dataclass

from .model import Enclosure, Case, ConfigError

SIGMA = 5.670374419e-8  # Stefan-Boltzmann constant, W/(m^2 K^4)


class UnderconstrainedError(ConfigError):
    """A radiation-connected component lacks a fixed-temperature surface."""


@dataclass(frozen=True)
class SurfaceResult:
    name: str
    temperature_K: float
    radiosity: float        # J, W/m^2
    irradiation: float      # G, W/m^2
    net_heat_flux: float    # J - G, W/m^2, positive outward
    net_power: float        # W, positive outward


@dataclass(frozen=True)
class CaseResult:
    name: str
    surfaces: tuple[SurfaceResult, ...]
    exchange: tuple[tuple[float, ...], ...]  # P[i][j] W, net i -> j
    energy_residual: float                   # sum of net powers, W


def _components(enc: Enclosure) -> list[list[int]]:
    n = len(enc.surfaces)
    seen = [False] * n
    comps = []
    for start in range(n):
        if seen[start]:
            continue
        stack, comp = [start], []
        seen[start] = True
        while stack:
            i = stack.pop()
            comp.append(i)
            for j in range(n):
                if not seen[j] and (enc.view_factors[i][j] > 0.0
                                    or enc.view_factors[j][i] > 0.0):
                    seen[j] = True
                    stack.append(j)
        comps.append(sorted(comp))
    return comps


def _check_temperature_anchor(enc: Enclosure, case: Case) -> None:
    for comp in _components(enc):
        if all(case.surfaces[i].temperature is None for i in comp):
            names = ", ".join(enc.surfaces[i].name for i in comp)
            raise UnderconstrainedError(
                f"case {case.name!r}: radiation-connected component "
                f"({names}) has no fixed-temperature surface; "
                f"the problem is under-constrained")


def _solve_linear(a: list[list[float]], b: list[float]) -> list[float]:
    n = len(b)
    for col in range(n):
        piv = max(range(col, n), key=lambda r: abs(a[r][col]))
        if abs(a[piv][col]) < 1e-300:
            raise ConfigError("radiosity system is singular")
        a[col], a[piv] = a[piv], a[col]
        b[col], b[piv] = b[piv], b[col]
        inv = 1.0 / a[col][col]
        for r in range(col + 1, n):
            f = a[r][col] * inv
            if f == 0.0:
                continue
            for c in range(col, n):
                a[r][c] -= f * a[col][c]
            b[r] -= f * b[col]
    x = [0.0] * n
    for i in range(n - 1, -1, -1):
        s = b[i] - sum(a[i][j] * x[j] for j in range(i + 1, n))
        x[i] = s / a[i][i]
    return x


def solve_case(enc: Enclosure, case: Case) -> CaseResult:
    """Solve one temperature case via the radiosity-irradiation equations.

    Fixed-T surface i:  J_i - (1-eps_i) * sum_j F_ij J_j = eps_i * sigma * T_i^4
    Adiabatic surface:  J_i - sum_j F_ij J_j = 0   (net heat flux zero)
    """
    _check_temperature_anchor(enc, case)
    n = len(enc.surfaces)
    a = [[0.0] * n for _ in range(n)]
    b = [0.0] * n
    for i, surf in enumerate(enc.surfaces):
        temp = case.surfaces[i].temperature
        rho = 1.0 - surf.emissivity if temp is not None else 1.0
        a[i][i] = 1.0
        for j in range(n):
            a[i][j] -= rho * enc.view_factors[i][j]
        b[i] = 0.0 if temp is None else surf.emissivity * SIGMA * temp ** 4
    j_rad = _solve_linear(a, b)

    results = []
    for i, surf in enumerate(enc.surfaces):
        g = sum(enc.view_factors[i][j] * j_rad[j] for j in range(n))
        temp = case.surfaces[i].temperature
        if temp is None:
            temp = (j_rad[i] / SIGMA) ** 0.25
        flux = j_rad[i] - g
        results.append(SurfaceResult(
            name=surf.name, temperature_K=temp, radiosity=j_rad[i],
            irradiation=g, net_heat_flux=flux,
            net_power=flux * surf.area))

    exchange = [[0.0] * n for _ in range(n)]
    for i in range(n):
        for j in range(n):
            if i != j:
                exchange[i][j] = (enc.surfaces[i].area
                                  * enc.view_factors[i][j]
                                  * (j_rad[i] - j_rad[j]))
    residual = sum(r.net_power for r in results)
    return CaseResult(name=case.name, surfaces=tuple(results),
                      exchange=tuple(tuple(row) for row in exchange),
                      energy_residual=residual)
