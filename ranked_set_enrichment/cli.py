"""Command-line interface for offline ranked set enrichment.

Reads a feature-score CSV and a feature-set JSON, writes a traceable
enrichment curve CSV per set plus a statistical summary CSV/JSON.
No network access is performed.
"""

from __future__ import annotations

import argparse
import csv
import json
import math
import sys
from pathlib import Path

from .enrichment import (
    RankedFeature,
    bh_adjust,
    compute_enrichment,
    sort_ranked_list,
)


def load_ranked_csv(path: Path) -> list[RankedFeature]:
    """Load and validate the feature-score CSV (id,score columns)."""
    features: list[RankedFeature] = []
    seen: set[str] = set()
    with path.open(newline="", encoding="utf-8-sig") as handle:
        reader = csv.DictReader(handle)
        if reader.fieldnames is None or not {"id", "score"} <= set(
            reader.fieldnames
        ):
            raise ValueError("CSV must contain 'id' and 'score' columns")
        for line, row in enumerate(reader, start=2):
            identifier = (row["id"] or "").strip()
            if not identifier:
                raise ValueError(f"line {line}: empty feature id")
            if identifier in seen:
                raise ValueError(f"line {line}: duplicate id {identifier!r}")
            seen.add(identifier)
            try:
                score = float(row["score"])
            except (TypeError, ValueError):
                raise ValueError(
                    f"line {line}: non-numeric score {row['score']!r}"
                ) from None
            if not math.isfinite(score):
                raise ValueError(
                    f"line {line}: non-finite score for {identifier!r}"
                )
            features.append(RankedFeature(identifier, score))
    if len(features) < 2:
        raise ValueError("ranked list needs at least two features")
    return sort_ranked_list(features)


def load_sets_json(path: Path) -> list[tuple[str, list[str]]]:
    """Load feature sets from JSON: {name: [members...]} or list of objects."""
    data = json.loads(path.read_text(encoding="utf-8-sig"))
    sets: list[tuple[str, list[str]]] = []
    if isinstance(data, dict):
        items = list(data.items())
    elif isinstance(data, list):
        items = []
        for entry in data:
            if not isinstance(entry, dict) or not {
                "name",
                "members",
            } <= set(entry):
                raise ValueError("list entries need 'name' and 'members'")
            items.append((entry["name"], entry["members"]))
    else:
        raise ValueError("sets JSON must be an object or a list")
    for name, members in items:
        if not isinstance(members, list) or not all(
            isinstance(m, str) for m in members
        ):
            raise ValueError(
                f"set {name!r}: members must be a list of strings"
            )
        sets.append((str(name), list(members)))
    return sets


def write_curve(path: Path, ranked: list[RankedFeature], walk) -> None:
    """Write the per-position running-enrichment curve for one set."""
    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(["position", "id", "score", "running_enrichment"])
        for pos, (feature, value) in enumerate(zip(ranked, walk)):
            writer.writerow([pos, feature.identifier, feature.score, value])


def run(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="ranked-set-enrichment",
        description="Offline ranked feature-set enrichment (stdlib only).",
    )
    parser.add_argument("--scores", required=True, type=Path,
                        help="CSV with columns id,score")
    parser.add_argument("--sets", required=True, type=Path,
                        help="JSON mapping set names to member lists")
    parser.add_argument("--outdir", required=True, type=Path,
                        help="output directory for summary and curves")
    parser.add_argument("--permutations", type=int, default=1000,
                        help="number of position permutations (default 1000)")
    parser.add_argument("--seed", type=int, default=1,
                        help="random seed for permutations (default 1)")
    args = parser.parse_args(argv)

    try:
        ranked = load_ranked_csv(args.scores)
        sets = load_sets_json(args.sets)
    except (OSError, ValueError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    args.outdir.mkdir(parents=True, exist_ok=True)
    curves_dir = args.outdir / "curves"
    curves_dir.mkdir(exist_ok=True)

    results = [
        compute_enrichment(name, members, ranked, args.permutations, args.seed)
        for name, members in sets
    ]
    valid = [r for r in results if r.computable]
    adjusted = bh_adjust([r.p_value for r in valid])
    adjusted_by_name = {r.name: q for r, q in zip(valid, adjusted)}

    summary_rows = []
    for result in results:
        row = {
            "set": result.name,
            "computable": result.computable,
            "reason": result.reason or "",
            "effective_size": result.effective_size,
            "enrichment_score": result.enrichment_score,
            "extremum_position": result.extremum_position,
            "p_value": result.p_value,
            "q_value_bh": adjusted_by_name.get(result.name),
            "exceedances": result.exceedances,
            "missing_members": ";".join(result.missing_members),
            "duplicate_members": ";".join(result.duplicate_members),
        }
        summary_rows.append(row)
        if result.computable:
            write_curve(curves_dir / f"{result.name}.csv", ranked, result.walk)

    summary_csv = args.outdir / "summary.csv"
    if not summary_rows:
        print("error: no feature sets provided", file=sys.stderr)
        return 2
    with summary_csv.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=list(summary_rows[0]))
        writer.writeheader()
        writer.writerows(summary_rows)

    summary_json = args.outdir / "summary.json"
    summary_json.write_text(
        json.dumps(
            {
                "parameters": {
                    "permutations": args.permutations,
                    "seed": args.seed,
                    "ranked_size": len(ranked),
                },
                "results": summary_rows,
            },
            indent=2,
        ),
        encoding="utf-8",
    )
    print(f"wrote {summary_csv} and {summary_json}; curves in {curves_dir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(run())
