"""Explicit cell-centred finite-volume solver for retarded 1-D transport.

Semi-discrete form per cell i (unit cross-section, unit porosity):

    R * dc_i/dt = (F_{i-1/2} - F_{i+1/2}) / dx - decay * R * c_i

with advective flux F = v * c_upstream, diffusive flux
F = -D * (c_{i+1} - c_i) / dx.  The left boundary carries the total input
flux v * c_inlet(t); the right boundary has zero diffusive flux and the
advective flux v * c_last.  Decay acts on the total (retarded) inventory,
so the discrete mass balance

    initial_inventory + cumulative_inflow
        = cumulative_outflow + cumulative_decay + current_inventory

closes exactly with inventory = R * dx * sum(c).
"""

from __future__ import annotations

import math


class TransportError(ValueError):
    """Raised for invalid transport configuration."""


def _finite(name, value):
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        raise TransportError(f"{name} must be a number, got {value!r}")
    value = float(value)
    if not math.isfinite(value):
        raise TransportError(f"{name} must be finite, got {value!r}")
    return value


def _validate(config):
    if not isinstance(config, dict):
        raise TransportError("configuration must be a JSON object")
    length = _finite("length", config.get("length"))
    if length <= 0.0:
        raise TransportError("length must be positive")
    cells = config.get("cells")
    if not isinstance(cells, int) or isinstance(cells, bool) or cells < 2:
        raise TransportError("cells must be an integer >= 2")
    velocity = _finite("velocity", config.get("velocity", 0.0))
    dispersion = _finite("dispersion", config.get("dispersion", 0.0))
    retardation = _finite("retardation", config.get("retardation", 1.0))
    decay = _finite("decay", config.get("decay", 0.0))
    if velocity < 0.0 or dispersion < 0.0:
        raise TransportError("velocity and dispersion must be non-negative")
    if retardation < 1.0:
        raise TransportError("retardation must be >= 1")
    if decay < 0.0:
        raise TransportError("decay must be non-negative")

    inlet = config.get("inlet")
    if not isinstance(inlet, list) or not inlet:
        raise TransportError("inlet must be a non-empty list of [time, concentration]")
    table = []
    prev_t = -1.0
    for row in inlet:
        if not isinstance(row, (list, tuple)) or len(row) != 2:
            raise TransportError("each inlet entry must be [time, concentration]")
        t = _finite("inlet time", row[0])
        c = _finite("inlet concentration", row[1])
        if t < 0.0 or c < 0.0:
            raise TransportError("inlet times and concentrations must be non-negative")
        if t <= prev_t:
            raise TransportError("inlet times must be strictly increasing")
        prev_t = t
        table.append((t, c))
    if table[0][0] != 0.0:
        raise TransportError("inlet table must start at time 0")

    initial = config.get("initial", 0.0)
    if isinstance(initial, (int, float)) and not isinstance(initial, bool):
        initial = [_finite("initial", initial)] * cells
    elif isinstance(initial, list) and len(initial) == cells:
        initial = [_finite("initial", v) for v in initial]
    else:
        raise TransportError("initial must be a number or a list of length cells")
    if any(v < 0.0 for v in initial):
        raise TransportError("initial concentrations must be non-negative")

    times = config.get("output_times")
    if not isinstance(times, list) or not times:
        raise TransportError("output_times must be a non-empty list")
    out_times = []
    prev_t = -1.0
    for t in times:
        t = _finite("output time", t)
        if t < 0.0 or t <= prev_t:
            raise TransportError("output times must be non-negative and increasing")
        prev_t = t
        out_times.append(t)

    return {
        "length": length,
        "cells": cells,
        "velocity": velocity,
        "dispersion": dispersion,
        "retardation": retardation,
        "decay": decay,
        "inlet": table,
        "initial": initial,
        "output_times": out_times,
    }


