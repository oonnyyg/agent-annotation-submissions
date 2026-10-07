from __future__ import annotations

import argparse
import csv
import json
import sys
from pathlib import Path

from .errors import ObservationError, PlanError
from .exact import exact_stopping
from .observations import load_observations
from .plan import load_plan
from .replay import replay_sequence


def _write_steps_csv(path: Path, replays) -> None:
    with open(path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(["sequence_id", "index", "position", "value",
                         "increment", "cumulative_llr", "state"])
        for replay in replays:
            for step in replay.steps:
                writer.writerow([replay.sequence_id, step.index,
                                 step.position, step.value,
                                 f"{step.increment:.10f}",
                                 f"{step.cumulative:.10f}", step.state])


def _write_probabilities_csv(path: Path, exact) -> None:
    with open(path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(["step", "stop_alternative", "stop_null", "survival"])
        for step in exact.steps:
            writer.writerow([step.step, f"{step.stop_alternative:.12f}",
                             f"{step.stop_null:.12f}",
                             f"{step.survival:.12f}"])


def _summary(replays, exact, plan) -> dict:
    return {
        "plan": {
            "p0": plan.p0, "p1": plan.p1,
            "lower": plan.lower, "upper": plan.upper,
            "max_observations": plan.max_observations,
            "true_p": plan.true_p,
        },
        "sequences": [
            {
                "sequence_id": r.sequence_id,
                "decision": r.decision,
                "reason": r.reason,
                "stop_index": r.stop_index,
                "used": r.used,
                "unused": r.unused,
                "final_llr": r.final_llr,
            }
            for r in replays
        ],
        "exact": {
            "true_p": exact.true_p,
            "undecided_at_max": exact.undecided_at_max,
            "expected_sample": exact.expected_sample,
            "total_stop_probability": exact.total_stop_probability,
        },
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="seqaudit",
        description="Sequential probability ratio audit for Bernoulli data.")
    parser.add_argument("--plan", required=True, help="JSON plan file")
    parser.add_argument("--observations", required=True,
                        help="CSV with sequence_id, position, value")
    parser.add_argument("--out-dir", default="seqaudit_out",
                        help="directory for CSV/JSON outputs")
    args = parser.parse_args(argv)

    try:
        plan = load_plan(args.plan)
        grouped = load_observations(args.observations)
    except (PlanError, ObservationError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    replays = [replay_sequence(plan, seq, obs)
               for seq, obs in grouped.items()]
    exact = exact_stopping(plan)

    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    _write_steps_csv(out_dir / "steps.csv", replays)
    _write_probabilities_csv(out_dir / "stopping_probabilities.csv", exact)
    summary = _summary(replays, exact, plan)
    (out_dir / "summary.json").write_text(
        json.dumps(summary, indent=2) + "\n", encoding="utf-8")

    for r in replays:
        print(f"{r.sequence_id}: {r.decision} ({r.reason}) "
              f"used={r.used} unused={r.unused} llr={r.final_llr:.6f}")
    print(f"exact: P(undecided@max)={exact.undecided_at_max:.6f} "
          f"E[truncated N]={exact.expected_sample:.4f}")
    print(f"outputs written to {out_dir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
