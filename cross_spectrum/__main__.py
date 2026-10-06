"""Command line interface: python -m cross_spectrum <csv> <config> [options]."""

from __future__ import annotations

import argparse
import csv
import json
import sys

from .core import (
    ConfigError,
    DataError,
    compute_cross_spectrum,
    load_config,
    read_csv_channels,
)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="python -m cross_spectrum",
        description=(
            "Welch-averaged cross-spectrum analysis of a two-channel CSV. "
            "Finds shared frequency components and their phase relation."
        ),
    )
    parser.add_argument("csv_path", help="input CSV with two sample columns")
    parser.add_argument("config_path", help="JSON configuration file")
    parser.add_argument(
        "--spectrum",
        default="spectrum.csv",
        help="output spectrum CSV path (default: spectrum.csv)",
    )
    parser.add_argument(
        "--diagnostics",
        default="diagnostics.json",
        help="output diagnostics JSON path (default: diagnostics.json)",
    )
    return parser


def write_spectrum_csv(path: str, result) -> None:
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow(
            [
                "frequency_hz",
                "psd_ch1",
                "psd_ch2",
                "cross_real",
                "cross_imag",
                "coherence",
                "phase_rad",
            ]
        )
        for k, freq in enumerate(result.frequencies):
            coh = result.coherence[k]
            ph = result.phase[k]
            writer.writerow(
                [
                    f"{freq:.12g}",
                    f"{result.psd1[k]:.12g}",
                    f"{result.psd2[k]:.12g}",
                    f"{result.cross[k].real:.12g}",
                    f"{result.cross[k].imag:.12g}",
                    "" if coh is None else f"{coh:.12g}",
                    "" if ph is None else f"{ph:.12g}",
                ]
            )


def write_diagnostics_json(path: str, result, config) -> None:
    stats = result.stats
    doc = {
        "config": {
            "sample_rate": config.sample_rate,
            "segment_length": config.segment_length,
            "overlap": config.overlap,
            "nfft": config.nfft,
            "window": config.window,
        },
        "segments": {
            "candidates": stats.total_candidates,
            "used": stats.used,
            "skipped_missing": stats.skipped_missing,
            "skipped_missing_starts": stats.skipped_missing_starts,
        },
        "window_power": result.window_power,
        "energy_check": result.energy_check,
    }
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(doc, fh, indent=2)
        fh.write("\n")


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        config = load_config(args.config_path)
        ch1, ch2 = read_csv_channels(args.csv_path)
        result = compute_cross_spectrum(ch1, ch2, config)
    except (ConfigError, DataError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    write_spectrum_csv(args.spectrum, result)
    write_diagnostics_json(args.diagnostics, result, config)
    stats = result.stats
    print(
        f"segments: {stats.used} used, {stats.skipped_missing} skipped "
        f"(missing data) out of {stats.total_candidates} candidates"
    )
    print(f"spectrum written to {args.spectrum}")
    print(f"diagnostics written to {args.diagnostics}")
    if not result.energy_check["consistent"]:
        print(
            "warning: spectral integral does not match windowed segment "
            "energy; check inputs",
            file=sys.stderr,
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
