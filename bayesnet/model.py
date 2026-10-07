"""Network definition loading and validation."""

from __future__ import annotations

import itertools
import json
from dataclasses import dataclass

MAX_VARIABLES = 12
MAX_STATES = 4
SUM_TOLERANCE = 1e-9


class ModelError(ValueError):
    """Raised when the network definition is invalid."""


@dataclass
class CPT:
    variable: str
    parents: list[str]
    # maps parent-state tuple (in parents order) -> {state: prob}
    rows: dict[tuple[str, ...], dict[str, float]]


@dataclass
class BayesianNetwork:
    states: dict[str, list[str]]
    cpts: dict[str, CPT]

    def parents_of(self, var: str) -> list[str]:
        return self.cpts[var].parents


def _check_dag(cpts: dict[str, CPT]) -> None:
    visiting: set[str] = set()
    done: set[str] = set()

    def visit(node: str, stack: list[str]) -> None:
        if node in done:
            return
        if node in visiting:
            cycle = " -> ".join(stack + [node])
            raise ModelError(f"graph contains a cycle: {cycle}")
        visiting.add(node)
        for parent in cpts[node].parents:
            visit(parent, stack + [node])
        visiting.discard(node)
        done.add(node)

    for var in cpts:
        visit(var, [])


def load_network(path: str) -> BayesianNetwork:
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    return parse_network(data)


def parse_network(data: dict) -> BayesianNetwork:
    if not isinstance(data, dict):
        raise ModelError("network definition must be a JSON object")
    raw_vars = data.get("variables")
    raw_cpts = data.get("cpt")
    if not isinstance(raw_vars, dict) or not isinstance(raw_cpts, dict):
        raise ModelError("definition requires 'variables' and 'cpt' objects")
    if not raw_vars:
        raise ModelError("at least one variable is required")
    if len(raw_vars) > MAX_VARIABLES:
        raise ModelError(f"at most {MAX_VARIABLES} variables are allowed")

    states: dict[str, list[str]] = {}
    for var, var_states in raw_vars.items():
        if not isinstance(var_states, list) or not var_states:
            raise ModelError(f"variable {var!r} needs a non-empty state list")
        if len(var_states) > MAX_STATES:
            raise ModelError(f"variable {var!r} has more than {MAX_STATES} states")
        if len(set(var_states)) != len(var_states):
            raise ModelError(f"variable {var!r} has duplicate states")
        if not all(isinstance(s, str) for s in var_states):
            raise ModelError(f"variable {var!r} states must be strings")
        states[var] = list(var_states)

    if set(raw_cpts) != set(states):
        missing = set(states) - set(raw_cpts)
        extra = set(raw_cpts) - set(states)
        raise ModelError(f"cpt keys mismatch variables (missing={sorted(missing)}, extra={sorted(extra)})")

    cpts: dict[str, CPT] = {}
    for var, spec in raw_cpts.items():
        parents = spec.get("parents", [])
        if not isinstance(parents, list) or len(set(parents)) != len(parents):
            raise ModelError(f"cpt for {var!r} has invalid or duplicate parents")
        for parent in parents:
            if parent not in states:
                raise ModelError(f"cpt for {var!r} references unknown parent {parent!r}")
            if parent == var:
                raise ModelError(f"cpt for {var!r} lists itself as a parent")

        rows: dict[tuple[str, ...], dict[str, float]] = {}
        for row in spec.get("rows", []):
            pstates = row.get("parent_states", {})
            if set(pstates) != set(parents):
                raise ModelError(f"cpt row for {var!r} does not match its parents")
            key = tuple(pstates[p] for p in parents)
            for p, value in pstates.items():
                if value not in states[p]:
                    raise ModelError(f"cpt row for {var!r} uses unknown state {value!r} of {p!r}")
            if key in rows:
                raise ModelError(f"cpt for {var!r} has a duplicate parent-state row")
            probs = row.get("probs", {})
            if set(probs) != set(states[var]):
                raise ModelError(f"cpt row for {var!r} must list exactly its states")
            values = {}
            for state, prob in probs.items():
                if not isinstance(prob, (int, float)) or isinstance(prob, bool):
                    raise ModelError(f"probability for {var!r}={state!r} is not a number")
                if prob < 0:
                    raise ModelError(f"probability for {var!r}={state!r} is negative")
                values[state] = float(prob)
            if abs(sum(values.values()) - 1.0) > SUM_TOLERANCE:
                raise ModelError(f"probabilities for {var!r} given {key} do not sum to 1")
            rows[key] = values

        expected = set(itertools.product(*(states[p] for p in parents)))
        if set(rows) != expected:
            missing = sorted(expected - set(rows))
            raise ModelError(f"cpt for {var!r} is missing parent combinations: {missing}")
        cpts[var] = CPT(variable=var, parents=list(parents), rows=rows)

    _check_dag(cpts)
    return BayesianNetwork(states=states, cpts=cpts)