def _inlet_at(table, t):
    """Piecewise-constant inlet concentration, last value held forever."""
    value = table[0][1]
    for t_k, c_k in table:
        if t_k > t:
            break
        value = c_k
    return value


def _stable_dt(cfg, dx):
    """Largest stable explicit step for advection + diffusion + decay.

    Uses the combined restriction (rates add, so their sum bounds the
    step) with a safety factor below the marginal limit.
    """
    rate = 0.0
    if cfg["velocity"] > 0.0:
        rate += cfg["velocity"] / (cfg["retardation"] * dx)
    if cfg["dispersion"] > 0.0:
        rate += 2.0 * cfg["dispersion"] / (cfg["retardation"] * dx * dx)
    if cfg["decay"] > 0.0:
        rate += cfg["decay"]
    if rate == 0.0:
        return math.inf
    return 0.9 / rate


def _event_times(cfg, t_end):
    """Times the integrator must land on exactly: inlet jumps + outputs."""
    events = sorted(
        t for t, _ in cfg["inlet"] if 0.0 < t <= t_end
    )
    events.extend(cfg["output_times"])
    return sorted(set(events))


def simulate(config):
    """Run the simulation and return profiles, outlet curve and mass budget."""
    cfg = _validate(config)
    n = cfg["cells"]
    dx = cfg["length"] / n
    v = cfg["velocity"]
    diff = cfg["dispersion"]
    retard = cfg["retardation"]
    decay = cfg["decay"]
    c = list(cfg["initial"])

    t_end = cfg["output_times"][-1]
    events = _event_times(cfg, t_end)
    dt_stable = _stable_dt(cfg, dx)

    inventory0 = retard * dx * sum(c)
    inventory = inventory0
    inflow = 0.0
    outflow = 0.0
    decayed = 0.0

    profiles = []
    outlet = []
    out_iter = iter(cfg["output_times"])
    next_out = next(out_iter, None)

    def record(t_now):
        profiles.append({"time": t_now, "concentration": list(c)})
        outlet.append({"time": t_now, "concentration": c[-1]})

    t = 0.0
    if next_out == 0.0:
        record(0.0)
        next_out = next(out_iter, None)

    for t_event in events:
        while t < t_event:
            dt = min(dt_stable, t_event - t)
            cin = _inlet_at(cfg["inlet"], t)
            # Face fluxes: face 0 is the left boundary (total flux v*cin),
            # face n is the right boundary (zero diffusive, v*c_last).
            flux = [0.0] * (n + 1)
            flux[0] = v * cin
            for i in range(1, n):
                up = c[i - 1] if v >= 0.0 else c[i]
                flux[i] = v * up - diff * (c[i] - c[i - 1]) / dx
            flux[n] = v * c[-1]
            decay_step = decay * retard * dx * sum(c) * dt
            decayed += decay_step
            for i in range(n):
                dc = (flux[i] - flux[i + 1]) / (retard * dx) - decay * c[i]
                c[i] += dt * dc
            inflow += flux[0] * dt
            outflow += flux[n] * dt
            inventory += dt * (flux[0] - flux[n]) - decay_step
            t += dt
        if next_out is not None and t_event == next_out:
            record(t_event)
            next_out = next(out_iter, None)

    inventory_state = retard * dx * sum(c)
    residual = inventory0 + inflow - outflow - decayed - inventory
    scale = max(abs(inventory0) + abs(inflow), abs(outflow) + abs(decayed) + abs(inventory), 1e-300)
    return {
        "dx": dx,
        "dt_stable": dt_stable,
        "profiles": profiles,
        "outlet": outlet,
        "mass_balance": {
            "initial_inventory": inventory0,
            "cumulative_inflow": inflow,
            "cumulative_outflow": outflow,
            "cumulative_decay": decayed,
            "final_inventory": inventory,
            "inventory_from_state": inventory_state,
            "residual": residual,
            "relative_residual": residual / scale,
        },
    }
