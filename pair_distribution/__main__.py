"""CLI entry: python -m pair_distribution config.json"""

import sys

from .analysis import analyze_pair, check_r_max
from .output import write_csv, write_summary
from .trajectory import ConfigError, load_config, load_trajectory


def main(argv=None):
    argv = sys.argv[1:] if argv is None else argv
    if len(argv) != 1:
        print("usage: python -m pair_distribution CONFIG.json", file=sys.stderr)
        return 2
    try:
        cfg = load_config(argv[0])
        frames = load_trajectory(cfg["trajectory"])
        r_max_limit = check_r_max(frames, cfg["r_max"])
        results = [
            analyze_pair(frames, pair, cfg["n_bins"], cfg["r_max"])
            for pair in cfg["pairs"]
        ]
        write_csv(cfg["output_csv"], results, cfg["n_bins"], cfg["r_max"])
        write_summary(cfg["summary_json"], cfg, frames, results, r_max_limit)
    except ConfigError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    print(
        f"wrote {cfg['output_csv']} and {cfg['summary_json']} "
        f"({len(frames)} frames, {len(cfg['pairs'])} pairs)"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
