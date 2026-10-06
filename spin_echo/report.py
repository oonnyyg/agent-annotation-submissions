"""CSV and JSON summary output."""

from __future__ import annotations

import csv
import math

from .simulate import peak_in_window, total_signal


def write_csv(path, experiment, results):
    spins = experiment.spins
    header = ["time_s"]
    for i in range(len(spins)):
        header += [f"group{i}_mx", f"group{i}_my", f"group{i}_mz"]
    header += ["total_magnitude", "total_phase_rad"]
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(header)
        for t, states in results:
            row = [f"{t:.12g}"]
            for mx, my, mz in states:
                row += [f"{mx:.12g}", f"{my:.12g}", f"{mz:.12g}"]
            mag, phase = total_signal(states, spins)
            row.append(f"{mag:.12g}")
            row.append("" if phase is None else f"{phase:.12g}")
            writer.writerow(row)


def build_summary(experiment, results):
    spins = experiment.spins
    peak_time, peak_mag = peak_in_window(results, spins, experiment.peak_window)
    lo, hi = experiment.peak_window
    window = [
        None if math.isinf(lo) else lo,
        None if math.isinf(hi) else hi,
    ]
    return {
        "spin_groups": len(spins),
        "total_weight": sum(s.weight for s in spins),
        "sample_count": len(results),
        "pulse_count": len(experiment.pulses),
        "peak_window_s": window,
        "peak": {
            "time_s": peak_time,
            "magnitude": peak_mag,
        },
    }
