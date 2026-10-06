"""Terminal velocity solver balancing net gravity against drag."""
import math
from dataclasses import dataclass


class ConvergenceError(RuntimeError):
    """Raised when the terminal velocity iteration fails to converge."""


@dataclass(frozen=True)
class TerminalVelocityResult:
    velocity: float
    reynolds: float
    drag_coefficient: float
    relative_residual: float
    iterations: int


def drag_coefficient(reynolds):
    return 24.0 / reynolds + 3.0 / math.sqrt(reynolds) + 0.34


def forces(velocity, diameter, particle_density, fluid, gravity):
    reynolds = fluid.density * velocity * diameter / fluid.viscosity
    cd = drag_coefficient(reynolds)
    area = math.pi * diameter * diameter / 4.0
    drag = 0.5 * fluid.density * velocity * velocity * area * cd
    weight = (particle_density - fluid.density) * gravity * (
        math.pi * diameter**3 / 6.0
    )
    return drag, weight, reynolds, cd


def solve_terminal_velocity(
    diameter, particle_density, fluid, gravity, tol=1e-12, max_iter=200
):
    """Find the unique positive terminal velocity (guarded Newton method)."""

    def residual(v):
        drag, weight, _, _ = forces(v, diameter, particle_density, fluid, gravity)
        return drag - weight

    def derivative(v):
        h = max(v * 1e-7, 1e-12)
        return (residual(v + h) - residual(v - h)) / (2.0 * h)

    low = 0.0
    high = 1.0
    while residual(high) <= 0.0:
        high *= 2.0
        if high > 1e12:
            raise ConvergenceError("could not bracket the terminal velocity")

    v = 0.5 * (low + high)
    rel = math.inf
    for iteration in range(1, max_iter + 1):
        f_v = residual(v)
        if f_v > 0.0:
            high = v
        else:
            low = v
        try:
            step = f_v / derivative(v)
        except ZeroDivisionError:
            step = None
        candidate = v - step if step is not None else None
        if candidate is None or not (low < candidate < high):
            candidate = 0.5 * (low + high)
        v = candidate
        drag, weight, reynolds, cd = forces(
            v, diameter, particle_density, fluid, gravity
        )
        rel = abs(drag - weight) / weight
        if rel <= tol:
            return TerminalVelocityResult(
                velocity=v,
                reynolds=reynolds,
                drag_coefficient=cd,
                relative_residual=rel,
                iterations=iteration,
            )
    raise ConvergenceError(
        f"terminal velocity did not converge in {max_iter} iterations "
        f"(relative residual {rel:.3e})"
    )
