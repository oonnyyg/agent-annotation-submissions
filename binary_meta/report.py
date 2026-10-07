"""Build JSON-serializable report structures and CSV text output."""

from __future__ import annotations

import csv
import io
import json

from .stats import ExcludedStudy, MetaResult, StudyEffect


def _pooled_dict(pooled) -> dict | None:
    if pooled is None:
        return None
    orr, lo, hi = pooled.or_ci
    return {
        "log_or": pooled.log_or,
        "se": pooled.se,
        "ci95_log": [pooled.ci_lo, pooled.ci_hi],
        "odds_ratio": orr,
        "ci95_or": [lo, hi],
    }


def _het_dict(meta: MetaResult) -> dict:
    het = meta.heterogeneity
    return {
        "cochran_q": het.q,
        "df": het.df,
        "i2_percent": het.i2,
        "tau2": het.tau2,
        "estimable": het.q is not None,
    }


def _meta_dict(meta: MetaResult) -> dict:
    return {
        "k": meta.k,
        "fixed_effect": _pooled_dict(meta.fixed),
        "random_effect": _pooled_dict(meta.random),
        "heterogeneity": _het_dict(meta),
    }


def build_report(
    effects: list[StudyEffect],
    excluded: list[ExcludedStudy],
    meta: MetaResult,
    loo: list[tuple[str, MetaResult]],
) -> dict:
    studies = []
    for effect, fw, rw in zip(effects, meta.fixed_weights, meta.random_weights):
        orr, lo, hi = effect.or_ci
        studies.append(
            {
                "study_id": effect.study_id,
                "log_or": effect.log_or,
                "variance": effect.variance,
                "se": effect.se,
                "odds_ratio": orr,
                "ci95_or": [lo, hi],
                "continuity_correction": effect.corrected,
                "fixed_weight_percent": fw,
                "random_weight_percent": rw,
            }
        )
    leave_one_out = []
    for omitted_id, result in loo:
        leave_one_out.append({"omitted_study": omitted_id, "result": _meta_dict(result)})
    return {
        "method": {
            "effect": "log odds ratio (experimental vs control)",
            "fixed_effect": "inverse-variance",
            "random_effect": "DerSimonian-Laird",
            "interval": "normal-approximation 95% CI on log scale, exponentiated",
            "continuity_correction": 0.5,
        },
        "n_included": len(effects),
        "n_excluded": len(excluded),
        "excluded": [{"study_id": e.study_id, "reason": e.reason} for e in excluded],
        "studies": studies,
        "pooled": _meta_dict(meta),
        "leave_one_out": leave_one_out,
    }


def to_json(report: dict) -> str:
    return json.dumps(report, indent=2, ensure_ascii=False)


def _fmt(value) -> str:
    if value is None:
        return ""
    if isinstance(value, float):
        return f"{value:.6g}"
    return str(value)


_CSV_HEADER = [
    "section",
    "study_id",
    "k",
    "log_or",
    "se",
    "or",
    "ci95_lo",
    "ci95_hi",
    "fixed_weight_pct",
    "random_weight_pct",
    "cochran_q",
    "i2_pct",
    "tau2",
    "note",
]


def to_csv(report: dict) -> str:
    """Flat CSV with a 'section' column distinguishing row types."""
    out = io.StringIO()
    writer = csv.writer(out)
    writer.writerow(_CSV_HEADER)
    for s in report["studies"]:
        writer.writerow(
            [
                "study",
                s["study_id"],
                "",
                _fmt(s["log_or"]),
                _fmt(s["se"]),
                _fmt(s["odds_ratio"]),
                _fmt(s["ci95_or"][0]),
                _fmt(s["ci95_or"][1]),
                _fmt(s["fixed_weight_percent"]),
                _fmt(s["random_weight_percent"]),
                "",
                "",
                "",
                "continuity-corrected" if s["continuity_correction"] else "",
            ]
        )
    for e in report["excluded"]:
        writer.writerow(
            ["excluded", e["study_id"], "", "", "", "", "", "", "", "", "", "", "", e["reason"]]
        )

    def pooled_row(section: str, k, pooled, het, note: str = ""):
        if pooled is None:
            writer.writerow(
                [section, "", k, "", "", "", "", "", "", "", "", "", "", note or "no result"]
            )
            return
        writer.writerow(
            [
                section,
                "",
                k,
                _fmt(pooled["log_or"]),
                _fmt(pooled["se"]),
                _fmt(pooled["odds_ratio"]),
                _fmt(pooled["ci95_or"][0]),
                _fmt(pooled["ci95_or"][1]),
                "",
                "",
                _fmt(het["cochran_q"]),
                _fmt(het["i2_percent"]),
                _fmt(het["tau2"]),
                note,
            ]
        )

    p = report["pooled"]
    pooled_row("pooled_fixed", p["k"], p["fixed_effect"], p["heterogeneity"])
    pooled_row("pooled_random", p["k"], p["random_effect"], p["heterogeneity"])
    for entry in report["leave_one_out"]:
        r = entry["result"]
        note = "" if r["heterogeneity"]["estimable"] else "heterogeneity not estimable"
        pooled_row(
            f"loo_fixed_omit_{entry['omitted_study']}",
            r["k"],
            r["fixed_effect"],
            r["heterogeneity"],
            note,
        )
        pooled_row(
            f"loo_random_omit_{entry['omitted_study']}",
            r["k"],
            r["random_effect"],
            r["heterogeneity"],
            note,
        )
    return out.getvalue()
