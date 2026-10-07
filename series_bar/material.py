"""Linear isotropic hardening material model (1D, small strain)."""

from __future__ import annotations

import math
from dataclasses import dataclass


@dataclass
class MaterialParams:
    youngs_modulus: float
    yield_strength: float
    hardening_modulus: float

    def validate(self) -> None:
        for name, value in (
            ("youngs_modulus", self.youngs_modulus),
            ("yield_strength", self.yield_strength),
            ("hardening_modulus", self.hardening_modulus),
        ):
            if not isinstance(value, (int, float)) or not math.isfinite(value):
                raise ValueError(f"material parameter {name!r} must be finite, got {value!r}")
            if value <= 0.0:
                raise ValueError(f"material parameter {name!r} must be positive, got {value!r}")


@dataclass
class MaterialState:
    """Converged history variables for one element."""

    eps_p: float = 0.0
    alpha: float = 0.0

    def copy(self) -> "MaterialState":
        return MaterialState(self.eps_p, self.alpha)


def return_mapping(
    strain: float,
    state: MaterialState,
    params: MaterialParams,
) -> tuple[float, float, MaterialState]:
    """Return (stress, consistent tangent, trial state).

    ``state`` is the converged state of the previous load step and is never
    mutated; the returned trial state is committed by the caller only after
    the global Newton iteration converges.
    """
    e = params.youngs_modulus
    h = params.hardening_modulus

    stress_trial = e * (strain - state.eps_p)
    yield_trial = params.yield_strength + h * state.alpha
    f_trial = abs(stress_trial) - yield_trial

    if f_trial <= 0.0:
        return stress_trial, e, state.copy()

    d_gamma = f_trial / (e + h)
    direction = 1.0 if stress_trial > 0.0 else -1.0
    stress = stress_trial - d_gamma * e * direction
    new_state = MaterialState(
        eps_p=state.eps_p + d_gamma * direction,
        alpha=state.alpha + d_gamma,
    )
    tangent = e * h / (e + h)
    return stress, tangent, new_state
