"""Two-stage flash cascade and material balance checks."""

from __future__ import annotations

from dataclasses import dataclass

from .errors import FlashBalanceError
from .flash import FlashResult, flash_feed


@dataclass
class Stream:
    """A material stream: component molar flows."""

    name: str
    flows: dict[str, float]

    @property
    def total(self) -> float:
        return sum(self.flows.values())

    @property
    def empty(self) -> bool:
        return self.total <= 0.0

    def composition(self) -> dict[str, float] | None:
        """Mole fractions, or None for an empty stream."""
        if self.empty:
            return None
        total = self.total
        return {c: f / total for c, f in self.flows.items()}


def mix_feeds(feeds: list[dict[str, float]]) -> dict[str, float]:
    """Combine feed streams component-wise; reject negative flows."""
    mixed: dict[str, float] = {}
    for feed in feeds:
        for comp, flow in feed.items():
            if flow < 0.0:
                raise FlashBalanceError(
                    f"negative molar flow for component {comp!r}"
                )
            mixed[comp] = mixed.get(comp, 0.0) + flow
    if not mixed or all(v == 0.0 for v in mixed.values()):
        raise FlashBalanceError("total feed is zero; nothing to separate")
    return mixed


@dataclass
class StageReport:
    stage: int
    result: FlashResult | None  # None when the stage receives no feed
    feed: dict[str, float]


@dataclass
class CascadeResult:
    components: list[str]
    mixed_feed: dict[str, float]
    stages: list[StageReport]
    products: dict[str, Stream]  # L1, V2, L2
    stage_balances: list[dict[str, float]]
    overall_balance: dict[str, float]
    tolerance: float


def run_cascade(
    feeds: list[dict[str, float]],
    stage_k: list[dict[str, float]],
    tolerance: float = 1e-12,
) -> CascadeResult:
    """Run the two-stage cascade for one set of K-values."""
    if len(stage_k) != 2:
        raise FlashBalanceError("exactly two stages of K-values are required")
    mixed = mix_feeds(feeds)
    components = sorted(mixed)

    stage1 = flash_feed(mixed, stage_k[0], tolerance)
    liquid1 = stage1.liquid
    vapor1 = stage1.vapor

    vapor1_total = sum(vapor1.values())
    if vapor1_total > 0.0:
        stage2: FlashResult | None = flash_feed(vapor1, stage_k[1], tolerance)
        vapor2 = stage2.vapor
        liquid2 = stage2.liquid
    else:
        stage2 = None
        vapor2 = {c: 0.0 for c in components}
        liquid2 = {c: 0.0 for c in components}

    products = {
        "L1": Stream("L1", liquid1),
        "V2": Stream("V2", vapor2),
        "L2": Stream("L2", liquid2),
    }

    stage_balances = []
    bal1 = {
        c: mixed[c] - (liquid1[c] + vapor1[c]) for c in components
    }
    stage_balances.append(bal1)
    if stage2 is not None:
        bal2 = {
            c: vapor1[c] - (vapor2[c] + liquid2[c]) for c in components
        }
    else:
        bal2 = {c: 0.0 for c in components}
    stage_balances.append(bal2)

    overall = {
        c: mixed[c] - (liquid1[c] + vapor2[c] + liquid2[c])
        for c in components
    }

    return CascadeResult(
        components=components,
        mixed_feed=mixed,
        stages=[StageReport(1, stage1, mixed), StageReport(2, stage2, vapor1)],
        products=products,
        stage_balances=stage_balances,
        overall_balance=overall,
        tolerance=tolerance,
    )
