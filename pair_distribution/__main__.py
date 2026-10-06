"""Command line entry: python -m pair_distribution CONFIG.json"""

from __future__ import annotations

import sys

from .config import load_config
from .errors import PairDistributionError
from .output import write_csv, write_summary
from .rdf import check_r_max, compute_pair
from .trajectory import load_trajectory


def main(argv: list[str] | None = None) -> int:
    args = list(sys.argv[1:] if argv is None else argv)
    if len(args) != 1:
        print("usage: python -m pair_distribution CONFIG.json", file=sys.stderr)
        return 2
    try:
        config = load_config(args[0])
        frames = load_trajectory(config.trajectory)
        r_max_limit = check_r_max(frames, config.r_max)
        results = [
            compute_pair(frames, pair, config.n_bins, config.dr, config.r_max)
            for pair in config.pairs
        ]
        write_csv(config.output_csv, config, results)
        write_summary(config.summary_json, config, frames, results, r_max_limit)
    except PairDistributionError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    except OSError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    print(f"wrote {config.output_csv} and {config.summary_json}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
