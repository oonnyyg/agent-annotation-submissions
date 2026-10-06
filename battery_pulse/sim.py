"""Event-accurate simulation of piecewise-constant current profiles."""

from __future__ import annotations

import math
from dataclasses import dataclass

from .model import Battery, ModelError


@dataclass
class Segment:
    duration_s: float
    current_a: float


@dataclass
class State:
    t: float
    soc: float
    v_pol: list[float]  # per-branch polarization voltage (V)


@dataclass
class Sample:
    t: float
    side: str  # "left" / "right" at discontinuities, "" otherwise
    current_a: float
    soc: float
    v_oc: float
    v_ohm: float
    v_pol: float
    v_term: float


def terminal_voltage(bat: Battery, soc: float, v_pol: list[float], current: float) -> float:
    return bat.ocv(soc) - current * bat.r0_ohm - sum(v_pol)


def _advance(bat: Battery, state: State, current: float, dt: float) -> None:
    """Analytic constant-current update over dt seconds."""
    state.t += dt
    state.soc -= current * dt / bat.capacity_c
    for k, br in enumerate(bat.branches):
        target = current * br.resistance_ohm
        decay = math.exp(-dt / br.tau)
        state.v_pol[k] = target + (state.v_pol[k] - target) * decay


def _time_to_voltage(bat: Battery, state: State, current: float, v_min: float,
                     horizon: float) -> float | None:
    """Bisect the analytic V(t) within (0, horizon]; None if no crossing."""

    def v_at(dt: float) -> float:
        soc = state.soc - current * dt / bat.capacity_c
        vp = 0.0
        for k, br in enumerate(bat.branches):
            target = current * br.resistance_ohm
            vp += target + (state.v_pol[k] - target) * math.exp(-dt / br.tau)
        return bat.ocv(soc) - current * bat.r0_ohm - vp

    if v_at(horizon) > v_min:
        return None
    lo, hi = 0.0, horizon
    for _ in range(80):
        mid = 0.5 * (lo + hi)
        if v_at(mid) > v_min:
            lo = mid
        else:
            hi = mid
    return hi


def simulate(bat: Battery, segments: list[Segment], v_min: float,
             soc0: float, v_pol0: list[float] | None = None,
             sample_dt: float = 1.0) -> tuple[list[Sample], dict]:
    if not 0.0 <= soc0 <= 1.0:
        raise ModelError(f"initial soc must be in [0, 1], got {soc0}")
    if sample_dt <= 0:
        raise ModelError("sample_dt must be positive")
    state = State(0.0, soc0, list(v_pol0) if v_pol0 is not None
                  else [0.0] * len(bat.branches))
    if len(state.v_pol) != len(bat.branches):
        raise ModelError("initial polarization voltage count must match rc_branches")

    samples: list[Sample] = []
    discharged_ah = 0.0
    stop_reason = "completed"

    def record(side: str, current: float) -> None:
        samples.append(Sample(
            t=state.t, side=side, current_a=current, soc=state.soc,
            v_oc=bat.ocv(state.soc), v_ohm=current * bat.r0_ohm,
            v_pol=sum(state.v_pol),
            v_term=terminal_voltage(bat, state.soc, state.v_pol, current)))

    record("", segments[0].current_a if segments else 0.0)
    if terminal_voltage(bat, state.soc, state.v_pol,
                        segments[0].current_a if segments else 0.0) <= v_min:
        record("right", segments[0].current_a)
        stop_reason = "voltage_cutoff"
        return samples, _report(state, discharged_ah, stop_reason)

    next_sample = sample_dt
    for seg in segments:
        if stop_reason != "completed":
            break
        if seg.duration_s <= 0:
            raise ModelError("segment duration_s must be positive")
        current = seg.current_a
        # Right-side check at the current jump (polarization is continuous).
        v_right = terminal_voltage(bat, state.soc, state.v_pol, current)
        if state.t > 0.0:
            record("right", current)
        if v_right <= v_min:
            stop_reason = "voltage_cutoff"
            break

        # SOC boundary crossing inside the segment (discharge only).
        dt_soc = math.inf
        if current > 0.0:
            dt_soc = state.soc * bat.capacity_c / current

        dt_v = _time_to_voltage(bat, state, current, v_min, seg.duration_s)
        dt_stop = seg.duration_s
        reason = None
        if dt_v is not None:
            dt_stop, reason = dt_v, "voltage_cutoff"
        if dt_soc <= dt_stop:
            dt_stop, reason = dt_soc, "soc_depleted"

        # Emit regular samples strictly inside the segment.
        seg_start = state.t
        while next_sample < seg_start + dt_stop - 1e-12:
            _advance(bat, state, current, next_sample - state.t)
            record("", current)
            next_sample += sample_dt

        _advance(bat, state, current, seg_start + dt_stop - state.t)
        discharged_ah += current * dt_stop / 3600.0
        if reason == "soc_depleted":
            state.soc = 0.0
        record("left", current)
        if reason is not None:
            stop_reason = reason
            break

    return samples, _report(state, discharged_ah, stop_reason)


def _report(state: State, discharged_ah: float, reason: str) -> dict:
    return {
        "stop_reason": reason,
        "stop_time_s": state.t,
        "final_soc": state.soc,
        "net_discharge_ah": discharged_ah,
    }
