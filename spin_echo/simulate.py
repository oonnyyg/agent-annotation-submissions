"""Exact Bloch evolution with instantaneous RF pulses."""

from __future__ import annotations

import math

from .config import Experiment


def _free_evolve(state, offset_hz, t1, t2, meq, dt):
    """Advance one spin group by dt seconds (exact, no step-size dependence)."""
    mx, my, mz = state
    if dt <= 0.0:
        return state
    angle = 2.0 * math.pi * offset_hz * dt  # right-hand rotation about +z
    cos_a, sin_a = math.cos(angle), math.sin(angle)
    decay = 1.0 if math.isinf(t2) else math.exp(-dt / t2)
    mx2 = (mx * cos_a - my * sin_a) * decay
    my2 = (mx * sin_a + my * cos_a) * decay
    if math.isinf(t1):
        mz2 = mz
    else:
        mz2 = meq + (mz - meq) * math.exp(-dt / t1)
    return (mx2, my2, mz2)


def _apply_pulse(state, phase_rad, flip_rad):
    """Instantaneous right-hand rotation about axis (cos ph, sin ph, 0)."""
    mx, my, mz = state
    ux, uy = math.cos(phase_rad), math.sin(phase_rad)
    cos_a, sin_a = math.cos(flip_rad), math.sin(flip_rad)
    dot = ux * mx + uy * my
    # Rodrigues' rotation formula with u_z = 0.
    rx = mx * cos_a + (-uy * mz) * sin_a + ux * dot * (1.0 - cos_a)
    ry = my * cos_a + (ux * mz) * sin_a + uy * dot * (1.0 - cos_a)
    rz = mz * cos_a + (uy * mx - ux * my) * sin_a
    return (rx, ry, rz)


def run(experiment: Experiment):
    """Simulate and return [(time, [(mx, my, mz), ...]), ...] per sample time."""
    exp = experiment
    states = [s.initial for s in exp.spins]
    # Events keyed by time; pulses keep input order (stable grouping), and
    # samples at a time are recorded after every pulse at that time.
    events: dict[float, dict] = {}
    for t in exp.sample_times:
        events.setdefault(t, {"pulses": [], "sample": True})["sample"] = True
    for pulse in exp.pulses:
        events.setdefault(pulse.time, {"pulses": [], "sample": False})[
            "pulses"
        ].append(pulse)
    now = 0.0
    results = []
    for t in sorted(events):
        dt = t - now
        if dt > 0.0:
            states = [
                _free_evolve(st, s.offset_hz, s.t1, s.t2, s.equilibrium_mz, dt)
                for st, s in zip(states, exp.spins)
            ]
            now = t
        event = events[t]
        for pulse in event["pulses"]:
            states = [
                _apply_pulse(st, pulse.phase_rad, pulse.flip_rad) for st in states
            ]
        if event["sample"]:
            results.append((t, list(states)))
    return results


def total_signal(states, spins):
    """Weight-normalised transverse sum; returns (magnitude, phase_rad | None)."""
    total_weight = sum(s.weight for s in spins)
    mx = sum(s.weight * st[0] for s, st in zip(spins, states)) / total_weight
    my = sum(s.weight * st[1] for s, st in zip(spins, states)) / total_weight
    magnitude = math.hypot(mx, my)
    if magnitude == 0.0:
        return 0.0, None
    return magnitude, math.atan2(my, mx)


def peak_in_window(results, spins, window):
    """Largest total magnitude within [lo, hi]; earliest time wins ties."""
    lo, hi = window
    best_time = None
    best_mag = -1.0
    for t, states in results:
        if t < lo or t > hi:
            continue
        mag, _ = total_signal(states, spins)
        if mag > best_mag:
            best_mag = mag
            best_time = t
    return best_time, (best_mag if best_time is not None else None)
