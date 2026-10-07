"""Displacement-based Newton solver for a 1D series bar (stdlib only)."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .material import MaterialParams, MaterialState, return_mapping


@dataclass
class Element:
    length: float
    area: float
    material: MaterialParams

    def validate(self, index: int) -> None:
        for name, value in (("length", self.length), ("area", self.area)):
            if not isinstance(value, (int, float)) or not math.isfinite(value):
                raise ValueError(f"element {index}: {name} must be finite, got {value!r}")
            if value <= 0.0:
                raise ValueError(f"element {index}: {name} must be positive, got {value!r}")
        self.material.validate()


@dataclass
class ElementResult:
    strain: float
    stress: float
    eps_p: float
    alpha: float
    axial_force: float
    tangent: float


@dataclass
class StepResult:
    step: int
    prescribed_displacement: float
    converged: bool
    iterations: int
    node_displacements: list[float]
    reaction: float
    elements: list[ElementResult]
    force_imbalance: float
    compatibility_residual: float


def _solve_tridiagonal(
    lower: list[float], diag: list[float], upper: list[float], rhs: list[float]
) -> list[float]:
    """Thomas algorithm for a tridiagonal system."""
    n = len(diag)
    c = upper[:]
    d = rhs[:]
    for i in range(1, n):
        factor = lower[i] / diag[i - 1]
        diag[i] -= factor * c[i - 1]
        d[i] -= factor * d[i - 1]
    x = [0.0] * n
    x[-1] = d[-1] / diag[-1]
    for i in range(n - 2, -1, -1):
        x[i] = (d[i] - c[i] * x[i + 1]) / diag[i]
    return x


@dataclass
class BarModel:
    """Series bar with converged material history and nodal displacements."""

    elements: list[Element]
    states: list[MaterialState] = field(init=False)
    converged_u: list[float] = field(init=False)

    def __post_init__(self) -> None:
        if not self.elements:
            raise ValueError("model must contain at least one element")
        for i, el in enumerate(self.elements):
            el.validate(i)
        self.states = [MaterialState() for _ in self.elements]
        self.converged_u = [0.0] * (len(self.elements) + 1)

    def solve_step(
        self,
        step: int,
        u_end: float,
        tol_force: float,
        tol_disp: float,
        max_iter: int,
    ) -> StepResult:
        if not math.isfinite(u_end):
            raise ValueError(f"step {step}: prescribed displacement must be finite")
        n_el = len(self.elements)
        n_unknown = n_el - 1
        u = list(self.converged_u)
        u[-1] = u_end

        converged = False
        iterations = 0
        trial_states = [s.copy() for s in self.states]
        stresses = [0.0] * n_el
        tangents = [0.0] * n_el
        residual = [0.0] * max(n_unknown, 1)

        def evaluate() -> float:
            for e, el in enumerate(self.elements):
                strain = (u[e + 1] - u[e]) / el.length
                stress, tangent, trial_states[e] = return_mapping(
                    strain, self.states[e], el.material
                )
                stresses[e] = stress
                tangents[e] = tangent
            for i in range(n_unknown):
                node = i + 1
                residual[i] = (
                    self.elements[node - 1].area * stresses[node - 1]
                    - self.elements[node].area * stresses[node]
                )
            return max((abs(r) for r in residual), default=0.0)

        for iterations in range(1, max_iter + 1):
            res_norm = evaluate()
            if n_unknown == 0 or res_norm <= tol_force:
                converged = True
                break

            lower = [0.0] * n_unknown
            diag = [0.0] * n_unknown
            upper = [0.0] * n_unknown
            for i in range(n_unknown):
                node = i + 1
                k_left = (
                    self.elements[node - 1].area
                    * tangents[node - 1]
                    / self.elements[node - 1].length
                )
                k_right = (
                    self.elements[node].area
                    * tangents[node]
                    / self.elements[node].length
                )
                lower[i] = -k_left if i > 0 else 0.0
                diag[i] = k_left + k_right
                upper[i] = -k_right if i < n_unknown - 1 else 0.0

            delta = _solve_tridiagonal(lower, diag, upper, [-r for r in residual])
            # Damped line search: shrink the step until the residual decreases.
            scale = 1.0
            base_u = u[:]
            for _ in range(30):
                for i in range(n_unknown):
                    u[i + 1] = base_u[i + 1] + scale * delta[i]
                new_norm = evaluate()
                if new_norm < res_norm or scale < 1e-10:
                    break
                scale *= 0.5
            if max(abs(scale * d) for d in delta) <= tol_disp:
                if evaluate() <= tol_force:
                    converged = True
                    break

        if not converged:
            return StepResult(
                step=step,
                prescribed_displacement=u_end,
                converged=False,
                iterations=iterations,
                node_displacements=[],
                reaction=0.0,
                elements=[],
                force_imbalance=max(abs(r) for r in residual),
                compatibility_residual=0.0,
            )

        # Commit: history variables advance only after global convergence.
        self.states = trial_states
        self.converged_u = list(u)

        element_results = []
        for e, el in enumerate(self.elements):
            strain = (u[e + 1] - u[e]) / el.length
            element_results.append(
                ElementResult(
                    strain=strain,
                    stress=stresses[e],
                    eps_p=trial_states[e].eps_p,
                    alpha=trial_states[e].alpha,
                    axial_force=el.area * stresses[e],
                    tangent=tangents[e],
                )
            )

        force_imbalance = max(
            (abs(element_results[i].axial_force - element_results[i + 1].axial_force)
             for i in range(n_el - 1)),
            default=0.0,
        )
        total_elongation = sum(
            element_results[e].strain * self.elements[e].length for e in range(n_el)
        )
        compatibility_residual = abs(total_elongation - u_end)

        return StepResult(
            step=step,
            prescribed_displacement=u_end,
            converged=True,
            iterations=iterations,
            node_displacements=list(u),
            reaction=-element_results[0].axial_force,
            elements=element_results,
            force_imbalance=force_imbalance,
            compatibility_residual=compatibility_residual,
        )
