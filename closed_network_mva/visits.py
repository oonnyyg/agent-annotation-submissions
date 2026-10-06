"""Visit-ratio computation for a closed routing network."""

from __future__ import annotations

from .model import ConfigError

_TOLERANCE = 1e-12
_MAX_ITERATIONS = 1_000_000


def compute_visit_ratios(routing: list[list[float]], reference_index: int) -> list[float]:
    """Solve v = v P with v[reference_index] = 1 via lazy-chain power iteration.

    A plain iteration v <- v P oscillates on periodic chains; averaging with the
    identity (lazy chain) guarantees convergence for any irreducible P while
    keeping the same stationary vector.
    """
    n = len(routing)
    if not 0 <= reference_index < n:
        raise ConfigError(f"reference index {reference_index} out of range for {n} stations")
    v = [0.0] * n
    v[reference_index] = 1.0
    for _ in range(_MAX_ITERATIONS):
        nxt = [0.0] * n
        for i in range(n):
            vi = v[i]
            if vi == 0.0:
                continue
            row = routing[i]
            nxt[i] += 0.5 * vi
            for j in range(n):
                p = row[j]
                if p > 0.0:
                    nxt[j] += 0.5 * vi * p
        scale = nxt[reference_index]
        if scale <= 0.0:
            raise ConfigError("reference station is not reachable from itself; "
                              "routing graph is not strongly connected")
        nxt = [x / scale for x in nxt]
        if max(abs(a - b) for a, b in zip(nxt, v)) < _TOLERANCE:
            return nxt
        v = nxt
    raise ConfigError("visit-ratio iteration did not converge; check the routing matrix")
