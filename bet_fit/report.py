"""Plain-text report rendering for BET analysis results."""

from __future__ import annotations

from .data import Config, Sample
from .fit import analyze_sample


def _fmt(value, digits=6):
    if value is None:
        return "undefined"
    return f"{value:.{digits}g}"


def render_sample(sample: Sample, config: Config):
    """Render one sample's report section; returns (text, success)."""
    selected, candidates = analyze_sample(
        sample.pressures, sample.quantities,
        config.cross_section_nm2, config.min_points, config.interval)

    lines = [f"Sample: {sample.name}", "=" * (8 + len(sample.name))]
    ok = selected is not None and selected.accepted
    if ok:
        lines.append(
            f"Selected interval: p/p0 = {selected.p_start:.6g} .. "
            f"{selected.p_end:.6g} ({selected.n_points} points)")
        lines.append(f"  slope                {selected.slope:.6g} g/mol")
        lines.append(f"  intercept            {selected.intercept:.6g} g/mol")
        lines.append(
            f"  monolayer capacity   "
            f"{selected.monolayer_mol_per_g:.6g} mol/g")
        lines.append(f"  BET constant C       {selected.bet_constant:.6g}")
        lines.append(f"  R^2                  {_fmt(selected.r_squared, 8)}")
        lines.append(
            f"  surface area         {selected.area_m2_per_g:.6g} m^2/g")
        lines.append("  Residuals (fit - observed, BET transform space):")
        for i in range(selected.start, selected.end + 1):
            r = selected.residuals[i - selected.start]
            lines.append(
                f"    p/p0={sample.pressures[i]:.6g}  residual={r:+.6g}")
    else:
        lines.append("FAILED: no interval passed all screening criteria.")
        if config.interval is not None and not candidates:
            lines.append(
                f"Manual interval {config.interval[0]}..{config.interval[1]} "
                f"contains fewer than {config.min_points} points.")
    if candidates:
        lines.append("")
        lines.append("Candidate intervals:")
        lines.append(
            "  range (p/p0)       pts  slope      intercept  "
            "n_m (mol/g)  C          R^2       status")
        for c in candidates:
            if c.accepted:
                status = "accepted"
            else:
                status = "rejected: " + "; ".join(c.rejections)
            lines.append(
                f"  {c.p_start:.6g}..{c.p_end:.6g}"
                f"  {c.n_points:>3}  {c.slope:+.4g}  {c.intercept:+.4g}"
                f"  {_fmt(c.monolayer_mol_per_g):>10}"
                f"  {_fmt(c.bet_constant):>10}"
                f"  {_fmt(c.r_squared, 5):>9}  {status}")
    return "\n".join(lines), ok


def render_report(samples, config):
    """Render the full report; returns (text, all_samples_ok)."""
    manual = ("none (auto enumeration)" if config.interval is None
              else f"{config.interval[0]}..{config.interval[1]}")
    header = [
        "BET surface area report",
        f"cross-section: {config.cross_section_nm2} nm^2, "
        f"min points per interval: {config.min_points}",
        f"manual interval: {manual}",
        "Note: screening criteria are tool-internal checks only; they do not",
        "constitute a full assessment of BET applicability.",
        "",
    ]
    sections = []
    all_ok = True
    for sample in samples:
        text, ok = render_sample(sample, config)
        sections.append(text)
        all_ok = all_ok and ok
    return "\n".join(header) + "\n\n".join(sections) + "\n", all_ok
