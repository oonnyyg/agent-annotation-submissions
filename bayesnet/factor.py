"""Factor representation and operations for variable elimination."""

from __future__ import annotations

from dataclasses import dataclass


@dataclass
class Factor:
    vars: tuple[str, ...]
    table: dict[tuple[str, ...], float]

    @property
    def size(self) -> int:
        return len(self.table)


def multiply(left: Factor, right: Factor) -> Factor:
    out_vars = tuple(dict.fromkeys(left.vars + right.vars))
    li = [out_vars.index(v) for v in left.vars]
    ri = [out_vars.index(v) for v in right.vars]
    shared = [v for v in left.vars if v in right.vars]
    l_shared = [left.vars.index(v) for v in shared]
    r_shared = [right.vars.index(v) for v in shared]
    table: dict[tuple[str, ...], float] = {}
    for key, value in left.table.items():
        for rkey, rvalue in right.table.items():
            if any(key[i] != rkey[j] for i, j in zip(l_shared, r_shared)):
                continue
            full = [None] * len(out_vars)
            for pos, idx in enumerate(li):
                full[idx] = key[pos]
            for pos, idx in enumerate(ri):
                full[idx] = rkey[pos]
            table[tuple(full)] = value * rvalue
    return Factor(vars=out_vars, table=table)


def sum_out(factor: Factor, var: str) -> Factor:
    idx = factor.vars.index(var)
    out_vars = tuple(v for v in factor.vars if v != var)
    table: dict[tuple[str, ...], float] = {}
    for key, value in factor.table.items():
        reduced = key[:idx] + key[idx + 1 :]
        table[reduced] = table.get(reduced, 0.0) + value
    return Factor(vars=out_vars, table=table)


def reduce_evidence(factor: Factor, evidence: dict[str, str]) -> Factor:
    relevant = {v: s for v, s in evidence.items() if v in factor.vars}
    if not relevant:
        return factor
    positions = {v: factor.vars.index(v) for v in relevant}
    keep_idx = [i for i, v in enumerate(factor.vars) if v not in relevant]
    out_vars = tuple(factor.vars[i] for i in keep_idx)
    table: dict[tuple[str, ...], float] = {}
    for key, value in factor.table.items():
        if all(key[pos] == s for v, s in relevant.items() for pos in [positions[v]]):
            table[tuple(key[i] for i in keep_idx)] = value
    return Factor(vars=out_vars, table=table)
