"""Command-line interface for offline ranked-set enrichment analysis."""

from __future__ import annotations

import argparse
import csv
import json
import sys
from pathlib import Path

from .core import load_feature_scores, load_feature_sets
from .pipeline import analyze


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="ranked-set-enrichment",
        description="Offline ranked-set enrichment analysis (standard library only).",
    )
    parser.add_argument("--scores", required=True, help="CSV with feature_id,score columns")
    parser.add_argument("--sets", required=True, help="JSON file with predefined feature sets")
    parser.add_argument("--permutations", type=int, default=1000, help="permutation count (default: 1000)")
    parser.add_argument("--seed", type=int, default=1, help="random seed (default: 1)")
    parser.add_argument("--summary", default="summary.csv", help="output summary CSV path")
    parser.add_argument("--curves-dir", default="curves", help="directory for per-set running-sum CSVs")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    if args.permutations < 1:
        print("error: --permutations must be >= 1", file=sys.stderr)
        return 2
    try:
        ranked = load_feature_scores(args.scores)
        feature_sets = load_feature_sets(args.sets)
    except (OSError, ValueError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    results = analyze(ranked, feature_sets, args.permutations, args.seed)

    curves_dir = Path(args.curves_dir)
    curves_dir.mkdir(parents=True, exist_ok=True)
    for result in results:
        if result.status != "ok":
            continue
        curve_path = curves_dir / f"{result.name}.csv"
        with open(curve_path, "w", newline="", encoding="utf-8") as handle:
            writer = csv.writer(handle)
            writer.writerow(["position", "feature_id", "score", "hit", "running_sum"])
            hits = set(result.hit_positions)
            for pos, (record, value) in enumerate(zip(ranked, result.running_sum)):
                writer.writerow([pos, record.feature_id, record.score, 1 if pos in hits else 0, value])

    with open(args.summary, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow([
            "set", "status", "reason", "enrichment_score", "peak_position",
            "member_count", "ignored_members", "p_value", "q_value",
        ])
        for result in results:
            writer.writerow([
                result.name,
                result.status,
                result.reason,
                f"{result.enrichment_score:.6g}" if result.status == "ok" else "",
                result.peak_position if result.status == "ok" else "",
                result.member_count,
                ";".join(result.ignored_members),
                f"{result.p_value:.6g}" if result.p_value is not None else "",
                f"{result.q_value:.6g}" if result.q_value is not None else "",
            ])
    print(json.dumps({"sets": len(results), "computable": sum(1 for r in results if r.status == "ok")}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
