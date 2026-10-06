"""CLI: python -m friction_motion INPUT.json [-o OUT_DIR]

Reads one or more scenarios from JSON, simulates each independently from
its own initial state, and writes per-scenario files:
  <name>_trajectory.csv  sampled trajectory (t, x, v, distance, work, ...)
  <name>_events.json     events and the work/energy balance
A failing scenario records its error in summary.json without affecting
the other scenarios.
"""

from __future__ import annotations

import argparse
import csv
import json
import re
import sys
from pathlib import Path

from .model import ScenarioError, parse_scenario
from .simulate import energy_summary, sample_trajectory, simulate


def _safe_name(name: str) -> str:
    return re.sub(r"[^A-Za-z0-9_.-]+", "_", name)


def _load_items(path: Path) -> list[dict]:
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    if isinstance(data, dict) and "scenarios" in data:
        items = data["scenarios"]
    elif isinstance(data, list):
        items = data
    else:
        items = [data]
    if not isinstance(items, list) or not items:
        raise ScenarioError("input JSON must contain at least one scenario")
    return items


def _run_one(item: dict, index: int, out_dir: Path) -> dict:
    scn = parse_scenario(item, index)
    result = simulate(scn)
    rows = sample_trajectory(result)
    energy = energy_summary(result)
    base = _safe_name(scn.name)

    csv_path = out_dir / f"{base}_trajectory.csv"
    with open(csv_path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(
            fh,
            fieldnames=["t", "x", "v", "moving", "distance",
                        "work_ext", "dissipation"],
        )
        writer.writeheader()
        writer.writerows(rows)

    events = [
        {"t": e.t, "kind": e.kind, "x": e.x, "v": e.v,
         "force": e.force, "detail": e.detail}
        for e in result.events
    ]
    json_path = out_dir / f"{base}_events.json"
    with open(json_path, "w", encoding="utf-8") as fh:
        json.dump(
            {"name": scn.name, "events": events, "energy": energy},
            fh, indent=2,
        )

    return {
        "name": scn.name,
        "status": "ok",
        "trajectory_csv": str(csv_path),
        "events_json": str(json_path),
        "energy": energy,
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="friction_motion")
    parser.add_argument("input", help="scenario JSON file")
    parser.add_argument("-o", "--out-dir", default="friction_out",
                        help="output directory (default: friction_out)")
    args = parser.parse_args(argv)

    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    try:
        items = _load_items(Path(args.input))
    except (OSError, json.JSONDecodeError, ScenarioError) as exc:
        print(f"error: cannot load scenarios: {exc}", file=sys.stderr)
        return 2

    summaries = []
    for index, item in enumerate(items):
        try:
            summaries.append(_run_one(item, index, out_dir))
        except (ScenarioError, ValueError, KeyError, TypeError) as exc:
            label = (
                item.get("name", f"scenario_{index}")
                if isinstance(item, dict)
                else f"scenario_{index}"
            )
            summaries.append({"name": label, "status": "error",
                              "error": str(exc)})

    summary_path = out_dir / "summary.json"
    with open(summary_path, "w", encoding="utf-8") as fh:
        json.dump({"scenarios": summaries}, fh, indent=2)

    for entry in summaries:
        if entry["status"] == "ok":
            energy = entry["energy"]
            print(f"[ok] {entry['name']}: residual="
                  f"{energy['residual']:.3e} J -> {entry['trajectory_csv']}")
        else:
            print(f"[error] {entry['name']}: {entry['error']}")
    print(f"summary -> {summary_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
