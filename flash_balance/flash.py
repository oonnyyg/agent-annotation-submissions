"""Single-stage flash via the Rachford-Rice equation."""

from __future__ import annotations

from dataclasses import dataclass, field

from .errors import FlashBalanceError

TOLERANCE = 1e-12
MAX_ITERATIONS = 200


@dataclass
class FlashResult:
    """Outcome of flashing one feed stream with given K-values."""

    components: list[str]
    feed: dict[str, float]
    vapor_fraction: float
    phase: str  # "liquid", "vapor", "two-phase"
    degenerate: bool = False
    iterations: int = 0
    vapor: dict[str, float] = field(default_factory=dict)
    liquid: dict[str, float] = field(default_factory=dict)
    equilibrium_residual: float = 0.0


def _rachford_rice(psi: float, z: list[float], k: list[float]) -> float:
    return sum(
        zi * (ki - 1.0) / (1.0 + psi * (ki - 1.0))
        for zi, ki in zip(z, k)
        if zi > 0.0
    )


def flash_feed(
    feed: dict[str, float],
    k_values: dict[str, float],
    tolerance: float = TOLERANCE,
) -> FlashResult:
    """Flash a single mixed feed. ``feed`` maps component -> molar flow."""
    components = sorted(feed)
    active = [c for c in components if feed[c] > 0.0]
    if not active:
        raise FlashBalanceError("cannot flash an empty feed")
    for c in active:
        k = k_values.get(c)
        if k is None or k <= 0.0:
            raise FlashBalanceError(
                f"missing or non-positive K-value for component {c!r}"
            )

    total = sum(feed[c] for c in active)
    z = [feed[c] / total for c in active]
    k = [k_values[c] for c in active]

    degenerate = all(ki == 1.0 for ki in k)
    f0 = _rachford_rice(0.0, z, k)
    f1 = _rachford_rice(1.0, z, k)

    iterations = 0
    if degenerate or f0 <= 0.0:
        psi, phase = 0.0, "liquid"
    elif f1 >= 0.0:
        psi, phase = 1.0, "vapor"
    else:
        phase = "two-phase"
        psi, iterations = _solve(z, k, tolerance)

    vapor: dict[str, float] = {}
    liquid: dict[str, float] = {}
    for c in components:
        if feed[c] <= 0.0 or psi == 0.0:
            vapor[c] = 0.0
            liquid[c] = feed[c]
        elif psi == 1.0:
            vapor[c] = feed[c]
            liquid[c] = 0.0
        else:
            denom = 1.0 + psi * (k_values[c] - 1.0)
            vapor[c] = feed[c] * psi * k_values[c] / denom
            liquid[c] = feed[c] - vapor[c]

    residual = 0.0
    if phase == "two-phase":
        liquid_total = sum(liquid[a] for a in active)
        vapor_total = sum(vapor[a] for a in active)
        for c in active:
            if liquid[c] > 0.0 and vapor[c] > 0.0:
                x = liquid[c] / liquid_total
                y = vapor[c] / vapor_total
                residual = max(residual, abs(y - k_values[c] * x))

    return FlashResult(
        components=components,
        feed=dict(feed),
        vapor_fraction=psi,
        phase=phase,
        degenerate=degenerate,
        iterations=iterations,
        vapor=vapor,
        liquid=liquid,
        equilibrium_residual=residual,
    )


def _solve(z: list[float], k: list[float], tol: float) -> tuple[float, int]:
    """Bisection on (0, 1); robust for K-values near one."""
    lo, hi = 0.0, 1.0
    flo = _rachford_rice(lo, z, k)
    iterations = 0
    while hi - lo > tol and iterations < MAX_ITERATIONS:
        mid = 0.5 * (lo + hi)
        fm = _rachford_rice(mid, z, k)
        if fm == 0.0:
            return mid, iterations
        if (flo < 0.0) == (fm < 0.0):
            lo, flo = mid, fm
        else:
            hi = mid
        iterations += 1
    if hi - lo > tol:
        raise FlashBalanceError("Rachford-Rice solver did not converge")
    return 0.5 * (lo + hi), iterations
