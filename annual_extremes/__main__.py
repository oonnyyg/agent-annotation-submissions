"""Command line entry point: python -m annual_extremes <config.json>."""

import json
import sys

from .analysis import run_analysis


def main(argv=None):
    args = list(sys.argv[1:] if argv is None else argv)
    if len(args) != 1:
        print("usage: python -m annual_extremes <config.json>", file=sys.stderr)
        return 2
    with open(args[0], "r", encoding="utf-8") as fh:
        config = json.load(fh)
    try:
        result = run_analysis(config)
    except ValueError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    print(json.dumps(result["summary"], indent=2, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main())
