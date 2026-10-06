"""Mean value analysis for a single-class closed queueing network."""

from __future__ import annotations

from dataclasses import dataclass

from .model import ConfigError, NetworkConfig, Scenario
from .visits import compute_visit_ratios

_UTILIZATION_TOLERANCE = 1e-9


@dataclass(frozen=True)
class StationResult:
    name: str
    kind: str
    mean_service_time: float
    visit_ratio: float
    residence_time: float
    mean_queue_length: float
    load: float


@dataclass(frozen=True)
class ScenarioResult:
    name: str
    population: int
    throughput: float
    cycle_time: float
    stations: list[StationResult]
    zero_population_queues: list[float]
    relative_throughput: float | None = None


def _service_times(config: NetworkConfig, scenario: Scenario) -> list[float]:
    times = [s.mean_service_time for s in config.stations]
    for name, value in scenario.service_time_overrides.items():
        times[config.station_index[name]] = value
    return times


def run_mva(config: NetworkConfig, scenario: Scenario) -> ScenarioResult:
    """Run MVA for one scenario; each call starts from a zero-population state."""
    ref = config.station_index[config.reference_station]
    visits = compute_visit_ratios(config.routing, ref)
    service = _service_times(config, scenario)
    n_stations = len(config.stations)

    queues = [0.0] * n_stations
    zero_population_queues = list(queues)
    residence = [0.0] * n_stations
    throughput = 0.0

    for n in range(1, scenario.population + 1):
        for i, station in enumerate(config.stations):
            if station.kind == "single":
                residence[i] = service[i] * (1.0 + queues[i])
            else:
                residence[i] = service[i]
        denom = sum(visits[i] * residence[i] for i in range(n_stations))
        throughput = n / denom
        for i in range(n_stations):
            queues[i] = throughput * visits[i] * residence[i]

    results: list[StationResult] = []
    for i, station in enumerate(config.stations):
        load = throughput * visits[i] * service[i]
        if station.kind == "single" and load > 1.0 + _UTILIZATION_TOLERANCE:
            raise ConfigError(
                f"scenario '{scenario.name}': utilization of single-server station "
                f"'{station.name}' would be {load:.6f} (> 1); reduce its service time "
                "or the population")
        results.append(StationResult(
            name=station.name,
            kind=station.kind,
            mean_service_time=service[i],
            visit_ratio=visits[i],
            residence_time=residence[i],
            mean_queue_length=queues[i],
            load=load,
        ))

    total = sum(r.mean_queue_length for r in results)
    if abs(total - scenario.population) > 1e-6:
        raise ConfigError(
            f"scenario '{scenario.name}': mean queue lengths sum to {total}, "
            f"expected population {scenario.population}")

    cycle_time = scenario.population / throughput if throughput > 0.0 else 0.0
    return ScenarioResult(
        name=scenario.name,
        population=scenario.population,
        throughput=throughput,
        cycle_time=cycle_time,
        stations=results,
        zero_population_queues=zero_population_queues,
    )


def run_scenarios(config: NetworkConfig) -> list[ScenarioResult]:
    """Run all scenarios independently and add throughput ratios vs. the baseline."""
    results = [run_mva(config, scenario) for scenario in config.scenarios]
    baseline = results[0].throughput
    out: list[ScenarioResult] = []
    for result in results:
        ratio = result.throughput / baseline if baseline > 0.0 else None
        out.append(ScenarioResult(
            name=result.name,
            population=result.population,
            throughput=result.throughput,
            cycle_time=result.cycle_time,
            stations=result.stations,
            zero_population_queues=result.zero_population_queues,
            relative_throughput=ratio,
        ))
    return out
