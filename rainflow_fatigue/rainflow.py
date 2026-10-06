"""Three-point stack rainflow counting."""

from __future__ import annotations

from dataclasses import dataclass

from .turning_points import Point


@dataclass(frozen=True)
class Cycle:
    """One counted cycle with provenance (original endpoint times)."""

    start_time: float
    end_time: float
    stress_range: float
    mean: float
    count: float  # 0.5 half cycle, 1.0 full cycle

    @property
    def amplitude(self) -> float:
        return self.stress_range / 2.0


def _make_cycle(first: Point, second: Point, count: float) -> Cycle:
    return Cycle(
        start_time=first.time,
        end_time=second.time,
        stress_range=abs(second.stress - first.stress),
        mean=(first.stress + second.stress) / 2.0,
        count=count,
    )


def count_cycles(turning_points: list[Point]) -> list[Cycle]:
    """Count cycles from a turning-point sequence.

    Compare the last range X with the previous range Y. If X < Y keep
    pushing; otherwise close a cycle on Y. With exactly three points on
    the stack it is a half cycle and the oldest point is removed; with
    more than three points it is a full cycle and both endpoints of Y
    are removed. Repeat the comparison. Leftover adjacent pairs each
    count as a half cycle (residuals are kept, never dropped/doubled).
    """
    cycles: list[Cycle] = []
    stack: list[Point] = []

    for point in turning_points:
        stack.append(point)
        while len(stack) >= 3:
            x = abs(stack[-1].stress - stack[-2].stress)
            y = abs(stack[-2].stress - stack[-3].stress)
            if x < y:
                break
            if len(stack) == 3:
                cycles.append(_make_cycle(stack[0], stack[1], 0.5))
                del stack[0]
            else:
                cycles.append(_make_cycle(stack[-3], stack[-2], 1.0))
                del stack[-3:-1]

    for index in range(len(stack) - 1):
        cycles.append(_make_cycle(stack[index], stack[index + 1], 0.5))

    return cycles
