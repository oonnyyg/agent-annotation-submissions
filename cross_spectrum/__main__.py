"""Command line entry point: python -m cross_spectrum CONFIG.json"""

from __future__ import annotations

import sys

from .config import ConfigError, load_config
from .core import AnalysisError, analyze
from .data import DataError, load_samples
from .output import assert_finite, write_diagnostics_json, write_spectrum_csv


def main(argv: list[str] | None = None) -> int:
    args = sys.argv[1:] if argv is None else argv
    if len(args) != 1:
        print("usage: python -m cross_spectrum CONFIG.json", file=sys.stderr)
        return 2
    try:
        config = load_config(args[0])
        samples = load_samples(
            config.csv_path,
            config.index_column,
            config.channel1_column,
            config.channel2_column,
        )
        result = analyze(samples, config)
        assert_finite(result)
        write_spectrum_csv(config.spectrum_output, result)
        write_diagnostics_json(config.diagnostics_output, result, config)
    except (ConfigError, DataError, AnalysisError, OSError, ValueError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    print(
        f"valid segments: {result.valid_segments}, "
        f"skipped: {len(result.skipped)}; "
        f"wrote {config.spectrum_output} and {config.diagnostics_output}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
