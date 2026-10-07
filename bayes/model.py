"""Load and validate a discrete Bayesian network from JSON."""

from __future__ import annotations

import json
from dataclasses import dataclass

MAX_VARIABLES = 12
MAX_STATES = 4
PROB_SUM_TOL = 1e-9


class ModelError(ValueError):
    """Raised when the network definition is invalid."""


@dataclass
class Variable:
    name: str
    states: list[str]
    parents: list[str]
    # cpt[(parent_state_tuple)] -> {state: prob}; () key when no parents.
    cpt: dict[tuple[str, ...], dict[str, float]]


@dataclass
class Network:
    variables: dict[str, Variable]
    order: list[str]  # topological order of variable names


def load_network(path: str) -> Network:
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    return parse_network(data)


def parse_network(data: dict) -> Network:
    if not isinstance(data, dict) or "variables" not in data:
        raise ModelError("network JSON must contain a 'variables' list")
    raw_vars = data["variables"]
    if not isinstance(raw_vars, list) or not raw_vars:
        raise ModelError("'variables' must be a non-empty list")
    if len(raw_vars) > MAX_VARIABLES:
        raise ModelError(f"at most {MAX_VARIABLES} variables are allowed")

    variables: dict[str, Variable] = {}
    for entry in raw_vars:
        variables[entry["name"]] = _parse_variable(entry)

    for var in variables.values():
        for parent in var.parents:
            if parent not in variables:
                raise ModelError(f"{var.name}: unknown parent '{parent}'")
        _validate_cpt(var, variables)

    order = _topological_order(variables)
    return Network(variables=variables, order=order)


def _parse_variable(entry: dict) -> Variable:
    name = entry.get("name")
    if not isinstance(name, str) or not name:
        raise ModelError("each variable needs a non-empty string 'name'")
    states = entry.get("states")
    if not isinstance(states, list) or not states:
        raise ModelError(f"{name}: 'states' must be a non-empty list")
    if len(states) > MAX_STATES:
        raise ModelError(f"{name}: at most {MAX_STATES} states allowed")
    if len(set(states)) != len(states):
        raise ModelError(f"{name}: duplicate states are not allowed")
    parents = entry.get("parents", [])
    if not isinstance(parents, list) or len(set(parents)) != len(parents):
        raise ModelError(f"{name}: 'parents' must be a list without duplicates")
    if name in parents:
        raise ModelError(f"{name}: a variable cannot be its own parent")
    cpt = entry.get("cpt")
    if not isinstance(cpt, list) or not cpt:
        raise ModelError(f"{name}: 'cpt' must be a non-empty list of rows")
    return Variable(name=name, states=list(states), parents=list(parents),
                    cpt=_rows_to_cpt(name, parents, cpt))


def _rows_to_cpt(name, parents, rows):
    cpt: dict[tuple[str, ...], dict[str, float]] = {}
    for row in rows:
        key = tuple(row.get("when", []))
        if len(key) != len(parents):
            raise ModelError(f"{name}: 'when' must list one state per parent")
        if key in cpt:
            raise ModelError(f"{name}: duplicate CPT row for parent states {key}")
        probs = row.get("probs")
        if not isinstance(probs, dict):
            raise ModelError(f"{name}: each row needs a 'probs' object")
        cpt[key] = {str(k): float(v) for k, v in probs.items()}
    return cpt


def _validate_cpt(var: Variable, variables: dict[str, Variable]) -> None:
    parent_states = [variables[p].states for p in var.parents]
    expected = _cartesian(parent_states)
    missing = [key for key in expected if key not in var.cpt]
    if missing:
        raise ModelError(f"{var.name}: missing CPT rows for parent states {missing}")
    for key, probs in var.cpt.items():
        for parent, state in zip(var.parents, key):
            if state not in variables[parent].states:
                raise ModelError(f"{var.name}: unknown state '{state}' for parent '{parent}'")
        if set(probs) != set(var.states):
            raise ModelError(f"{var.name}: CPT row {key} must give exactly its states")
        if any(p < 0 for p in probs.values()):
            raise ModelError(f"{var.name}: negative probability in row {key}")
        if abs(sum(probs.values()) - 1.0) > PROB_SUM_TOL:
            raise ModelError(f"{var.name}: probabilities in row {key} do not sum to 1")


def _cartesian(lists):
    keys = [()]
    for states in lists:
        keys = [k + (s,) for k in keys for s in states]
    return keys


def _topological_order(variables: dict[str, Variable]) -> list[str]:
    order: list[str] = []
    done: set[str] = set()
    remaining = dict(variables)
    while remaining:
        ready = [n for n, v in remaining.items() if all(p in done for p in v.parents)]
        if not ready:
            raise ModelError("network contains a directed cycle")
        for name in ready:
            order.append(name)
            done.add(name)
            del remaining[name]
    return order
