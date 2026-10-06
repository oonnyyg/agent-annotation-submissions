"""Scenario orchestration: run MVA per scenario and cross-check results."""

from __future__ import annotations

from dataclasses import dataclass

from .model import Network
from .mva import PopulationPoint, run_mva
from .visits import visit_ratios


class ConsistencyError(RuntimeError):
    """Raised when an internal MVA invariant is violated."""


@dataclass(frozen=True)
class ScenarioReport:
    name: str
    population: int
    final: PopulationPoint
    curve: list[PopulationPoint]
    throughput_change_vs_baseline: float | None  # relative change, None for baseline


@dataclass(frozen=True)
class AnalysisReport:
    reference_station: str
    visit_ratios: dict[str, float]
    scenarios: list[ScenarioReport]


def _check_point(point: PopulationPoint) -> None:
    total = sum(s.queue_length for s in point.stations)
    if abs(total - point.population) > 1e-6 * max(1, point.population):
        raise ConsistencyError(
            f"population {point.population}: station populations sum to {total}, "
            f"expected {point.population}")
    for s in point.stations:
        if s.kind == "single" and s.load > 1.0 + 1e-9:
            raise ConsistencyError(
                f"population {point.population}: utilization of {s.name!r} is "
                f"{s.load:.6f} > 1; the network is unstable for this configuration")


def analyze(net: Network) -> AnalysisReport:
    """Run every scenario independently and compare throughput to the baseline."""
    ratios = visit_ratios(net)
    reports: list[ScenarioReport] = []
    baseline_throughput: float | None = None
    for scenario in net.scenarios:
        curve = run_mva(net, scenario, ratios)
        for point in curve:
            _check_point(point)
        final = curve[-1]
        if baseline_throughput is None:
            baseline_throughput = final.throughput
            change = None
        else:
            change = ((final.throughput - baseline_throughput) / baseline_throughput
                      if baseline_throughput else None)
        reports.append(ScenarioReport(
            name=scenario.name, population=scenario.population,
            final=final, curve=curve,
            throughput_change_vs_baseline=change))
    return AnalysisReport(reference_station=net.reference_station,
                          visit_ratios=ratios, scenarios=reports)
