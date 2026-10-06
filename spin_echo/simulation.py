"""Sequence runner: interleaves free evolution, pulses, and sampling."""

from __future__ import annotations

import cmath

from .physics import apply_pulse, free_evolve


def simulate(config):
    """Run the sequence; return one record per sample time.

    Pulses at the same time apply in input order; sampling at a time
    happens after all pulses at that time. Sample times are processed
    in ascending order, so query order in the config cannot affect
    the evolution.
    """
    spins = config.spins
    total_weight = sum(s.weight for s in spins)
    pulses = sorted(config.pulses, key=lambda p: (p.time, p.order))

    states = [s.m0 for s in spins]
    now = 0.0
    pidx = 0
    records = []

    for t in config.samples:
        while pidx < len(pulses) and pulses[pidx].time <= t:
            pt = pulses[pidx].time
            states = [
                free_evolve(m, s.offset_hz, s.t1, s.t2, s.mz_eq, pt - now)
                for m, s in zip(states, spins)
            ]
            now = pt
            while pidx < len(pulses) and pulses[pidx].time == pt:
                p = pulses[pidx]
                states = [apply_pulse(m, p.phase, p.flip) for m in states]
                pidx += 1
        sampled = [
            free_evolve(m, s.offset_hz, s.t1, s.t2, s.mz_eq, t - now)
            for m, s in zip(states, spins)
        ]
        signal = complex(0.0, 0.0)
        for m, s in zip(sampled, spins):
            signal += (s.weight / total_weight) * complex(m[0], m[1])
        amp = abs(signal)
        records.append(
            {
                "time": t,
                "groups": [
                    (s.name, m[0], m[1], m[2]) for s, m in zip(spins, sampled)
                ],
                "amplitude": amp,
                "phase": cmath.phase(signal) if amp > 0.0 else None,
            }
        )
    return records


def summarize(config, records):
    """Peak total-signal amplitude inside the configured window."""
    start, end = config.window
    best = None
    for rec in records:
        t = rec["time"]
        if start <= t <= end:
            if best is None or rec["amplitude"] > best["amplitude"]:
                best = {
                    "time": t,
                    "amplitude": rec["amplitude"],
                    "phase": rec["phase"],
                }
    return {
        "window": [start, end],
        "peak": best,
        "sample_count": len(records),
        "group_count": len(config.spins),
        "pulse_count": len(config.pulses),
    }
