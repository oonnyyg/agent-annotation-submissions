"""Variable elimination inference."""

from __future__ import annotations

import itertools
from dataclasses import dataclass, field

from .factor import Factor, multiply, reduce_evidence, sum_out
from .model import BayesianNetwork


class QueryError(ValueError):
    """Raised for invalid queries or zero-probability evidence."""


@dataclass
class QueryResult:
    target: list[str]
    evidence: dict[str, str]
    evidence_probability: float
    distribution: dict[tuple[str, ...], float]
    map_assignment: dict[str, str]
    elimination_order: list[str]
    max_factor_size: int
    hidden_variables: list[str] = field(default_factory=list)


def _initial_factors(net: BayesianNetwork) -> list[Factor]:
    factors = []
    for var, cpt in net.cpts.items():
        vars_ = tuple(cpt.parents + [var])
        table = {
            key + (state,): prob
            for key, row in cpt.rows.items()
            for state, prob in row.items()
        }
        factors.append(Factor(vars=vars_, table=table))
    return factors


def _pick_variable(factors: list[Factor], candidates: list[str]) -> str:
    def product_size(var: str) -> int:
        size = 1
        for factor in factors:
            if var in factor.vars:
                size *= factor.size
        return size

    return min(candidates, key=lambda v: (product_size(v), v))


def query(
    net: BayesianNetwork,
    target: list[str],
    evidence: dict[str, str] | None = None,
    order: list[str] | None = None,
) -> QueryResult:
    evidence = dict(evidence or {})
    if not target:
        raise QueryError("query needs at least one target variable")
    if len(set(target)) != len(target):
        raise QueryError("duplicate target variables")
    for var in target:
        if var not in net.states:
            raise QueryError(f"unknown target variable {var!r}")
    overlap = set(target) & set(evidence)
    if overlap:
        raise QueryError(f"target and evidence overlap: {sorted(overlap)}")
    for var, state in evidence.items():
        if var not in net.states:
            raise QueryError(f"unknown evidence variable {var!r}")
        if state not in net.states[var]:
            raise QueryError(f"unknown state {state!r} for variable {var!r}")

    hidden = [v for v in net.states if v not in target and v not in evidence]
    if order is not None:
        if len(order) != len(hidden) or set(order) != set(hidden):
            raise QueryError("elimination order must cover each hidden variable exactly once")
        elim_order = list(order)
    else:
        elim_order = []

    factors = [reduce_evidence(f, evidence) for f in _initial_factors(net)]
    factors = [f for f in factors if f.vars]
    max_size = max((f.size for f in factors), default=1)

    remaining = list(hidden)
    while remaining:
        if order is not None:
            var = order[len(hidden) - len(remaining)]
        else:
            var = _pick_variable(factors, remaining)
            elim_order.append(var)
        involved = [f for f in factors if var in f.vars]
        factors = [f for f in factors if var not in f.vars]
        combined = involved[0]
        for other in involved[1:]:
            combined = multiply(combined, other)
        max_size = max(max_size, combined.size)
        reduced = sum_out(combined, var)
        max_size = max(max_size, reduced.size)
        if reduced.vars:
            factors.append(reduced)
        remaining.remove(var)

    result = factors[0]
    for other in factors[1:]:
        result = multiply(result, other)
    max_size = max(max_size, result.size)

    ordered_target = sorted(target)
    if result.vars != tuple(ordered_target):
        perm = [result.vars.index(v) for v in ordered_target]
        result = Factor(
            vars=tuple(ordered_target),
            table={tuple(key[i] for i in perm): v for key, v in result.table.items()},
        )

    evidence_probability = sum(result.table.values())
    if evidence_probability <= 0.0:
        raise QueryError("evidence has zero probability; cannot condition on it")

    distribution = {
        key: value / evidence_probability for key, value in sorted(result.table.items())
    }

    best_key = None
    best_value = -1.0
    for combo in itertools.product(*(sorted(net.states[v]) for v in ordered_target)):
        value = result.table.get(combo, 0.0)
        if value > best_value + 1e-12:
            best_value = value
            best_key = combo
    map_assignment = dict(zip(ordered_target, best_key))

    return QueryResult(
        target=list(target),
        evidence=evidence,
        evidence_probability=evidence_probability,
        distribution=distribution,
        map_assignment=map_assignment,
        elimination_order=elim_order,
        max_factor_size=max_size,
        hidden_variables=hidden,
    )
