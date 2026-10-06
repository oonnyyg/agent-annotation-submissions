"""Radiosity solver for a gray, diffuse, opaque enclosure.

Unknowns are the radiosities J_i (W/m^2). For a surface with a fixed
temperature T_i:

    J_i - (1 - eps_i) * sum_j F_ij J_j = eps_i * sigma * T_i^4

For an adiabatic surface (zero net radiative flux, J_i = G_i):

    J_i - sum_j F_ij J_j = 0

Solving this linear system accounts for all reflections inside the
enclosure. Adiabatic temperatures are recovered as T = (J/sigma)^(1/4).
"""

from __future__ import annotations

from dataclasses import dataclass

from .model import Case, Enclosure, InputError

STEFAN_BOLTZMANN = 5.670374419e-8  # W/(m^2 K^4), exact SI value


class UnderconstrainedError(InputError):
    """A radiatively connected component has no fixed-temperature surface."""


@dataclass
class SurfaceResult:
    name: str
    area: float
    emissivity: float
    adiabatic: bool
    temperature: float          # K (solved for adiabatic surfaces)
    radiosity: float            # J, W/m^2
    irradiation: float          # G, W/m^2
    net_heat_flux: float        # q, W/m^2, positive = leaving the surface
    net_power: float            # q * A, W


@dataclass
class CaseResult:
    name: str
    surfaces: list[SurfaceResult]
    pair_powers: list[list[float]]  # [i][j] = net power from i to j, W
    residuals: list[float]          # per-surface energy balance residual, W
    global_residual: float          # sum of net powers, ~0, W


def connected_components(view_factors: list[list[float]]) -> list[list[int]]:
    n = len(view_factors)
    seen = [False] * n
    components: list[list[int]] = []
    for start in range(n):
        if seen[start]:
            continue
        stack = [start]
        seen[start] = True
        comp = []
        while stack:
            i = stack.pop()
            comp.append(i)
            for j in range(n):
                if not seen[j] and (view_factors[i][j] > 0.0 or view_factors[j][i] > 0.0):
                    seen[j] = True
                    stack.append(j)
        components.append(sorted(comp))
    return components


def _solve_linear(matrix: list[list[float]], rhs: list[float]) -> list[float]:
    """Gaussian elimination with partial pivoting (no external deps)."""
    n = len(matrix)
    a = [row[:] + [rhs[i]] for i, row in enumerate(matrix)]
    for col in range(n):
        pivot = max(range(col, n), key=lambda r: abs(a[r][col]))
        if abs(a[pivot][col]) < 1e-300:
            raise InputError("radiosity system is singular; check view factors and inputs")
        a[col], a[pivot] = a[pivot], a[col]
        inv = 1.0 / a[col][col]
        for r in range(col + 1, n):
            factor = a[r][col] * inv
            if factor == 0.0:
                continue
            for c in range(col, n + 1):
                a[r][c] -= factor * a[col][c]
    x = [0.0] * n
    for i in range(n - 1, -1, -1):
        s = a[i][n] - sum(a[i][j] * x[j] for j in range(i + 1, n))
        x[i] = s / a[i][i]
    return x


def solve_case(enclosure: Enclosure, case: Case) -> CaseResult:
    surfaces = enclosure.surfaces
    vf = enclosure.view_factors
    n = len(surfaces)
    temps = case.temperatures

    for comp in connected_components(vf):
        if all(temps[i] is None for i in comp):
            names = ", ".join(repr(surfaces[i].name) for i in comp)
            raise UnderconstrainedError(
                f"case {case.name!r}: radiatively connected component "
                f"({names}) has no fixed-temperature surface; "
                "the problem is under-constrained")

    matrix = [[0.0] * n for _ in range(n)]
    rhs = [0.0] * n
    for i in range(n):
        eps = surfaces[i].emissivity
        for j in range(n):
            if temps[i] is None:
                matrix[i][j] = (1.0 if i == j else 0.0) - vf[i][j]
            else:
                matrix[i][j] = (1.0 if i == j else 0.0) - (1.0 - eps) * vf[i][j]
        if temps[i] is not None:
            rhs[i] = eps * STEFAN_BOLTZMANN * temps[i] ** 4

    radiosity = _solve_linear(matrix, rhs)

    results: list[SurfaceResult] = []
    for i, surf in enumerate(surfaces):
        irradiation = sum(vf[i][j] * radiosity[j] for j in range(n))
        if temps[i] is None:
            temperature = (radiosity[i] / STEFAN_BOLTZMANN) ** 0.25
            flux = 0.0
        elif surf.emissivity < 1.0:
            temperature = temps[i]
            flux = surf.emissivity / (1.0 - surf.emissivity) \
                * (STEFAN_BOLTZMANN * temperature ** 4 - radiosity[i])
        else:
            temperature = temps[i]
            flux = STEFAN_BOLTZMANN * temperature ** 4 - irradiation
        results.append(SurfaceResult(
            name=surf.name, area=surf.area, emissivity=surf.emissivity,
            adiabatic=temps[i] is None, temperature=temperature,
            radiosity=radiosity[i], irradiation=irradiation,
            net_heat_flux=flux, net_power=flux * surf.area))

    pair_powers = [[0.0] * n for _ in range(n)]
    for i in range(n):
        for j in range(n):
            if i != j:
                pair_powers[i][j] = surfaces[i].area * vf[i][j] \
                    * (radiosity[i] - radiosity[j])

    residuals = []
    for i in range(n):
        balance = results[i].net_power - sum(pair_powers[i][j] for j in range(n))
        residuals.append(balance)
    global_residual = sum(r.net_power for r in results)

    return CaseResult(name=case.name, surfaces=results, pair_powers=pair_powers,
                      residuals=residuals, global_residual=global_residual)
