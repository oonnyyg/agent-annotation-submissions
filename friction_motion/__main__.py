"""Command line interface: python -m friction_motion input.json [-o outdir]

Input JSON is either a single scenario object or
{"scenarios": [<scenario>, ...]}. Each scenario is simulated independently
from its own initial state; a failing scenario reports its error without
affecting the others.
"""

from __future__ import annotations

import argparse
import csv
import json
import sys
from pathlib import Path

from .core import scenario_from_dict, simulate, ValidationError
from .sampling import sample_rows


def _run_one(data: dict, outdir: Path) -> dict:
    scenario = scenario_from_dict(data)
    result = simulate(scenario)
    safe = "".join(c if c.isalnum() or c in "-_" else "_" for c in scenario.name)

    csv_path = outdir / f"{safe}_trajectory.csv"
    with csv_path.open("w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["t", "x", "v"])
        for t, x, v in sample_rows(result["pieces"], scenario.t_end, scenario.dt):
            writer.writerow([f"{t:.12g}", f"{x:.12g}", f"{v:.12g}"])

    summary = {
        "name": scenario.name,
        "status": "ok",
        "trajectory_csv": csv_path.name,
        "events": result["events"],
        "final": result["final"],
        "distance": result["distance"],
        "net_displacement": result["net_displacement"],
        "energy": result["energy"],
    }
    json_path = outdir / f"{safe}_events_energy.json"
    json_path.write_text(json.dumps(summary, indent=2), encoding="utf-8")
    summary["events_energy_json"] = json_path.name
    return summary


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m friction_motion",
        description="Simulate a horizontal slider with dry friction under "
                    "piecewise-constant external force.")
    parser.add_argument("input", help="scenario JSON file")
    parser.add_argument("-o", "--outdir", default=".",
                        help="output directory (default: current directory)")
    args = parser.parse_args(argv)

    outdir = Path(args.outdir)
    outdir.mkdir(parents=True, exist_ok=True)

    try:
        payload = json.loads(Path(args.input).read_text(encoding="utf-8-sig"))
    except (OSError, json.JSONDecodeError) as exc:
        print(f"error: cannot read input: {exc}", file=sys.stderr)
        return 2

    if isinstance(payload, dict) and "scenarios" in payload:
        entries = payload["scenarios"]
        if not isinstance(entries, list) or not entries:
            print("error: 'scenarios' must be a non-empty list", file=sys.stderr)
            return 2
    else:
        entries = [payload]

    results = []
    for i, entry in enumerate(entries):
        name = entry.get("name", f"scenario_{i}") if isinstance(entry, dict) \
            else f"scenario_{i}"
        try:
            results.append(_run_one(entry, outdir))
        except (ValidationError, KeyError, TypeError) as exc:
            results.append({"name": name, "status": "error", "reason": str(exc)})

    index_path = outdir / "results.json"
    index_path.write_text(json.dumps(results, indent=2), encoding="utf-8")
    for res in results:
        if res["status"] == "ok":
            energy = res["energy"]
            print(f"[ok] {res['name']}: {len(res['events'])} events, "
                  f"residual={energy['residual']:.3e} J")
        else:
            print(f"[error] {res['name']}: {res['reason']}")
    print(f"summary written to {index_path}")
    return 0 if all(r["status"] == "ok" for r in results) else 1


if __name__ == "__main__":
    raise SystemExit(main())
