"""Command line entry: python -m enclosure_radiation config.json -o outdir"""

from __future__ import annotations

import argparse
import sys

from .model import load_config, ConfigError
from .solver import solve_case
from .report import write_surfaces_csv, write_exchange_json


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m enclosure_radiation",
        description="Gray diffuse enclosure radiation calculator")
    parser.add_argument("config", help="JSON configuration file")
    parser.add_argument("--surfaces-csv", default="surfaces.csv",
                        help="output CSV of per-surface results")
    parser.add_argument("--exchange-json", default="exchange.json",
                        help="output JSON of pairwise exchange and balances")
    args = parser.parse_args(argv)

    try:
        enc = load_config(args.config)
        results = [solve_case(enc, case) for case in enc.cases]
    except (ConfigError, OSError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    write_surfaces_csv(results, args.surfaces_csv)
    write_exchange_json(enc, results, args.exchange_json)
    for res in results:
        print(f"case {res.name}: enclosure residual "
              f"{res.energy_residual:.6g} W")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
