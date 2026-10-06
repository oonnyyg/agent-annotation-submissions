"""Event-driven stick-slip simulation.

The external force is piecewise constant, so between events (force switch,
stop, restart) the acceleration is constant and the motion is integrated
analytically. Results therefore do not depend on the output sampling rate.

Event ordering at a coincident instant: a sliding block first reaches the
zero-velocity (stopped) state, then the *new* force segment decides whether
it stays stuck or restarts (possibly in the opposite direction). No
instantaneous reverse displacement is fabricated.
"""

from __future__ import annotations

from dataclasses import dataclass, field

from .model import Scenario

_EPS = 1e-12


@dataclass
class Phase:
    """Analytic motion phase on [t0, t1]: x = x0 + v0*dt + 0.5*a*dt^2."""

    t0: float
    t1: float
    x0: float
    v0: float
    a: float
    force: float
    moving: bool
    # Cumulative integrals at t1 (absolute values at phase end).
    distance: float = 0.0
    work_ext: float = 0.0
    dissipation: float = 0.0


@dataclass
class Event:
    t: float
    kind: str  # "start_moving" | "stop" | "stick" | "force_switch"
    x: float
    v: float
    force: float
    detail: str = ""


@dataclass
class SimResult:
    scenario: Scenario
    phases: list[Phase] = field(default_factory=list)
    events: list[Event] = field(default_factory=list)


def _sliding_accel(scn: Scenario, force: float, velocity: float) -> float:
    if velocity > 0.0:
        direction = 1.0
    elif velocity < 0.0:
        direction = -1.0
    else:
        # Starting from rest: kinetic friction opposes the impending
        # motion, i.e. acts against the driving force.
        direction = 1.0 if force >= 0.0 else -1.0
    friction = scn.kinetic_friction * direction
    return (force - friction) / scn.mass


def simulate(scn: Scenario) -> SimResult:
    result = SimResult(scenario=scn)
    n = scn.normal_force
    f_s_max = scn.max_static_friction
    f_k = scn.kinetic_friction

    t = 0.0
    x = scn.x0
    v = scn.v0
    distance = 0.0
    work_ext = 0.0
    dissipation = 0.0
    seg_idx = 0

    def force_at(time: float) -> tuple[float, int]:
        # Left-closed, right-open segments.
        idx = seg_idx
        while idx + 1 < len(scn.segments) and scn.segments[idx + 1].start <= time + _EPS:
            idx += 1
        return scn.segments[idx].force, idx

    moving = abs(v) > 0.0
    if not moving:
        f0 = scn.segments[0].force
        if abs(f0) > f_s_max:
            moving = True
            result.events.append(
                Event(0.0, "start_moving", x, 0.0, f0,
                      "initial force exceeds max static friction")
            )
        else:
            result.events.append(
                Event(0.0, "stick", x, 0.0, f0,
                      "initial force within static friction")
            )

    while t < scn.t_end - _EPS:
        force, seg_idx = force_at(t)
        next_switch = (
            scn.segments[seg_idx + 1].start
            if seg_idx + 1 < len(scn.segments)
            else scn.t_end
        )
        horizon = min(next_switch, scn.t_end)

        if not moving:
            # Stuck until the next force switch (or t_end).
            result.phases.append(
                Phase(t, horizon, x, 0.0, 0.0, force, False,
                      distance, work_ext, dissipation)
            )
            t = horizon
            if t < scn.t_end - _EPS:
                new_force = scn.segments[seg_idx + 1].force
                seg_idx += 1
                result.events.append(
                    Event(t, "force_switch", x, 0.0, new_force,
                          "force changed while stuck")
                )
                if abs(new_force) > f_s_max:
                    moving = True
                    result.events.append(
                        Event(t, "start_moving", x, 0.0, new_force,
                              "new force exceeds max static friction")
                    )
            continue

        # Sliding with constant acceleration until stop or force switch.
        a = _sliding_accel(scn, force, v)
        t_stop = None
        if a * v < 0.0:  # acceleration opposes motion
            t_stop = t - v / a
            if t_stop > horizon + _EPS:
                t_stop = None
            else:
                # A stop landing on a force boundary is judged by the new
                # segment after reaching the zero-velocity state.
                t_stop = min(t_stop, horizon)

        if t_stop is not None:
            dt = t_stop - t
        else:
            dt = horizon - t
        dx = v * dt + 0.5 * a * dt * dt
        x_end = x + dx
        v_end = 0.0 if t_stop is not None else v + a * dt
        distance += abs(dx)
        work_ext += force * dx
        dissipation += f_k * abs(dx)
        result.phases.append(
            Phase(t, t + dt, x, v, a, force, True, distance, work_ext, dissipation)
        )
        x, v = x_end, v_end
        t = t + dt

        if t_stop is not None:
            # Reached zero velocity: first record the stop, then judge the
            # force in effect *at this instant* (left-closed segments).
            force_now, seg_idx = force_at(t)
            result.events.append(
                Event(t, "stop", x, 0.0, force_now, "velocity reached zero")
            )
            if abs(force_now) <= f_s_max:
                moving = False
                result.events.append(
                    Event(t, "stick", x, 0.0, force_now,
                          "force within static friction, block sticks")
                )
            else:
                result.events.append(
                    Event(t, "start_moving", x, 0.0, force_now,
                          "force exceeds static friction, restarts")
                )
            continue

        if t < scn.t_end - _EPS:
            seg_idx += 1
            new_force = scn.segments[seg_idx].force
            result.events.append(
                Event(t, "force_switch", x, v, new_force,
                      "force changed while sliding")
            )

    return result


