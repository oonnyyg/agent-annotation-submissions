"""Command-line interface: history CSV + model JSON -> cycles CSV + summary JSON."""
from __future__ import annotations

import argparse
import json
import sys
from pathlib import Path

from .damage import assess_cycles, total_damage
from .io import load_history, load_model, write_cycles_csv
from .rainflow import count_cycles
from .turning_points import extract_turning_points


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="rainflow-fatigue",
        description="Rainflow cycle counting and accumulated damage audit.",
    )
    parser.add_argument("history", help="CSV with time,stress columns (MPa)")
    parser.add_argument("config", help="JSON damage model configuration")
    parser.add_argument("--cycles-out", default="cycles.csv",
                        help="output CSV of counted cycles")
    parser.add_argument("--summary-out", default="summary.json",
                        help="output JSON summary")
    return parser


def run(history_path: str, config_path: str,
        cycles_out: str, summary_out: str) -> dict:
    points = load_history(history_path)
    model = load_model(config_path)
    turning = extract_turning_points(points)
    cycles = count_cycles(turning)
    results = assess_cycles(cycles, model)
    total = total_damage(results)

    write_cycles_csv(results, cycles_out)
    summary = {
        "samples": len(points),
        "turning_points": len(turning),
        "cycle_count": sum(c.count for c in cycles),
        "half_cycles": sum(1 for c in cycles if c.count == 0.5),
        "full_cycles": sum(1 for c in cycles if c.count == 1.0),
        "evaluable": all(r.evaluable for r in results),
        "total_damage": total,
        "cycles": [
            {
                "start_time": r.cycle.start_time,
                "end_time": r.cycle.end_time,
                "stress_range": r.cycle.stress_range,
                "amplitude": r.cycle.amplitude,
                "mean": r.cycle.mean,
                "count": r.cycle.count,
                "evaluable": r.evaluable,
                "equivalent_amplitude": r.equivalent_amplitude,
                "life": r.life,
                "damage": r.damage,
            }
            for r in results
        ],
    }
    Path(summary_out).write_text(
        json.dumps(summary, indent=2) + "\n", encoding="utf-8")
    return summary


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        summary = run(args.history, args.config, args.cycles_out, args.summary_out)
    except (ValueError, OSError, KeyError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    if summary["total_damage"] is None:
        print("total damage: NOT EVALUABLE (see per-cycle flags)")
    else:
        print(f"total damage: {summary['total_damage']:.6g}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
