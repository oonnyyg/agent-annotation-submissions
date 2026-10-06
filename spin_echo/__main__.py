"""CLI: python -m spin_echo CONFIG.json [--csv OUT.csv] [--summary OUT.json]"""

from __future__ import annotations

import argparse
import csv
import json
import math
import sys

from .config import ConfigError, load_config
from .simulation import simulate, summarize


def _fmt(value):
    if value is None:
        return ""
    if isinstance(value, float):
        return repr(value)
    return str(value)


def write_csv(records, fileobj):
    writer = csv.writer(fileobj)
    writer.writerow(["time_s", "group", "mx", "my", "mz",
                     "signal_amplitude", "signal_phase_rad"])
    for rec in records:
        for name, mx, my, mz in rec["groups"]:
            writer.writerow([_fmt(rec["time"]), name, _fmt(mx), _fmt(my),
                             _fmt(mz), "", ""])
        writer.writerow([_fmt(rec["time"]), "<total>", "", "", "",
                         _fmt(rec["amplitude"]), _fmt(rec["phase"])])


def _jsonable(summary):
    def fix(v):
        if isinstance(v, float) and math.isinf(v):
            return "Infinity" if v > 0 else "-Infinity"
        if isinstance(v, list):
            return [fix(x) for x in v]
        if isinstance(v, dict):
            return {k: fix(x) for k, x in v.items()}
        return v
    return fix(summary)


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="python -m spin_echo",
        description="Local spin echo sequence simulator (no hardware).",
    )
    parser.add_argument("config", help="path to JSON configuration file")
    parser.add_argument("--csv", default="spin_echo_output.csv",
                        help="CSV output path (default: spin_echo_output.csv)")
    parser.add_argument("--summary", default="spin_echo_summary.json",
                        help="JSON summary path (default: spin_echo_summary.json)")
    args = parser.parse_args(argv)

    try:
        with open(args.config, "r", encoding="utf-8") as fh:
            config = load_config(fh.read())
    except OSError as exc:
        print(f"error: cannot read config: {exc}", file=sys.stderr)
        return 2
    except ConfigError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    records = simulate(config)
    summary = summarize(config, records)

    with open(args.csv, "w", encoding="utf-8", newline="") as fh:
        write_csv(records, fh)
    with open(args.summary, "w", encoding="utf-8") as fh:
        json.dump(_jsonable(summary), fh, indent=2)
        fh.write("\n")

    peak = summary["peak"]
    if peak is None:
        print("no samples inside the report window")
    else:
        print(f"peak amplitude {peak['amplitude']:.6g} "
              f"at t={peak['time']:.6g}s (earliest)")
    print(f"wrote {args.csv} and {args.summary}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
