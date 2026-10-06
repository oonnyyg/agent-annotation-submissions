"""JSON input parsing and CSV/JSON report rendering."""

from __future__ import annotations

import csv
import io as _io
import json

from .cascade import CascadeResult
from .errors import FlashBalanceError


def _as_number(value, what: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise FlashBalanceError(f"{what} must be a number")
    return float(value)


def parse_config(text: str) -> tuple[list[dict[str, float]], list[dict], float]:
    """Parse the JSON configuration into (feeds, cases, tolerance)."""
    try:
        cfg = json.loads(text)
    except json.JSONDecodeError as exc:
        raise FlashBalanceError(f"invalid JSON: {exc}") from exc
    if not isinstance(cfg, dict):
        raise FlashBalanceError("configuration must be a JSON object")
    raw_feeds = cfg.get("feeds")
    if not isinstance(raw_feeds, list) or not raw_feeds:
        raise FlashBalanceError("'feeds' must be a non-empty list")
    feeds = []
    for i, entry in enumerate(raw_feeds):
        flows = entry.get("flows") if isinstance(entry, dict) else None
        if not isinstance(flows, dict) or not flows:
            raise FlashBalanceError(f"feed #{i} needs a non-empty 'flows' object")
        feeds.append(
            {str(c): _as_number(v, f"feed #{i} flow") for c, v in flows.items()}
        )
    raw_cases = cfg.get("cases")
    if not isinstance(raw_cases, list) or not raw_cases:
        raise FlashBalanceError("'cases' must be a non-empty list")
    cases = []
    for i, case in enumerate(raw_cases):
        if not isinstance(case, dict):
            raise FlashBalanceError(f"case #{i} must be an object")
        name = str(case.get("name", f"case{i}"))
        stages = case.get("stages")
        if not isinstance(stages, list) or len(stages) != 2:
            raise FlashBalanceError(f"case {name!r} needs exactly two stages")
        parsed_stages = []
        for j, stage in enumerate(stages):
            kv = stage.get("k_values") if isinstance(stage, dict) else None
            if not isinstance(kv, dict) or not kv:
                raise FlashBalanceError(
                    f"case {name!r} stage {j + 1} needs a non-empty 'k_values'"
                )
            parsed = {
                str(c): _as_number(v, f"case {name!r} K-value")
                for c, v in kv.items()
            }
            for c, k in parsed.items():
                if k <= 0.0:
                    raise FlashBalanceError(
                        f"case {name!r}: K-value for {c!r} must be positive"
                    )
            parsed_stages.append({"k_values": parsed})
        cases.append({"name": name, "stages": parsed_stages})
    tolerance = _as_number(cfg.get("tolerance", 1e-12), "tolerance")
    if tolerance <= 0.0:
        raise FlashBalanceError("tolerance must be positive")
    return feeds, cases, tolerance


def render_streams_csv(results: dict[str, CascadeResult]) -> str:
    """Render the stream table; cases sorted for order independence."""
    buf = _io.StringIO()
    writer = csv.writer(buf, lineterminator="\n")
    writer.writerow(["case", "stream", "component", "molar_flow", "mole_fraction"])
    for case_name in sorted(results):
        res = results[case_name]
        streams = [("feed", res.mixed_feed)] + [
            (name, res.products[name].flows) for name in ("L1", "V2", "L2")
        ]
        for stream_name, flows in streams:
            total = sum(flows.values())
            writer.writerow([case_name, stream_name, "TOTAL", f"{total:.12g}", ""])
            if total <= 0.0:
                continue
            for comp in sorted(flows):
                writer.writerow(
                    [case_name, stream_name, comp,
                     f"{flows[comp]:.12g}", f"{flows[comp] / total:.12g}"]
                )
    return buf.getvalue()


def render_summary_json(results: dict[str, CascadeResult]) -> str:
    """Render the solver summary; cases sorted for order independence."""
    cases = []
    for case_name in sorted(results):
        res = results[case_name]
        stages = []
        for report in res.stages:
            entry = {
                "stage": report.stage,
                "feed_total": sum(report.feed.values()),
                "balance_residual": res.stage_balances[report.stage - 1],
            }
            if report.result is None:
                entry["status"] = "skipped-empty-feed"
            else:
                r = report.result
                entry.update({
                    "status": "solved",
                    "phase": r.phase,
                    "vapor_fraction": r.vapor_fraction,
                    "degenerate": r.degenerate,
                    "iterations": r.iterations,
                    "equilibrium_residual": r.equilibrium_residual,
                })
            stages.append(entry)
        cases.append({
            "case": case_name,
            "components": res.components,
            "mixed_feed": res.mixed_feed,
            "stages": stages,
            "products": {
                name: {
                    "total": res.products[name].total,
                    "composition": res.products[name].composition(),
                }
                for name in ("L1", "V2", "L2")
            },
            "overall_balance_residual": res.overall_balance,
            "tolerance": res.tolerance,
        })
    return json.dumps({"cases": cases}, indent=2) + "\n"
