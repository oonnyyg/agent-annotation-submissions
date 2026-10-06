"""Command line interface: batch steady-state scenario runs.

Usage:
    python -m pipenet NETWORK.json SCENARIOS.json --pipes-out pipes.csv --nodes-out nodes.json
"""

from __future__ import annotations

import argparse
import csv
import json
import os
import sys

from .model import ModelError, load_network, load_scenarios
from .solver import solve


def _boundary_supply(net, result) -> dict[str, float]:
    """Net outflow from each fixed-head node into the network."""
    supply = {nid: 0.0 for nid in net.fixed_ids}
    for p in net.pipes:
        q = result.flows[p.pipe_id]
        if p.from_node in supply:
            supply[p.from_node] += q
        if p.to_node in supply:
            supply[p.to_node] -= q
    return supply


def run(args: argparse.Namespace) -> int:
    try:
        net = load_network(args.network)
        scenarios = load_scenarios(args.scenarios, net)
    except (ModelError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    pipe_rows = []
    node_report = {"scenarios": []}
    failures = 0
    for sc in scenarios:
        res = solve(net, sc["demands"], args.max_iter, args.tol_node, args.tol_pipe)
        entry = {
            "name": sc["name"],
            "converged": res.converged,
            "reason": res.reason,
            "iterations": res.iterations,
            "node_residual": res.node_residual,
            "pipe_residual": res.pipe_residual,
        }
        if res.converged:
            supply = _boundary_supply(net, res)
            total_supply = sum(supply.values())
            total_demand = sum(sc["demands"].values())
            entry["heads"] = res.heads
            entry["demands"] = sc["demands"]
            entry["boundary_supply"] = supply
            entry["total_supply"] = total_supply
            entry["total_demand"] = total_demand
            entry["balance_error"] = total_supply - total_demand
            for p in net.pipes:
                q = res.flows[p.pipe_id]
                pipe_rows.append({
                    "scenario": sc["name"],
                    "pipe": p.pipe_id,
                    "from": p.from_node,
                    "to": p.to_node,
                    "flow": f"{q:.10g}",
                    "direction": "forward" if q > 0 else ("reverse" if q < 0 else "zero"),
                    "head_loss": f"{res.heads[p.from_node] - res.heads[p.to_node]:.10g}",
                })
        else:
            failures += 1
            print(
                f"scenario {sc['name']!r} failed: {res.reason} "
                f"(node residual {res.node_residual:.3g}, "
                f"pipe residual {res.pipe_residual:.3g})",
                file=sys.stderr,
            )
        node_report["scenarios"].append(entry)

    for out in (args.pipes_out, args.nodes_out):
        parent = os.path.dirname(os.path.abspath(out))
        os.makedirs(parent, exist_ok=True)
    with open(args.pipes_out, "w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(
            fh, fieldnames=["scenario", "pipe", "from", "to", "flow", "direction", "head_loss"]
        )
        writer.writeheader()
        writer.writerows(pipe_rows)
    with open(args.nodes_out, "w", encoding="utf-8") as fh:
        json.dump(node_report, fh, indent=2, ensure_ascii=False)
        fh.write("\n")
    return 1 if failures else 0


def main(argv: list[str] | None = None) -> int:
    ap = argparse.ArgumentParser(prog="pipenet", description=__doc__)
    ap.add_argument("network", help="network JSON file")
    ap.add_argument("scenarios", help="scenarios JSON file")
    ap.add_argument("--pipes-out", default="pipes.csv")
    ap.add_argument("--nodes-out", default="nodes.json")
    ap.add_argument("--max-iter", type=int, default=50)
    ap.add_argument("--tol-node", type=float, default=1e-8)
    ap.add_argument("--tol-pipe", type=float, default=1e-8)
    return run(ap.parse_args(argv))


if __name__ == "__main__":
    sys.exit(main())
