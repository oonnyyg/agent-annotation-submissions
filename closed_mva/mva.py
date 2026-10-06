"""Mean value analysis recursion for a single-class closed network."""

from __future__ import annotations

from dataclasses import dataclass

from .model import Network, Scenario


@dataclass(frozen=True)
class StationResult:
    name: str
    kind: str
    mean_service_time: float
    visit_ratio: float
    residence_time: float   # per-visit mean residence time R_i
    queue_length: float     # mean number at station Q_i
    load: float             # utilization (single) or busy servers (infinite)
    load_kind: str          # "utilization" or "busy_servers"


@dataclass(frozen=True)
class PopulationPoint:
    population: int
    throughput: float       # reference-station throughput X0
    cycle_time: float       # full cycle time at the reference station
    stations: list[StationResult]


def run_mva(net: Network, scenario: Scenario, ratios: dict[str, float]) -> list[PopulationPoint]:
    """Run the MVA recursion from population 0 up to the scenario population.

    Each scenario starts from an empty network, so queue state is never
    shared between scenarios.
    """
    service = {s.name: scenario.service_time_overrides.get(s.name, s.mean_service_time)
               for s in net.stations}
    kinds = {s.name: s.kind for s in net.stations}
    names = net.station_names()
    points: list[PopulationPoint] = []
    queue = {n: 0.0 for n in names}

    def snapshot(pop: int, throughput: float, cycle: float,
                 residence: dict[str, float]) -> PopulationPoint:
        stations = []
        for n in names:
            x_i = throughput * ratios[n]
            if kinds[n] == "single":
                load, load_kind = x_i * service[n], "utilization"
            else:
                load, load_kind = x_i * service[n], "busy_servers"
            stations.append(StationResult(
                name=n, kind=kinds[n], mean_service_time=service[n],
                visit_ratio=ratios[n], residence_time=residence[n],
                queue_length=queue[n], load=load, load_kind=load_kind))
        return PopulationPoint(population=pop, throughput=throughput,
                               cycle_time=cycle, stations=stations)

    # Population 0: the empty network, reported explicitly.
    empty_residence = {n: service[n] for n in names}
    points.append(snapshot(0, 0.0, sum(ratios[n] * service[n] for n in names),
                           empty_residence))
    for pop in range(1, scenario.population + 1):
        residence = {}
        for n in names:
            if kinds[n] == "single":
                residence[n] = service[n] * (1.0 + queue[n])
            else:  # infinite server: no queueing, delay is constant
                residence[n] = service[n]
        cycle = sum(ratios[n] * residence[n] for n in names)
        throughput = pop / cycle
        for n in names:
            queue[n] = throughput * ratios[n] * residence[n]
        points.append(snapshot(pop, throughput, cycle, residence))
    return points
