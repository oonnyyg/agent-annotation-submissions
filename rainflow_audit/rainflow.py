"""Three-point stack rainflow counting."""

from __future__ import annotations

from dataclasses import dataclass

from .turning_points import Point


@dataclass(frozen=True)
class Cycle:
    """One counted cycle with provenance (original endpoint times)."""

    stress_range: float
    mean: float
    count: float
    start_time: float
    end_time: float

    @property
    def amplitude(self) -> float:
        return self.stress_range / 2.0


def count_cycles(turning_points: list[Point]) -> list[Cycle]:
    """Count cycles with the three-point stack rule."""
    stack: list[Point] = []
    cycles: list[Cycle] = []
    for point in turning_points:
        stack.append(point)
        while len(stack) >= 3:
            prev_range = abs(stack[-2].stress - stack[-3].stress)
            new_range = abs(stack[-1].stress - stack[-2].stress)
            if new_range < prev_range:
                break
            if len(stack) == 3:
                cycles.append(_make_cycle(stack[0], stack[1], 0.5))
                stack.pop(0)
            else:
                cycles.append(_make_cycle(stack[-3], stack[-2], 1.0))
                del stack[-3:-1]
    for index in range(len(stack) - 1):
        cycles.append(_make_cycle(stack[index], stack[index + 1], 0.5))
    return cycles


def _make_cycle(first: Point, second: Point, count: float) -> Cycle:
    return Cycle(
        stress_range=abs(second.stress - first.stress),
        mean=(first.stress + second.stress) / 2.0,
        count=count,
        start_time=first.time,
        end_time=second.time,
    )
