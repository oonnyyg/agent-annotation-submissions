"""Spectrum CSV and diagnostics JSON writers."""

from __future__ import annotations

import csv
import json
import math

from .core import AnalysisResult


def _fmt(value: float | None) -> str:
    return "" if value is None else f"{value:.10g}"


def write_spectrum_csv(path: str, result: AnalysisResult) -> None:
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow([
            "frequency_hz",
            "psd_ch1",
            "psd_ch2",
            "cross_real",
            "cross_imag",
            "coherence",
            "phase_rad",
        ])
        for k, freq in enumerate(result.frequencies):
            writer.writerow([
                f"{freq:.10g}",
                f"{result.psd1[k]:.10g}",
                f"{result.psd2[k]:.10g}",
                f"{result.cross[k].real:.10g}",
                f"{result.cross[k].imag:.10g}",
                _fmt(result.coherence[k]),
                _fmt(result.phase[k]),
            ])


def write_diagnostics_json(path: str, result: AnalysisResult, config) -> None:
    payload = {
        "sample_rate_hz": config.sample_rate,
        "segment_length": config.segment_length,
        "overlap": config.overlap,
        "nfft": config.nfft,
        "window": config.window,
        "valid_segments": result.valid_segments,
        "skipped_segments": [
            {"start_index": s.start_index, "reason": s.reason} for s in result.skipped
        ],
        "energy_check": result.energy_check,
        "notes": {
            "phase_convention": (
                "phase = arg(conj(FFT(ch1)) * FFT(ch2)); positive means ch2 leads ch1"
            ),
            "psd_units": "signal_unit^2/Hz, one-sided",
        },
    }
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(payload, fh, indent=2, allow_nan=False)
        fh.write("\n")


def assert_finite(result: AnalysisResult) -> None:
    for series in (result.psd1, result.psd2):
        for value in series:
            if not math.isfinite(value):
                raise ValueError("non-finite spectral value encountered")
