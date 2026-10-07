"""Command line interface: python -m bayes network.json queries.jsonl"""

from __future__ import annotations

import argparse
import json
import sys

from .inference import QueryError, run_query
from .model import ModelError, load_network


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="bayes", description="Query a discrete Bayesian network.")
    parser.add_argument("network", help="path to the network JSON file")
    parser.add_argument("queries", nargs="?", default="-",
                        help="path to a JSONL query file (default: stdin)")
    args = parser.parse_args(argv)

    try:
        net = load_network(args.network)
    except (OSError, json.JSONDecodeError, ModelError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    stream = sys.stdin if args.queries == "-" else open(args.queries, encoding="utf-8")
    try:
        for lineno, line in enumerate(stream, 1):
            line = line.strip()
            if not line:
                continue
            print(json.dumps(_handle_line(net, line, lineno), ensure_ascii=False))
    finally:
        if stream is not sys.stdin:
            stream.close()
    return 0


def _handle_line(net, line: str, lineno: int) -> dict:
    try:
        query = json.loads(line)
        targets = query.get("targets") or [query["target"]]
        evidence = query.get("evidence", {})
        order = query.get("elimination_order")
        result = run_query(net, targets, evidence, order)
    except (json.JSONDecodeError, KeyError, TypeError, QueryError) as exc:
        return {"line": lineno, "error": str(exc)}
    return {
        "line": lineno,
        "target": result.target,
        "evidence": result.evidence,
        "evidence_probability": result.evidence_probability,
        "distribution": result.distribution,
        "map_assignment": result.map_assignment,
        "summary": {
            "elimination_order": result.elimination_order,
            "max_factor_size": result.max_factor_size,
        },
    }


if __name__ == "__main__":
    raise SystemExit(main())