def _state_at(result: SimResult, time: float) -> dict:
    """Exact state and cumulative integrals at an arbitrary time."""
    scn = result.scenario
    time = min(max(time, 0.0), scn.t_end)
    phase = result.phases[-1]
    for candidate in result.phases:
        if candidate.t0 <= time <= candidate.t1:
            phase = candidate
            break
    idx = result.phases.index(phase)
    if idx > 0:
        prev = result.phases[idx - 1]
        dist0, work0, diss0 = prev.distance, prev.work_ext, prev.dissipation
    else:
        dist0 = work0 = diss0 = 0.0

    dtau = time - phase.t0
    dx = phase.v0 * dtau + 0.5 * phase.a * dtau * dtau
    v = phase.v0 + phase.a * dtau
    if not phase.moving:
        v = 0.0
    return {
        "t": time,
        "x": phase.x0 + dx,
        "v": v,
        "moving": phase.moving,
        "distance": dist0 + abs(dx),
        "work_ext": work0 + phase.force * dx,
        "dissipation": diss0 + scn.kinetic_friction * abs(dx),
    }


def sample_trajectory(result: SimResult) -> list[dict]:
    """Sample the trajectory at 0, sample_dt, 2*sample_dt, ..., t_end.

    Sampling only evaluates the analytic solution; it never influences the
    simulation, so results are independent of the sampling frequency.
    """
    scn = result.scenario
    rows = []
    n_steps = int(scn.t_end / scn.sample_dt)
    times = [i * scn.sample_dt for i in range(n_steps + 1)]
    if not times or times[-1] < scn.t_end - _EPS:
        times.append(scn.t_end)
    else:
        times[-1] = scn.t_end
    for time in times:
        rows.append(_state_at(result, time))
    return rows


def energy_summary(result: SimResult) -> dict:
    """Work/energy balance over the whole simulation.

    residual = work_ext - dissipation - delta_kinetic  (should be ~0).
    """
    scn = result.scenario
    final = _state_at(result, scn.t_end)
    ke0 = 0.5 * scn.mass * scn.v0 * scn.v0
    ke1 = 0.5 * scn.mass * final["v"] * final["v"]
    residual = final["work_ext"] - final["dissipation"] - (ke1 - ke0)
    return {
        "distance_total": final["distance"],
        "displacement_net": final["x"] - scn.x0,
        "work_external": final["work_ext"],
        "friction_dissipation": final["dissipation"],
        "kinetic_energy_initial": ke0,
        "kinetic_energy_final": ke1,
        "kinetic_energy_change": ke1 - ke0,
        "residual": residual,
    }
