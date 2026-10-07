"""Command line interface: python -m bayesnet NETWORK.json QUERIES.jsonl"""

from __future__ import annotations

import argparse
import json
import sys

from .inference import QueryError, query
from .model import ModelError, load_network


def _run_query(net, spec: dict) -> dict:
    target = spec.get("target")
    if isinstance(target, str):
        target = [target]
    if not isinstance(target, list):
        raise QueryError("'target' must be a variable name or a list of names")
    evidence = spec.get("evidence", {})
    if not isinstance(evidence, dict):
        raise QueryError("'evidence' must be an object mapping variables to states")
    order = spec.get("order")
    result = query(net, target, evidence, order)
    return {
        "target": result.target,
        "evidence": result.evidence,
        "evidence_probability": result.evidence_probability,
        "distribution": [
            {"assignment": dict(zip(sorted(result.target), key)), "probability": prob}
            for key, prob in result.distribution.items()
        ],
        "map_assignment": result.map_assignment,
        "summary": {
            "elimination_order": result.elimination_order,
            "max_factor_size": result.max_factor_size,
            "hidden_variables": result.hidden_variables,
        },
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="bayesnet", description=__doc__)
    parser.add_argument("network", help="path to the network JSON definition")
    parser.add_argument("queries", help="path to a JSONL file of queries")
    args = parser.parse_args(argv)

    try:
        net = load_network(args.network)
    except (OSError, json.JSONDecodeError, ModelError) as exc:
        print(f"error: invalid network: {exc}", file=sys.stderr)
        return 2

    status = 0
    with open(args.queries, "r", encoding="utf-8") as fh:
        for lineno, line in enumerate(fh, 1):
            line = line.strip()
            if not line:
                continue
            try:
                spec = json.loads(line)
                output = _run_query(net, spec)
                output["query"] = lineno
            except (json.JSONDecodeError, QueryError) as exc:
                status = 1
                output = {"query": lineno, "error": str(exc)}
            print(json.dumps(output, ensure_ascii=False))
    return status


if __name__ == "__main__":
    raise SystemExit(main())
