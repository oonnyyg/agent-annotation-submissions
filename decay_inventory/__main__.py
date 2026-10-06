"""Command line interface: python -m decay_inventory <input.json>.
 
The input JSON document contains the decay network, optional external
feeds, the output time schedule and the accumulation windows. On any
validation or convergence failure the tool exits with a non-zero status
and writes no report files.
"""
 
from __future__ import annotations
 
import argparse
import json
import math
import sys

from .model import ModelError, load_network
from .report import write_csv, write_json
from .solver import SolverError, solve
 
 
def _parse_schedule(data: dict) -> list[float]:
    schedule = data.get("schedule")
    if not isinstance(schedule, dict):
        raise ModelError("'schedule' must be an object")
    if "times" in schedule:
        times = schedule["times"]
        if not isinstance(times, list) or not times:
            raise ModelError("'schedule.times' must be a non-empty list")
        values = [float(t) for t in times]
    else:
        try:
            start = float(schedule["start"])
            stop = float(schedule["stop"])
            step = float(schedule["step"])
        except (KeyError, TypeError, ValueError) as exc:
            raise ModelError(
                "schedule requires 'times' or 'start'/'stop'/'step'") from exc
        if step <= 0 or stop < start:
            raise ModelError("schedule requires step > 0 and stop >= start")
        values = []
        t = start
        while t <= stop + 1e-12 * max(1.0, abs(stop)):
            values.append(min(t, stop) if t > stop else t)
            t += step
    if any(not math.isfinite(t) for t in values):
        raise ModelError("schedule times must be finite")
    if any(t2 < t1 for t1, t2 in zip(values, values[1:])):
        raise ModelError("schedule times must be sorted ascending")
    return values
 
 
def _parse_windows(data: dict) -> list[tuple[float, float]]:
    raw = data.get("windows", [])
    if not isinstance(raw, list):
        raise ModelError("'windows' must be a list")
    windows = []
    for entry in raw:
        if not isinstance(entry, dict):
            raise ModelError("each window must be an object")
        try:
            start = float(entry["start"])
            end = float(entry["end"])
        except (KeyError, TypeError, ValueError) as exc:
            raise ModelError("each window requires numeric 'start'/'end'") from exc
        if not (math.isfinite(start) and math.isfinite(end)) or end < start:
            raise ModelError(f"invalid window [{start}, {end}]")
        windows.append((start, end))
    return windows
 
 
def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m decay_inventory",
        description="Branched decay inventory calculator")
    parser.add_argument("input", help="JSON network and schedule file")
    parser.add_argument("--csv", default="decay_inventory.csv",
                        help="output CSV path (default: decay_inventory.csv)")
    parser.add_argument("--json", default="decay_inventory_windows.json",
                        help="output JSON path "
                             "(default: decay_inventory_windows.json)")
    args = parser.parse_args(argv)
 
    try:
        with open(args.input, "r", encoding="utf-8") as handle:
            data = json.load(handle)
        net = load_network(data)
        times = _parse_schedule(data)
        windows = _parse_windows(data)
        sol = solve(net, times, windows)
    except (OSError, json.JSONDecodeError, ModelError, SolverError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
 
    with open(args.csv, "w", encoding="utf-8", newline="") as handle:
        write_csv(net, sol, handle)
    with open(args.json, "w", encoding="utf-8", newline="") as handle:
        write_json(net, sol, handle)
    print(f"wrote {args.csv} and {args.json}")
    return 0
 
 
if __name__ == "__main__":
    sys.exit(main())
