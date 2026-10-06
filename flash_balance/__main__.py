"""Command line interface: python -m flash_balance config.json --outdir OUT."""

from __future__ import annotations

import argparse
import csv
import json
import sys
from pathlib import Path

from .cascade import TwoStageResult, run_two_stage
from .flash import FlashError

STREAMS = ("stage1_liquid", "stage2_liquid", "stage2_vapor")


def _load_config(path: Path) -> dict:
    try:
        config = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise FlashError(f"cannot read config {path}: {exc}") from exc
    if not isinstance(config, dict):
        raise FlashError("config must be a JSON object")
    feeds = config.get("feeds")
    if not isinstance(feeds, list) or not feeds or not all(isinstance(f, dict) for f in feeds):
        raise FlashError("config 'feeds' must be a non-empty list of objects")
    cases = config.get("cases")
    if not isinstance(cases, list) or not cases:
        raise FlashError("config 'cases' must be a non-empty list")
    for case in cases:
        if not isinstance(case, dict) or not isinstance(case.get("stage1_k"), dict) \
                or not isinstance(case.get("stage2_k"), dict):
            raise FlashError("each case needs 'stage1_k' and 'stage2_k' objects")
    return config


def _stream_rows(result: TwoStageResult) -> list[dict]:
    rows = []
    for stream in STREAMS:
        flows = result.products[stream]
        total = sum(flows.values())
        row = {"stream": stream, "total_flow": total}
        for comp in result.components:
            row[f"n_{comp}"] = flows[comp]
            row[f"x_{comp}"] = (flows[comp] / total) if total > 0.0 else ""
        rows.append(row)
    return rows


def _write_csv(result: TwoStageResult, path: Path) -> None:
    rows = _stream_rows(result)
    fieldnames = ["stream", "total_flow"]
    for comp in result.components:
        fieldnames += [f"n_{comp}", f"x_{comp}"]
    with path.open("w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(fh, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(rows)


def _summary(result: TwoStageResult, case: dict) -> dict:
    def stage_info(stage):
        if stage.flash is None:
            return {"skipped": True, "reason": "no vapor feed from upstream"}
        f = stage.flash
        return {
            "skipped": False,
            "phase": f.phase,
            "vapor_fraction": f.vapor_fraction,
            "degenerate": f.degenerate,
            "rr_residual": f.residual,
            "iterations": f.iterations,
        }

    return {
        "case": case.get("name", "case"),
        "status": "ok",
        "stage1": stage_info(result.stage1),
        "stage2": stage_info(result.stage2),
        "products": {s: {"total_flow": sum(result.products[s].values())} for s in STREAMS},
        "balance": result.balance,
        "max_balance_residual": result.max_balance_residual,
        "equilibrium_residual": result.equilibrium_residual,
        "warnings": result.warnings,
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="flash_balance",
                                     description="Two-stage flash material balance")
    parser.add_argument("config", type=Path, help="JSON config with feeds and cases")
    parser.add_argument("--outdir", type=Path, default=Path("."),
                        help="directory for CSV and summary outputs")
    args = parser.parse_args(argv)

    try:
        config = _load_config(args.config)
        feeds = config["feeds"]
        summaries = []
        for index, case in enumerate(config["cases"]):
            name = str(case.get("name", f"case{index + 1}"))
            result = run_two_stage(feeds, case["stage1_k"], case["stage2_k"])
            summaries.append((name, _summary(result, case), result))
    except FlashError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    # Sort by case name so output ordering does not depend on input order.
    summaries.sort(key=lambda item: item[0])
    args.outdir.mkdir(parents=True, exist_ok=True)
    summary_list = []
    for name, summary, result in summaries:
        safe = "".join(c if c.isalnum() or c in "-_" else "_" for c in name)
        _write_csv(result, args.outdir / f"streams_{safe}.csv")
        summary_list.append(summary)
    (args.outdir / "summary.json").write_text(
        json.dumps({"cases": summary_list}, indent=2), encoding="utf-8")
    print(f"wrote {len(summary_list)} case(s) to {args.outdir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
