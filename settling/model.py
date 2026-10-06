"""Core physics for still-water settling of non-interacting spherical particles.

All quantities use SI units: metres, kilograms, seconds.
"""
from __future__ import annotations

import json
import math
from dataclasses import dataclass, field


class ValidationError(ValueError):
    """Raised when scenario or sampling input data is invalid."""


class ConvergenceError(RuntimeError):
    """Raised when the terminal-velocity solve fails to converge."""


def _finite_positive(value, name):
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        raise ValidationError(f"{name} must be a number, got {value!r}")
    if not math.isfinite(value) or value <= 0.0:
        raise ValidationError(f"{name} must be finite and positive, got {value!r}")
    return float(value)


def _finite_nonnegative(value, name):
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        raise ValidationError(f"{name} must be a number, got {value!r}")
    if not math.isfinite(value) or value < 0.0:
        raise ValidationError(f"{name} must be finite and non-negative, got {value!r}")
    return float(value)


@dataclass(frozen=True)
class SizeClass:
    """One particle size class."""

    diameter: float  # particle diameter [m]
    density: float  # particle density [kg/m^3]
    mass: float  # initial total mass in the column [kg]


@dataclass(frozen=True)
class TerminalVelocityResult:
    """Outcome of the terminal-velocity solve for one size class."""

    velocity: float  # terminal settling velocity [m/s]
    reynolds: float  # particle Reynolds number [-]
    drag_coefficient: float  # Cd at the terminal velocity [-]
    relative_residual: float  # |drag - weight| / weight at the solution [-]
    iterations: int


def drag_coefficient(reynolds: float) -> float:
    """Cd = 24/Re + 3/sqrt(Re) + 0.34 (valid for Re > 0)."""
    return 24.0 / reynolds + 3.0 / math.sqrt(reynolds) + 0.34


def solve_terminal_velocity(
    diameter: float,
    particle_density: float,
    fluid_density: float,
    viscosity: float,
    gravity: float,
    *,
    tol: float = 1e-12,
    max_iterations: int = 200,
) -> TerminalVelocityResult:
    """Solve drag == submerged weight for the unique positive velocity.

    Force balance: 0.5 * rho_f * Cd(Re) * A * v^2 = (rho_p - rho_f) * g * V.
    The drag side is strictly increasing in v for v > 0, so a bisection on an
    expanding bracket converges to the unique positive root.
    """
    if particle_density <= fluid_density:
        raise ValidationError(
            "particle density must exceed fluid density for settling"
        )
    area = math.pi * diameter * diameter / 4.0
    volume = math.pi * diameter**3 / 6.0
    weight = (particle_density - fluid_density) * gravity * volume

    def residual(v: float) -> float:
        reynolds = fluid_density * v * diameter / viscosity
        drag = 0.5 * fluid_density * drag_coefficient(reynolds) * area * v * v
        return drag - weight

    lo = 0.0
    hi = 1.0
    while residual(hi) < 0.0:
        hi *= 2.0
        if hi > 1e12:
            raise ConvergenceError("could not bracket the terminal velocity")

    iterations = 0
    mid = hi
    rel = math.inf
    for iterations in range(1, max_iterations + 1):
        mid = 0.5 * (lo + hi)
        res = residual(mid)
        rel = abs(res) / weight
        if rel <= tol:
            break
        if res > 0.0:
            hi = mid
        else:
            lo = mid
    else:
        raise ConvergenceError(
            f"terminal velocity did not converge in {max_iterations} iterations "
            f"(relative residual {rel:.3e})"
        )

    reynolds = fluid_density * mid * diameter / viscosity
    return TerminalVelocityResult(
        velocity=mid,
        reynolds=reynolds,
        drag_coefficient=drag_coefficient(reynolds),
        relative_residual=abs(residual(mid)) / weight,
        iterations=iterations,
    )


