"""Variable elimination over discrete factors."""

from __future__ import annotations

from dataclasses import dataclass, field

from .model import Network


class QueryError(ValueError):
    """Raised for invalid queries or impossible evidence."""


@dataclass
class Factor:
    scope: tuple[str, ...]  # variable names, sorted
    # table[assignment tuple aligned with scope] -> probability
    table: dict[tuple[str, ...], float] = field(default_factory=dict)

    @property
    def size(self) -> int:
        return len(self.table)


def factor_from_cpt(net: Network, name: str) -> Factor:
    var = net.variables[name]
    scope = tuple(sorted([name, *var.parents]))
    table: dict[tuple[str, ...], float] = {}
    for key, probs in var.cpt.items():
        for state, prob in probs.items():
            assignment = dict(zip(var.parents, key))
            assignment[name] = state
            table[tuple(assignment[v] for v in scope)] = prob
    return Factor(scope=scope, table=table)


def restrict(factor: Factor, evidence: dict[str, str]) -> Factor:
    keep = [v for v in factor.scope if v not in evidence]
    idx = [factor.scope.index(v) for v in keep]
    table: dict[tuple[str, ...], float] = {}
    for key, prob in factor.table.items():
        if all(factor.table and key[factor.scope.index(v)] == s
               for v, s in evidence.items() if v in factor.scope):
            new_key = tuple(key[i] for i in idx)
            table[new_key] = table.get(new_key, 0.0) + prob
    return Factor(scope=tuple(keep), table=table)


def multiply(a: Factor, b: Factor) -> Factor:
    scope = tuple(sorted(set(a.scope) | set(b.scope)))
    table: dict[tuple[str, ...], float] = {}
    for ka, pa in a.table.items():
        for kb, pb in b.table.items():
            if all(ka[a.scope.index(v)] == kb[b.scope.index(v)]
                   for v in set(a.scope) & set(b.scope)):
                key = [None] * len(scope)
                for v in a.scope:
                    key[scope.index(v)] = ka[a.scope.index(v)]
                for v in b.scope:
                    key[scope.index(v)] = kb[b.scope.index(v)]
                tkey = tuple(key)
                table[tkey] = table.get(tkey, 0.0) + pa * pb
    return Factor(scope=scope, table=table)


def sum_out(factor: Factor, var: str) -> Factor:
    keep = tuple(v for v in factor.scope if v != var)
    idx = factor.scope.index(var)
    keep_idx = [i for i in range(len(factor.scope)) if i != idx]
    table: dict[tuple[str, ...], float] = {}
    for key, prob in factor.table.items():
        new_key = tuple(key[i] for i in keep_idx)
        table[new_key] = table.get(new_key, 0.0) + prob
    return Factor(scope=keep, table=table)


def auto_order(net: Network, factors: list[Factor], hidden: list[str]) -> list[str]:
    """Greedy: eliminate the variable whose product factor has fewest entries.

    Entry count is the product of the state counts of the variables in the
    merged scope. Ties are broken by variable name (lexicographic).
    """
    order: list[str] = []
    factors = list(factors)
    remaining = list(hidden)
    while remaining:
        best_var, best_key = None, None
        for var in remaining:
            scope = {v for f in factors if var in f.scope for v in f.scope}
            entries = 1
            for v in scope:
                entries *= len(net.variables[v].states)
            key = (entries, var)
            if best_key is None or key < best_key:
                best_var, best_key = var, key
        order.append(best_var)
        remaining.remove(best_var)
        involved = [f for f in factors if best_var in f.scope]
        factors = [f for f in factors if best_var not in f.scope]
        merged = tuple(sorted({v for f in involved for v in f.scope} - {best_var}))
        factors.append(Factor(scope=merged, table={}))
    return order


@dataclass
class QueryResult:
    target: str
    evidence: dict[str, str]
    evidence_probability: float
    distribution: dict[str, float]
    map_assignment: dict[str, str]
    elimination_order: list[str]
    max_factor_size: int


def run_query(net: Network, targets: list[str], evidence: dict[str, str],
              elimination_order: list[str] | None = None) -> QueryResult:
    targets = sorted(targets)
    if not targets:
        raise QueryError("query must name at least one target variable")
    for name in targets + list(evidence):
        if name not in net.variables:
            raise QueryError(f"unknown variable '{name}'")
    overlap = set(targets) & set(evidence)
    if overlap:
        raise QueryError(f"targets and evidence overlap: {sorted(overlap)}")
    for name, state in evidence.items():
        if state not in net.variables[name].states:
            raise QueryError(f"unknown state '{state}' for variable '{name}'")

    factors = [factor_from_cpt(net, name) for name in net.order]
    factors = [restrict(f, evidence) for f in factors]
    constant = 1.0
    kept = []
    for f in factors:
        if f.scope:
            kept.append(f)
        else:
            constant *= sum(f.table.values())
    factors = kept

    hidden = [n for n in net.order if n not in targets and n not in evidence]
    if elimination_order is None:
        order = auto_order(net, factors, hidden)
    else:
        if sorted(elimination_order) != sorted(hidden):
            raise QueryError(
                "elimination order must cover exactly the hidden variables "
                f"{sorted(hidden)}")
        order = list(elimination_order)

    max_factor_size = max((f.size for f in factors), default=0)
    for var in order:
        involved = [f for f in factors if var in f.scope]
        factors = [f for f in factors if var not in f.scope]
        product = involved[0]
        for f in involved[1:]:
            product = multiply(product, f)
        max_factor_size = max(max_factor_size, product.size)
        factors.append(sum_out(product, var))

    joint = Factor(scope=(), table={(): constant})
    for f in factors:
        joint = multiply(joint, f)
    max_factor_size = max(max_factor_size, joint.size)

    evidence_prob = sum(joint.table.values())
    if evidence_prob <= 0.0:
        raise QueryError("evidence has zero probability")

    distribution: dict[tuple[str, ...], float] = {}
    for key, prob in joint.table.items():
        assignment = dict(zip(joint.scope, key))
        tkey = tuple(assignment[t] for t in targets)
        distribution[tkey] = distribution.get(tkey, 0.0) + prob / evidence_prob

    best_key = max(sorted(distribution), key=lambda k: distribution[k])
    return QueryResult(
        target=",".join(targets),
        evidence=dict(evidence),
        evidence_probability=evidence_prob,
        distribution={",".join(k): distribution[k] for k in sorted(distribution)},
        map_assignment=dict(zip(targets, best_key)),
        elimination_order=order,
        max_factor_size=max_factor_size,
    )
