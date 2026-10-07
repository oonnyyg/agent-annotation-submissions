"""Command line interface.

Usage:
    python -m gated_particle_tracker CONFIG FRAMES [-o OUTPUT]

CONFIG and FRAMES are JSON files (see README.md for the schema).
The tracking result is written as JSON to OUTPUT or stdout.
"""

from __future__ import annotations

import argparse
import json
import sys

from .config import Config, ConfigError
from .frames import parse_frames
from .tracker import run_tracking


def _load_json(path):
    try:
        with open(path, "r", encoding="utf-8") as handle:
            return json.load(handle)
    except OSError as exc:
        raise ConfigError(f"cannot read {path}: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise ConfigError(f"invalid JSON in {path}: {exc}") from exc


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="gated_particle_tracker",
        description="Link 2D particle observations into trajectories.",
    )
    parser.add_argument("config", help="path to the configuration JSON file")
    parser.add_argument("frames", help="path to the frame sequence JSON file")
    parser.add_argument(
        "-o", "--output", help="output JSON path (default: stdout)"
    )
    args = parser.parse_args(argv)

    try:
        config = Config.from_dict(_load_json(args.config))
        frames = parse_frames(
            _load_json(args.frames), config.max_observations_per_frame
        )
        result = run_tracking(frames, config)
    except ConfigError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    text = json.dumps(result, indent=2, sort_keys=False)
    if args.output:
        with open(args.output, "w", encoding="utf-8") as handle:
            handle.write(text + "\n")
    else:
        print(text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
