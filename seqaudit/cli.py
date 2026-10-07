"""Command-line interface: replay sequences and run exact stopping analysis."""

from __future__ import annotations

import argparse
import csv
import json
import sys
from pathlib import Path

from .exact import exact_stopping
from .plan import InputError, load_observations, load_plan
from .replay import replay


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="seqaudit",
        description="Sequential probability ratio audit for Bernoulli observations.",
    )
    parser.add_argument("--plan", required=True, help="JSON plan file")
    parser.add_argument("--observations", required=True, help="observations CSV file")
    parser.add_argument("--details-csv", required=True, help="output CSV with per-step detail")
    parser.add_argument("--summary-json", required=True, help="output JSON summary report")
    return parser


def run(plan_path: str, obs_path: str, details_csv: str, summary_json: str) -> dict:
    plan = load_plan(plan_path)
    series = load_observations(obs_path)

    replays = {sid: replay(plan, obs) for sid, obs in series.items()}
    exact = exact_stopping(plan)

    with open(details_csv, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(["series_id", "step", "position", "outcome",
                         "llr_increment", "cumulative_llr", "status"])
        for sid in sorted(replays):
            for step in replays[sid].steps:
                writer.writerow([sid, step.index, step.position, step.outcome,
                                 f"{step.llr_increment:.12g}", f"{step.cumulative_llr:.12g}",
                                 step.status])

    summary = {
        "plan": {
            "plan_id": plan.plan_id, "p0": plan.p0, "p1": plan.p1,
            "lower": plan.lower, "upper": plan.upper,
            "max_observations": plan.max_observations, "true_p": plan.true_p,
        },
        "series": {
            sid: {
                "decision": r.decision,
                "stop_reason": r.stop_reason,
                "used": r.used,
                "unused": r.unused,
                "stop_position": r.stop_position,
                "final_llr": r.final_llr,
            }
            for sid, r in sorted(replays.items())
        },
        "exact_analysis": {
            "true_p": exact.true_p,
            "prob_stop_null": exact.prob_stop_null,
            "prob_stop_alternative": exact.prob_stop_alternative,
            "prob_undecided_at_cap": exact.prob_undecided,
            "expected_sample_size": exact.expected_sample_size,
            "total_probability": exact.total_probability,
            "per_step": [
                {"step": s.step, "prob_stop_null": s.prob_stop_null,
                 "prob_stop_alternative": s.prob_stop_alternative,
                 "prob_running": s.prob_running}
                for s in exact.steps
            ],
        },
    }
    Path(summary_json).write_text(
        json.dumps(summary, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    return summary


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        summary = run(args.plan, args.observations, args.details_csv, args.summary_json)
    except InputError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    n = len(summary["series"])
    exact = summary["exact_analysis"]
    print(f"plan '{summary['plan']['plan_id']}': replayed {n} series; "
          f"P(stop null)={exact['prob_stop_null']:.6f} "
          f"P(stop alt)={exact['prob_stop_alternative']:.6f} "
          f"P(undecided)={exact['prob_undecided_at_cap']:.6f} "
          f"E[N]={exact['expected_sample_size']:.4f}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
