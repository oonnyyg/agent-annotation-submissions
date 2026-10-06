"""Command-line interface: batch-solve scenarios and emit CSV/JSON reports."""

from __future__ import annotations

import argparse
import csv
import json
import sys
from pathlib import Path

from .model import ModelError, Network, Scenario, load_network, load_scenarios
from .solver import SolveResult, SolverOptions, solve


def boundary_supplies(network: Network, result: SolveResult) -> dict[str, float]:
    """Net flow injected into the network by each fixed-head boundary node."""
    supplies = {node: 0.0 for node in network.fixed_heads}
    for pipe in network.pipes:
        q = result.flows[pipe.name]
        if pipe.start in supplies:
            supplies[pipe.start] += q
        if pipe.end in supplies:
            supplies[pipe.end] -= q
    return supplies


def run_scenarios(
    network: Network,
    scenarios: list[Scenario],
    options: SolverOptions,
) -> list[dict]:
    """Solve each scenario independently; order does not affect results."""
    reports = []
    for scenario in scenarios:
        result = solve(network, scenario, options)
        entry: dict = {
            "scenario": scenario.name,
            "converged": result.converged,
            "iterations": result.iterations,
            "node_residual": result.node_residual,
            "pipe_residual": result.pipe_residual,
            "message": result.message,
        }
        if result.converged:
            supplies = boundary_supplies(network, result)
            total_demand = sum(
                scenario.demands.get(node, 0.0) for node in network.demand_nodes
            )
            entry["heads"] = result.heads
            entry["flows"] = result.flows
            entry["boundary_supply"] = supplies
            entry["total_demand"] = total_demand
            entry["total_supply"] = sum(supplies.values())
            entry["balance_error"] = sum(supplies.values()) - total_demand
        reports.append(entry)
    return reports


def write_pipe_csv(reports: list[dict], pipe_names: list[str], path: Path) -> None:
    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(["scenario", "pipe", "flow", "converged"])
        for entry in reports:
            if not entry["converged"]:
                writer.writerow([entry["scenario"], "", "", "false"])
                continue
            for name in pipe_names:
                writer.writerow(
                    [entry["scenario"], name, f"{entry['flows'][name]:.10g}", "true"]
                )


def write_node_json(reports: list[dict], path: Path) -> None:
    nodes_report = []
    for entry in reports:
        item = {
            "scenario": entry["scenario"],
            "converged": entry["converged"],
            "iterations": entry["iterations"],
            "node_residual": entry["node_residual"],
            "pipe_residual": entry["pipe_residual"],
            "message": entry["message"],
        }
        if entry["converged"]:
            item["heads"] = entry["heads"]
            item["boundary_supply"] = entry["boundary_supply"]
            item["total_demand"] = entry["total_demand"]
            item["total_supply"] = entry["total_supply"]
            item["balance_error"] = entry["balance_error"]
        nodes_report.append(item)
    path.write_text(
        json.dumps({"scenarios": nodes_report}, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="pipenet",
        description="Steady-state solver for quadratic pipe networks.",
    )
    parser.add_argument("network", type=Path, help="network JSON file")
    parser.add_argument("scenarios", type=Path, help="scenarios JSON file")
    parser.add_argument("--pipes-csv", type=Path, default=Path("pipes.csv"))
    parser.add_argument("--nodes-json", type=Path, default=Path("nodes.json"))
    parser.add_argument("--max-iterations", type=int, default=50)
    parser.add_argument("--node-tolerance", type=float, default=1e-9)
    parser.add_argument("--pipe-tolerance", type=float, default=1e-9)
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        network = load_network(args.network)
        scenarios = load_scenarios(args.scenarios)
    except (ModelError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    options = SolverOptions(
        max_iterations=args.max_iterations,
        node_tolerance=args.node_tolerance,
        pipe_tolerance=args.pipe_tolerance,
    )
    reports = run_scenarios(network, scenarios, options)
    write_pipe_csv(reports, [pipe.name for pipe in network.pipes], args.pipes_csv)
    write_node_json(reports, args.nodes_json)

    failed = [entry["scenario"] for entry in reports if not entry["converged"]]
    for entry in reports:
        status = "ok" if entry["converged"] else f"FAILED ({entry['message']})"
        print(f"{entry['scenario']}: {status}")
    print(f"wrote {args.pipes_csv} and {args.nodes_json}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
