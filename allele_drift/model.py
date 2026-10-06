"""Exact discrete-time Wright-Fisher style model for a haploid population.

Each generation: selection first, then two-way mutation. The next
generation's target count is Binomial(N, p) where p combines the
fitness-weighted reproduction probability with mutation in both
directions. Probabilities are propagated exactly with the full
(N+1) x (N+1) transition matrix; no state is ever pruned.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from math import comb


@dataclass
class Scenario:
    name: str
    population_size: int
    initial_count: int
    generations: int
    selection_coefficient: float
    mutation_to_target: float
    mutation_to_ordinary: float


@dataclass
class GenerationRow:
    generation: int
    probabilities: list[float]
    expected_frequency: float
    expected_heterozygosity: float
    probability_sum: float
    prob_untouched_boundary: float
    boundary_hit_increment: float
    boundary_hit_cumulative: float


@dataclass
class ScenarioResult:
    scenario: Scenario
    name: str
    rows: list[GenerationRow] = field(default_factory=list)
    max_sum_error: float = 0.0


def _validate(raw: dict) -> Scenario:
    if not isinstance(raw, dict):
        raise ValueError("scenario must be an object")
    try:
        name = str(raw.get("name", "scenario"))
        n = int(raw["population_size"])
        i0 = int(raw["initial_count"])
        g = int(raw["generations"])
        s = float(raw.get("selection_coefficient", 0.0))
        u = float(raw.get("mutation_to_target", 0.0))
        v = float(raw.get("mutation_to_ordinary", 0.0))
    except (KeyError, TypeError, ValueError) as exc:
        raise ValueError(f"missing or invalid field: {exc}") from exc
    if n < 1:
        raise ValueError("population_size must be >= 1")
    if not 0 <= i0 <= n:
        raise ValueError("initial_count must be within [0, population_size]")
    if g < 0:
        raise ValueError("generations must be >= 0")
    if 1.0 + s <= 0.0:
        raise ValueError("target fitness (1 + selection_coefficient) must be positive")
    for label, rate in (("mutation_to_target", u), ("mutation_to_ordinary", v)):
        if not 0.0 <= rate <= 1.0:
            raise ValueError(f"{label} must be within [0, 1]")
    return Scenario(name, n, i0, g, s, u, v)


def _reproduction_probability(count: int, n: int, s: float, u: float, v: float) -> float:
    w_target = 1.0 + s
    selected = (count * w_target) / (count * w_target + (n - count))
    return selected * (1.0 - v) + (1.0 - selected) * u


def _binomial_row(n: int, p: float) -> list[float]:
    if p <= 0.0:
        row = [0.0] * (n + 1)
        row[0] = 1.0
        return row
    if p >= 1.0:
        row = [0.0] * (n + 1)
        row[n] = 1.0
        return row
    return [comb(n, j) * p**j * (1.0 - p) ** (n - j) for j in range(n + 1)]


def build_transition_matrix(sc: Scenario) -> list[list[float]]:
    n = sc.population_size
    return [
        _binomial_row(
            n,
            _reproduction_probability(
                i, n, sc.selection_coefficient,
                sc.mutation_to_target, sc.mutation_to_ordinary,
            ),
        )
        for i in range(n + 1)
    ]


def run_scenario(raw: dict) -> ScenarioResult:
    sc = _validate(raw)
    n = sc.population_size
    matrix = build_transition_matrix(sc)

    dist = [0.0] * (n + 1)
    dist[sc.initial_count] = 1.0
    untouched = [0.0] * (n + 1)
    if 0 < sc.initial_count < n:
        untouched[sc.initial_count] = 1.0
    cumulative_hit = 1.0 if (sc.initial_count == 0 or sc.initial_count == n) else 0.0

    result = ScenarioResult(scenario=sc, name=sc.name)

    def record(gen: int, increment: float) -> None:
        total = sum(dist)
        result.max_sum_error = max(result.max_sum_error, abs(total - 1.0))
        freq = sum(j * dist[j] for j in range(n + 1)) / n
        het = sum(2.0 * (j / n) * (1.0 - j / n) * dist[j] for j in range(n + 1))
        result.rows.append(
            GenerationRow(
                generation=gen,
                probabilities=list(dist),
                expected_frequency=freq,
                expected_heterozygosity=het,
                probability_sum=total,
                prob_untouched_boundary=sum(untouched),
                boundary_hit_increment=increment,
                boundary_hit_cumulative=cumulative_hit,
            )
        )

    record(0, cumulative_hit)

    for gen in range(1, sc.generations + 1):
        dist = [
            sum(dist[i] * matrix[i][j] for i in range(n + 1))
            for j in range(n + 1)
        ]
        next_untouched = [0.0] * (n + 1)
        for j in range(1, n):
            next_untouched[j] = sum(untouched[i] * matrix[i][j] for i in range(1, n))
        increment = sum(untouched) - sum(next_untouched)
        untouched = next_untouched
        cumulative_hit += increment
        record(gen, increment)

    return result
