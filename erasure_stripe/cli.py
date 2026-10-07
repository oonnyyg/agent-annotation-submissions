"""Command line interface: encode, check, restore."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from .codec import MAX_DATA_SHARDS, MAX_TOTAL_SHARDS, MIN_DATA_SHARDS
from .storage import (
    ManifestError,
    RestoreError,
    check_manifest,
    encode_file,
    restore_file,
)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="erasure-stripe",
        description="Local striped backup with Reed-Solomon erasure coding over GF(256).",
    )
    sub = parser.add_subparsers(dest="command", required=True)

    enc = sub.add_parser("encode", help="encode a file into shards + manifest.json")
    enc.add_argument("source", type=Path)
    enc.add_argument("out_dir", type=Path)
    enc.add_argument("-k", "--data-shards", type=int, default=4,
                     help=f"data shards per stripe ({MIN_DATA_SHARDS}..{MAX_DATA_SHARDS})")
    enc.add_argument("-p", "--parity-shards", type=int, default=2,
                     help=f"parity shards per stripe (total <= {MAX_TOTAL_SHARDS})")
    enc.add_argument("--stripe-size", type=int, default=1 << 16,
                     help="bytes per stripe (default 65536)")

    chk = sub.add_parser("check", help="verify shard files against the manifest")
    chk.add_argument("manifest", type=Path)

    res = sub.add_parser("restore", help="rebuild the original file from shards")
    res.add_argument("manifest", type=Path)
    res.add_argument("target", type=Path)
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        if args.command == "encode":
            path = encode_file(args.source, args.out_dir, args.data_shards,
                               args.parity_shards, args.stripe_size)
            print(f"encoded -> {path}")
        elif args.command == "check":
            return _cmd_check(args.manifest)
        elif args.command == "restore":
            restore_file(args.manifest, args.target)
            print(f"restored -> {args.target}")
    except (ManifestError, RestoreError, ValueError, OSError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    return 0


def _cmd_check(manifest: Path) -> int:
    reports = check_manifest(manifest)
    exit_code = 0
    for report in reports:
        bad = [s for s in report.shards if not s.ok]
        if bad:
            exit_code = 1
            detail = ", ".join(f"shard {s.index} ({s.reason})" for s in bad)
            print(f"stripe {report.index}: UNUSABLE {detail}")
        else:
            print(f"stripe {report.index}: ok ({len(report.shards)} shards)")
    return exit_code


if __name__ == "__main__":
    sys.exit(main())
