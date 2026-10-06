"""Two-stage flash cascade with component balance checks."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .flash import FlashError, FlashResult, solve_flash

BALANCE_TOLERANCE = 1e-9


@dataclass
class StageResult:
    name: str
    feed: dict[str, float]
    flash: FlashResult | None  # None when the stage received no feed
    skipped: bool = False


@dataclass
class TwoStageResult:
    components: list[str]
    combined_feed: dict[str, float]
    stage1: StageResult
    stage2: StageResult
    products: dict[str, dict[str, float]]  # name -> component flows
    balance: dict[str, dict[str, float]]  # component -> feed/product/residual
    max_balance_residual: float
    equilibrium_residual: float
    warnings: list[str] = field(default_factory=list)


def mix_feeds(feeds: list[dict[str, float]]) -> tuple[list[str], dict[str, float]]:
    """Mix feed streams component-wise; order of components follows first appearance."""
    components: list[str] = []
    combined: dict[str, float] = {}
    for stream in feeds:
        for name, value in stream.items():
            v = float(value)
            if not math.isfinite(v) or v < 0.0:
                raise FlashError(f"feed flow for {name!r} must be a finite non-negative number")
            if name not in combined:
                components.append(name)
                combined[name] = 0.0
            combined[name] += v
    if not combined or sum(combined.values()) <= 0.0:
        raise FlashError("all feed streams are zero; nothing to separate")
    return components, combined


def run_two_stage(
    feeds: list[dict[str, float]],
    k_stage1: dict[str, float],
    k_stage2: dict[str, float],
) -> TwoStageResult:
    components, combined = mix_feeds(feeds)

    flash1 = solve_flash(combined, k_stage1)
    stage1 = StageResult("stage1", combined, flash1)

    vapor_total = sum(flash1.vapor_flows.values())
    warnings = list(flash1.warnings)
    if vapor_total <= 0.0:
        stage2 = StageResult("stage2", {n: 0.0 for n in components}, None, skipped=True)
        warnings.append("stage 1 produced no vapor; stage 2 outputs are empty streams")
        products = {
            "stage1_liquid": flash1.liquid_flows,
            "stage2_liquid": {n: 0.0 for n in components},
            "stage2_vapor": {n: 0.0 for n in components},
        }
        eq_residual = abs(flash1.residual)
    else:
        flash2 = solve_flash(flash1.vapor_flows, k_stage2)
        warnings.extend(f"stage2: {w}" for w in flash2.warnings)
        stage2 = StageResult("stage2", flash1.vapor_flows, flash2)
        products = {
            "stage1_liquid": flash1.liquid_flows,
            "stage2_liquid": flash2.liquid_flows,
            "stage2_vapor": flash2.vapor_flows,
        }
        eq_residual = max(abs(flash1.residual), abs(flash2.residual))

    balance: dict[str, dict[str, float]] = {}
    max_residual = 0.0
    for name in components:
        out = sum(p[name] for p in products.values())
        residual = combined[name] - out
        scale = max(combined[name], 1.0)
        rel = abs(residual) / scale
        max_residual = max(max_residual, rel)
        balance[name] = {"feed": combined[name], "product": out, "residual": residual}
    if max_residual > BALANCE_TOLERANCE:
        raise FlashError(f"component balance does not close (max relative residual {max_residual:.3e})")

    return TwoStageResult(components, combined, stage1, stage2, products,
                          balance, max_residual, eq_residual, warnings)
