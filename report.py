"""将 Meta 分析结果序列化为 JSON / CSV 报告。"""
from __future__ import annotations

import csv
import json
from pathlib import Path

from meta_analysis import MetaAnalysisResult, PooledResult


def _round(value: float | None, digits: int = 6) -> float | None:
    return None if value is None else round(value, digits)


def _pooled_dict(pooled: PooledResult | None) -> dict | None:
    if pooled is None:
        return None
    data = {
        "log_or": _round(pooled.log_or),
        "se": _round(pooled.se),
        "odds_ratio": _round(pooled.odds_ratio),
        "ci_lower": _round(pooled.ci_lower),
        "ci_upper": _round(pooled.ci_upper),
    }
    if pooled.heterogeneity is not None:
        h = pooled.heterogeneity
        data["heterogeneity"] = {
            "estimable": h.estimable,
            "q": _round(h.q),
            "df": h.df,
            "i_squared_pct": _round(h.i_squared),
            "tau_squared": _round(h.tau_squared),
        }
    return data


def to_dict(result: MetaAnalysisResult) -> dict:
    return {
        "n_included": len(result.studies),
        "n_excluded": len(result.excluded),
        "excluded": [
            {"study_id": e.study_id, "reason": e.reason} for e in result.excluded
        ],
        "studies": [
            {
                "study_id": s.study_id,
                "log_or": _round(s.log_or),
                "variance": _round(s.variance),
                "odds_ratio": _round(s.odds_ratio),
                "ci_lower": _round(s.ci_lower),
                "ci_upper": _round(s.ci_upper),
                "continuity_corrected": s.corrected,
                "weight_fixed_pct": _round(s.weight_fixed),
                "weight_random_pct": _round(s.weight_random),
            }
            for s in result.studies
        ],
        "pooled_fixed": _pooled_dict(result.fixed),
        "pooled_random": _pooled_dict(result.random),
        "leave_one_out": [
            {
                "omitted_study_id": loo.omitted_id,
                "n_remaining": loo.n_remaining,
                "note": loo.note,
                "fixed": _pooled_dict(loo.fixed),
                "random": _pooled_dict(loo.random),
            }
            for loo in result.leave_one_out
        ],
    }


def write_json(result: MetaAnalysisResult, path: str | Path) -> None:
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(to_dict(result), fh, ensure_ascii=False, indent=2)
        fh.write("\n")


def write_csv(result: MetaAnalysisResult, path: str | Path) -> None:
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(
            [
                "section",
                "study_id",
                "model",
                "log_or",
                "odds_ratio",
                "ci_lower",
                "ci_upper",
                "variance_or_se",
                "weight_fixed_pct",
                "weight_random_pct",
                "continuity_corrected",
                "q",
                "df",
                "i_squared_pct",
                "tau_squared",
                "note",
            ]
        )
        for s in result.studies:
            writer.writerow(
                [
                    "study",
                    s.study_id,
                    "",
                    _round(s.log_or),
                    _round(s.odds_ratio),
                    _round(s.ci_lower),
                    _round(s.ci_upper),
                    _round(s.variance),
                    _round(s.weight_fixed),
                    _round(s.weight_random),
                    s.corrected,
                    "",
                    "",
                    "",
                    "",
                    "",
                ]
            )
        for e in result.excluded:
            writer.writerow(["excluded", e.study_id] + [""] * 13 + [e.reason])
        for label, pooled in (("fixed", result.fixed), ("random", result.random)):
            h = pooled.heterogeneity
            writer.writerow(
                [
                    "pooled",
                    "",
                    label,
                    _round(pooled.log_or),
                    _round(pooled.odds_ratio),
                    _round(pooled.ci_lower),
                    _round(pooled.ci_upper),
                    _round(pooled.se),
                    "",
                    "",
                    "",
                    _round(h.q) if h else "",
                    h.df if h else "",
                    _round(h.i_squared) if h else "",
                    _round(h.tau_squared) if h else "",
                    "" if (h is None or h.estimable) else "异质性不可估计",
                ]
            )
        for loo in result.leave_one_out:
            for label, pooled in (("fixed", loo.fixed), ("random", loo.random)):
                if pooled is None:
                    writer.writerow(
                        ["leave_one_out", loo.omitted_id, label] + [""] * 12 + [loo.note or ""]
                    )
                    continue
                h = pooled.heterogeneity
                writer.writerow(
                    [
                        "leave_one_out",
                        loo.omitted_id,
                        label,
                        _round(pooled.log_or),
                        _round(pooled.odds_ratio),
                        _round(pooled.ci_lower),
                        _round(pooled.ci_upper),
                        _round(pooled.se),
                        "",
                        "",
                        "",
                        _round(h.q) if h else "",
                        h.df if h else "",
                        _round(h.i_squared) if h else "",
                        _round(h.tau_squared) if h else "",
                        loo.note or "",
                    ]
                )
