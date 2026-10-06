"""Core exact-distribution engine for finite haploid allele drift."""

from __future__ import annotations

import math
from dataclasses import dataclass

PROB_SUM_TOLERANCE = 1e-9


class ConfigError(ValueError):
    """Raised when a scenario configuration is invalid."""


@dataclass(frozen=True)
class Scenario:
    name: str
    population_size: int
    initial_count: int
    generations: int
    selection_coefficient: float
    mutation_to_target: float    # u: normal -> target
    mutation_from_target: float  # v: target -> normal


def _require_number(value, field, index):
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ConfigError(f"scenario {index}: '{field}' must be a number")
    if isinstance(value, float) and (math.isnan(value) or math.isinf(value)):
        raise ConfigError(f"scenario {index}: '{field}' must be finite")
    return value


def validate_config(raw, index=0):
    """Validate one raw scenario mapping and return a Scenario."""
    if not isinstance(raw, dict):
        raise ConfigError(f"scenario {index}: must be an object")
    required = (
        "population_size", "initial_count", "generations",
        "selection_coefficient", "mutation_to_target",
        "mutation_from_target",
    )
    for field in required:
        if field not in raw:
            raise ConfigError(f"scenario {index}: missing '{field}'")

    n = raw["population_size"]
    if isinstance(n, bool) or not isinstance(n, int) or n < 1:
        raise ConfigError(
            f"scenario {index}: 'population_size' must be an integer >= 1")

    i0 = raw["initial_count"]
    if isinstance(i0, bool) or not isinstance(i0, int) or not 0 <= i0 <= n:
        raise ConfigError(
            f"scenario {index}: 'initial_count' must be an integer in [0, N]")

    g = raw["generations"]
    if isinstance(g, bool) or not isinstance(g, int) or g < 0:
        raise ConfigError(
            f"scenario {index}: 'generations' must be an integer >= 0")

    s = float(_require_number(raw["selection_coefficient"],
                              "selection_coefficient", index))
    if not 1.0 + s > 0.0:
        raise ConfigError(
            f"scenario {index}: target fitness 1 + s must be positive")

    u = float(_require_number(raw["mutation_to_target"],
                              "mutation_to_target", index))
    v = float(_require_number(raw["mutation_from_target"],
                              "mutation_from_target", index))
    for field, p in (("mutation_to_target", u), ("mutation_from_target", v)):
        if not 0.0 <= p <= 1.0:
            raise ConfigError(
                f"scenario {index}: '{field}' must be in [0, 1]")

    name = raw.get("name", f"scenario_{index}")
    if not isinstance(name, str) or not name:
        raise ConfigError(f"scenario {index}: 'name' must be a non-empty string")

    return Scenario(name, n, i0, g, s, u, v)


def breeding_probability(count, scenario):
    """Post-selection, post-mutation probability an offspring is target."""
    n = scenario.population_size
    w_target = 1.0 + scenario.selection_coefficient
    selected = count * w_target / (count * w_target + (n - count))
    p = selected * (1.0 - scenario.mutation_from_target) \
        + (1.0 - selected) * scenario.mutation_to_target
    return p


def binomial_row(n, p):
    """Full Binomial(n, p) probability row, handling p == 0 and p == 1."""
    if p <= 0.0:
        row = [0.0] * (n + 1)
        row[0] = 1.0
        return row
    if p >= 1.0:
        row = [0.0] * (n + 1)
        row[n] = 1.0
        return row
    log_p = math.log(p)
    log_q = math.log1p(-p)
    return [
        math.exp(math.lgamma(n + 1) - math.lgamma(k + 1)
                 - math.lgamma(n - k + 1) + k * log_p + (n - k) * log_q)
        for k in range(n + 1)
    ]


def transition_matrix(scenario):
    """(N+1) x (N+1) matrix; entry [i][j] = P(next count j | current i)."""
    n = scenario.population_size
    matrix = []
    for i in range(n + 1):
        matrix.append(binomial_row(n, breeding_probability(i, scenario)))
    return matrix


def _matvec(dist, matrix):
    size = len(dist)
    out = [0.0] * size
    for i, di in enumerate(dist):
        if di == 0.0:
            continue
        row = matrix[i]
        for j in range(size):
            if row[j]:
                out[j] += di * row[j]
    return out


def run_scenario(scenario):
    """Propagate the exact distribution and boundary statistics.

    Returns a dict with a 'generations' list. Each entry holds the full
    count distribution, expected frequency, expected 2f(1-f), the
    probability-sum defect, the first-boundary-hit probability increment
    for that generation, the cumulative hit probability, and the
    probability of not having touched either boundary yet.
    """
    n = scenario.population_size
    matrix = transition_matrix(scenario)

    dist = [0.0] * (n + 1)
    dist[scenario.initial_count] = 1.0

    # 'untouched' is the sub-distribution over counts for lineages that
    # have never been at 0 or N. Boundaries are NOT absorbing: the total
    # distribution keeps evolving across them; only first hits are
    # recorded, so leaving and returning never double-counts.
    untouched = [0.0] * (n + 1)
    first_hit_zero = scenario.initial_count in (0, n)
    if not first_hit_zero:
        untouched[scenario.initial_count] = 1.0

    cumulative = 1.0 if first_hit_zero else 0.0
    results = []

    for gen in range(scenario.generations + 1):
        if gen > 0:
            dist = _matvec(dist, matrix)
            arrived = _matvec(untouched, matrix)
            increment = arrived[0] + arrived[n]
            arrived[0] = 0.0
            arrived[n] = 0.0
            untouched = arrived
            cumulative += increment
        else:
            increment = cumulative  # 1.0 if starting on a boundary else 0.0

        total = sum(dist)
        freq = sum(k * dist[k] for k in range(n + 1)) / n
        hetero = sum(
            dist[k] * 2.0 * (k / n) * (1.0 - k / n) for k in range(n + 1)
        )
        results.append({
            "generation": gen,
            "distribution": list(dist),
            "probability_sum": total,
            "probability_sum_error": abs(total - 1.0),
            "expected_frequency": freq,
            "expected_heterozygosity": hetero,
            "first_hit_increment": increment,
            "first_hit_cumulative": cumulative,
            "untouched_probability": sum(untouched),
        })

    max_error = max(r["probability_sum_error"] for r in results)
    if max_error > PROB_SUM_TOLERANCE:
        raise ArithmeticError(
            f"scenario '{scenario.name}': probability sum deviates from 1 "
            f"by {max_error:.3e}, exceeding tolerance {PROB_SUM_TOLERANCE}")

    return {"scenario": scenario, "generations": results,
            "max_probability_sum_error": max_error}