@dataclass
class SettlingColumn:
    """Still-water column with initially uniform, non-interacting classes.

    Each class is uniformly distributed over the full depth at t = 0 and
    translates downward at its terminal velocity; particles reaching the
    bottom deposit instantly. No flocculation or particle interaction.
    """

    depth: float  # water depth H [m]
    area: float  # column cross-sectional area [m^2]
    fluid_density: float  # rho_f [kg/m^3]
    viscosity: float  # dynamic viscosity mu [Pa s]
    gravity: float  # g [m/s^2]
    classes: list  # list[SizeClass]
    _terminal: list = field(default=None, repr=False)

    def __post_init__(self):
        self.depth = _finite_positive(self.depth, "column depth")
        self.area = _finite_positive(self.area, "column area")
        self.fluid_density = _finite_positive(self.fluid_density, "fluid density")
        self.viscosity = _finite_positive(self.viscosity, "viscosity")
        self.gravity = _finite_positive(self.gravity, "gravity")
        if not self.classes:
            raise ValidationError("at least one size class is required")
        checked = []
        for i, cls in enumerate(self.classes):
            checked.append(
                SizeClass(
                    diameter=_finite_positive(cls.diameter, f"class {i} diameter"),
                    density=_finite_positive(cls.density, f"class {i} density"),
                    mass=_finite_positive(cls.mass, f"class {i} mass"),
                )
            )
        self.classes = checked
        self._terminal = [
            solve_terminal_velocity(
                cls.diameter,
                cls.density,
                self.fluid_density,
                self.viscosity,
                self.gravity,
            )
            for cls in self.classes
        ]

    @property
    def terminal(self):
        """list[TerminalVelocityResult] per size class."""
        return list(self._terminal)

    def _check_time(self, time):
        return _finite_nonnegative(time, "time")

    def suspended_mass(self, time):
        """Remaining suspended mass per class at `time` [kg]."""
        t = self._check_time(time)
        out = []
        for cls, term in zip(self.classes, self._terminal):
            fraction = max(0.0, 1.0 - term.velocity * t / self.depth)
            out.append(cls.mass * fraction)
        return out

    def deposited_mass(self, time):
        """Cumulative deposited mass per class at `time` [kg]."""
        suspended = self.suspended_mass(time)
        return [cls.mass - s for cls, s in zip(self.classes, suspended)]

    def mass_balance(self, time):
        """(suspended, deposited, total) per class; total stays constant."""
        suspended = self.suspended_mass(time)
        deposited = [cls.mass - s for cls, s in zip(self.classes, suspended)]
        totals = [s + d for s, d in zip(suspended, deposited)]
        return suspended, deposited, totals

    def concentration(self, time, z_top, z_bottom):
        """Mean mass concentration per class in [z_top, z_bottom] [kg/m^3].

        Depths are measured downward from the water surface. The interval
        must lie inside the column and have positive thickness. The result
        uses the overlap length between the translated suspended region
        [v*t, H] and the sampling interval, not just the interval centre.
        """
        t = self._check_time(time)
        z_top = _finite_nonnegative(z_top, "z_top")
        z_bottom = _finite_nonnegative(z_bottom, "z_bottom")
        if z_bottom <= z_top:
            raise ValidationError("sampling interval must have positive thickness")
        if z_bottom > self.depth:
            raise ValidationError(
                f"sampling interval [{z_top}, {z_bottom}] exceeds depth {self.depth}"
            )
        thickness = z_bottom - z_top
        out = []
        for cls, term in zip(self.classes, self._terminal):
            front = term.velocity * t  # top of the remaining suspended region
            overlap = max(0.0, min(z_bottom, self.depth) - max(z_top, front))
            mass_per_length = cls.mass / self.depth
            out.append(mass_per_length * overlap / (self.area * thickness))
        return out


def load_scenario(path):
    """Load a SettlingColumn from a JSON scenario file."""
    with open(path, "r", encoding="utf-8") as handle:
        data = json.load(handle)
    try:
        column = data["column"]
        fluid = data["fluid"]
        classes = data["classes"]
    except (KeyError, TypeError):
        raise ValidationError(
            "scenario JSON must define 'column', 'fluid' and 'classes'"
        ) from None
    try:
        size_classes = [
            SizeClass(
                diameter=entry["diameter"],
                density=entry["density"],
                mass=entry["mass"],
            )
            for entry in classes
        ]
        return SettlingColumn(
            depth=column["depth"],
            area=column["area"],
            fluid_density=fluid["density"],
            viscosity=fluid["viscosity"],
            gravity=fluid["gravity"],
            classes=size_classes,
        )
    except (KeyError, TypeError) as exc:
        raise ValidationError(f"missing scenario field: {exc}") from None
