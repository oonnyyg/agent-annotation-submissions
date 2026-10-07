"""Command-line interface: replay a JSON-defined spiking network."""

from __future__ import annotations

import argparse
import csv
import json
import sys

from .config import ConfigError, load_config
from .simulator import simulate


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="dsn", description="Delayed LIF spiking network playback simulator"
    )
    parser.add_argument("config", help="path to the JSON network configuration")
    parser.add_argument("--voltages", help="write per-step post-reset voltages to this CSV")
    parser.add_argument("--summary", help="write spike summary JSON to this path")
    args = parser.parse_args(argv)

    try:
        config = load_config(args.config)
    except (ConfigError, KeyError, OSError, ValueError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    result = simulate(config)

    if args.voltages:
        with open(args.voltages, "w", newline="", encoding="utf-8") as fh:
            writer = csv.writer(fh)
            writer.writerow(["step", "time"] + [n.name for n in result.neurons])
            for t in range(result.steps):
                writer.writerow(
                    [t, t * result.dt] + [n.voltages[t] for n in result.neurons]
                )

    summary = result.summary()
    text = json.dumps({"dt": result.dt, "steps": result.steps, "neurons": summary}, indent=2)
    if args.summary:
        with open(args.summary, "w", encoding="utf-8") as fh:
            fh.write(text + "\n")
    print(text)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
