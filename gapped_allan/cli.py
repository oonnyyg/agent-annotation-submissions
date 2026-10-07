"""Command line interface for gapped overlapping Allan analysis."""

from __future__ import annotations

import argparse
import sys

from .core import AnalysisError, build_segments, detrend_ols, overlapping_allan
from .io import load_config, read_csv, write_reports


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="gapped-allan",
        description=(
            "Overlapping Allan deviation of dimensionless fractional "
            "frequency data with sampling gaps handled as independent "
            "segments (no interpolation, no cross-gap averaging)."
        ),
    )
    parser.add_argument("--data", required=True, help="input CSV file")
    parser.add_argument("--config", required=True, help="JSON analysis config")
    parser.add_argument("--out-csv", required=True, help="output CSV report")
    parser.add_argument("--out-json", required=True, help="output JSON report")
    return parser


def run(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        cfg = load_config(args.config)
        times, values = read_csv(args.data, cfg)
        segments = build_segments(times, values, cfg.tau0)
        raw_results = overlapping_allan(segments, cfg.m_values, cfg.tau0)

        report: dict = {
            "input": args.data,
            "config": args.config,
            "tau0_s": cfg.tau0,
            "samples": len(times),
            "segments": [
                {
                    "start_time_s": seg.times[0],
                    "end_time_s": seg.times[-1],
                    "samples": seg.size,
                }
                for seg in segments
            ],
            "units": {
                "time": "seconds",
                "fractional_frequency": "dimensionless",
                "allan_dev": "dimensionless",
            },
            "detrended": cfg.detrend,
            "raw": [_result_dict(r) for r in raw_results],
        }

        if cfg.detrend:
            slope, intercept, residuals = detrend_ols(times, values)
            dt_segments = build_segments(times, residuals, cfg.tau0)
            dt_results = overlapping_allan(dt_segments, cfg.m_values, cfg.tau0)
            report["trend_fit"] = {
                "method": "ordinary_least_squares",
                "slope_per_s": slope,
                "intercept": intercept,
            }
            report["detrended_results"] = [_result_dict(r) for r in dt_results]

        write_reports(args.out_csv, args.out_json, report, raw_results)
    except AnalysisError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    return 0


def _result_dict(r) -> dict:
    return {
        "m": r.m,
        "tau_s": r.tau,
        "allan_dev": r.adev,
        "allan_var": r.avar,
        "pairs": r.pairs,
        "status": r.status,
    }


def main() -> None:
    sys.exit(run())


if __name__ == "__main__":
    main()
