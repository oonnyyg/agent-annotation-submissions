"""Command-line interface: read frames + config JSON, write tracking JSON."""

from __future__ import annotations

import argparse
import json
import sys

from .tracker import TrackerConfig, TrackerError, track_frames

CONFIG_KEYS = set(TrackerConfig.__dataclass_fields__)


def load_input(path):
    with open(path, "r", encoding="utf-8-sig") as handle:
        data = json.load(handle)
    if not isinstance(data, dict) or "frames" not in data or "config" not in data:
        raise TrackerError("input must be an object with 'config' and 'frames'")
    raw_config = data["config"]
    unknown = set(raw_config) - CONFIG_KEYS
    if unknown:
        raise TrackerError(f"unknown config keys: {sorted(unknown)}")
    try:
        config = TrackerConfig(**raw_config)
    except TypeError as exc:
        raise TrackerError(f"invalid config: {exc}") from exc
    return config, data["frames"]


def main(argv=None):
    parser = argparse.ArgumentParser(prog="gated-particle-tracker")
    parser.add_argument("input", help="JSON file with 'config' and 'frames'")
    parser.add_argument("-o", "--output", help="output JSON file (default: stdout)")
    args = parser.parse_args(argv)

    try:
        config, frames = load_input(args.input)
        result = track_frames(frames, config)
    except TrackerError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    except (OSError, json.JSONDecodeError) as exc:
        print(f"error: cannot read input: {exc}", file=sys.stderr)
        return 2

    text = json.dumps(result, indent=2)
    if args.output:
        with open(args.output, "w", encoding="utf-8") as handle:
            handle.write(text + "\n")
    else:
        print(text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
