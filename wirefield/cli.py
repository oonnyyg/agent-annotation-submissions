"""Command-line interface: JSON config + CSV probes -> JSON/CSV reports."""

from __future__ import annotations

import argparse
import csv
import json
import os
import sys

from .coil import CoilDataError, load_config
from .probe import load_probes
from .report import build_report


def _fmt(value: float) -> str:
    return f"{value:.9e}"


def _ensure_parent(path: str) -> None:
    parent = os.path.dirname(os.path.abspath(path))
    os.makedirs(parent, exist_ok=True)


def _csv_rows(report: dict) -> list[dict]:
    rows: list[dict] = []
    for point in report["points"]:
        base = {
            "x": point["point"]["x"],
            "y": point["point"]["y"],
            "z": point["point"]["z"],
        }
        if not point["computable"]:
            rows.append(
                {
                    **base,
                    "scenario": "(baseline)",
                    "coil": "(total)",
                    "computable": "no",
                    "blocked_by": ";".join(point["blocked_by"]),
                }
            )
            continue
        blocks = [("baseline", "(baseline)", point["baseline"])]
        blocks += [(f"scenario:{s['name']}", s["name"], s) for s in point["scenarios"]]
        coil_ids = list(point["baseline"]["per_coil"])
        for kind, name, block in blocks:
            for cid in coil_ids + ["(total)"]:
                if cid == "(total)":
                    world, local = block["total_world"], block["total_local"]
                else:
                    world, local = block["per_coil"][cid], None
                row = {
                    **base,
                    "scenario": name,
                    "coil": cid,
                    "computable": "yes",
                    "blocked_by": "",
                    "bx": _fmt(world["x"]),
                    "by": _fmt(world["y"]),
                    "bz": _fmt(world["z"]),
                    "b_magnitude": _fmt(world["magnitude"]),
                }
                if local is not None:
                    row.update(
                        {
                            "b_local_u": _fmt(local["x"]),
                            "b_local_v": _fmt(local["y"]),
                            "b_local_w": _fmt(local["z"]),
                        }
                    )
                if kind.startswith("scenario:") and cid == "(total)":
                    dw = block["diff_world_vs_baseline"]
                    dl = block["diff_local_vs_baseline"]
                    row.update(
                        {
                            "diff_bx": _fmt(dw["x"]),
                            "diff_by": _fmt(dw["y"]),
                            "diff_bz": _fmt(dw["z"]),
                            "diff_b_magnitude": _fmt(dw["magnitude"]),
                            "diff_local_u": _fmt(dl["x"]),
                            "diff_local_v": _fmt(dl["y"]),
                            "diff_local_w": _fmt(dl["z"]),
                        }
                    )
                rows.append(row)
    return rows


CSV_FIELDS = [
    "x",
    "y",
    "z",
    "scenario",
    "coil",
    "computable",
    "blocked_by",
    "bx",
    "by",
    "bz",
    "b_magnitude",
    "b_local_u",
    "b_local_v",
    "b_local_w",
    "diff_bx",
    "diff_by",
    "diff_bz",
    "diff_b_magnitude",
    "diff_local_u",
    "diff_local_v",
    "diff_local_w",
]


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="wirefield",
        description=(
            "Superpose magnetic fields of closed polyline current loops "
            "(Biot-Savart, ideal thin wires)."
        ),
    )
    parser.add_argument("config", help="JSON file with coils and optional scenarios")
    parser.add_argument("probes", help="CSV file of probe points and local axes")
    parser.add_argument(
        "--safe-distance",
        type=float,
        required=True,
        help="positive safety distance in metres; points this close to any "
        "wire segment are reported as not computable",
    )
    parser.add_argument("--json-out", help="write the full JSON report here")
    parser.add_argument("--csv-out", help="write a flat CSV report here")
    args = parser.parse_args(argv)

    if args.safe_distance <= 0:
        print("error: --safe-distance must be positive", file=sys.stderr)
        return 2

    try:
        with open(args.config, encoding="utf-8") as handle:
            coils, scenarios = load_config(json.load(handle))
        probes = load_probes(args.probes)
    except (OSError, json.JSONDecodeError, CoilDataError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    report = build_report(coils, probes, scenarios, args.safe_distance)

    if args.json_out:
        _ensure_parent(args.json_out)
        with open(args.json_out, "w", encoding="utf-8") as handle:
            json.dump(report, handle, indent=2)
            handle.write("\n")
    if args.csv_out:
        _ensure_parent(args.csv_out)
        with open(args.csv_out, "w", newline="", encoding="utf-8") as handle:
            writer = csv.DictWriter(handle, fieldnames=CSV_FIELDS)
            writer.writeheader()
            writer.writerows(_csv_rows(report))
    if not args.json_out and not args.csv_out:
        json.dump(report, sys.stdout, indent=2)
        sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
