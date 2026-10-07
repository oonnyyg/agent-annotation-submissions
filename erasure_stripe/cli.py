"""Command line interface: encode / check / recover."""

import argparse
import json
import sys

from . import storage


def _cmd_encode(args):
    manifest = storage.encode_file(
        args.input,
        args.output_dir,
        k=args.data_shards,
        m=args.parity_shards,
        stripe_size=args.stripe_size,
    )
    print(
        f"encoded {manifest['original_length']} bytes into "
        f"{len(manifest['stripes'])} stripe(s), "
        f"{args.data_shards}+{args.parity_shards} shards per stripe"
    )
    return 0


def _cmd_check(args):
    _, report = storage.check_directory(args.directory)
    if args.json:
        print(json.dumps(report, indent=2))
    else:
        for stripe in report["stripes"]:
            status = "ok" if not stripe["invalid"] else "degraded"
            if not stripe["recoverable"]:
                status = "UNRECOVERABLE"
            print(
                f"stripe {stripe['index']}: {status}; valid {stripe['valid']}"
                + (f"; invalid {stripe['invalid']}" if stripe["invalid"] else "")
            )
        print("overall: " + ("ok" if report["ok"] else "problems found"))
    return 0 if report["ok"] else 1


def _cmd_recover(args):
    manifest = storage.recover_file(args.directory, args.output)
    print(
        f"recovered {manifest['original_length']} bytes to {args.output}; "
        f"sha256 {manifest['original_sha256']} verified"
    )
    return 0


def build_parser():
    parser = argparse.ArgumentParser(
        prog="erasure-stripe",
        description="Local stripe backup tool (systematic Reed-Solomon over GF(256)).",
    )
    sub = parser.add_subparsers(dest="command", required=True)

    enc = sub.add_parser("encode", help="encode a file into shards + manifest")
    enc.add_argument("input")
    enc.add_argument("output_dir")
    enc.add_argument("-k", "--data-shards", type=int, default=4)
    enc.add_argument("-m", "--parity-shards", type=int, default=2)
    enc.add_argument("--stripe-size", type=int, default=storage.DEFAULT_STRIPE_SIZE)
    enc.set_defaults(func=_cmd_encode)

    chk = sub.add_parser("check", help="verify shard files against the manifest")
    chk.add_argument("directory")
    chk.add_argument("--json", action="store_true")
    chk.set_defaults(func=_cmd_check)

    rec = sub.add_parser("recover", help="rebuild the original file")
    rec.add_argument("directory")
    rec.add_argument("output")
    rec.set_defaults(func=_cmd_recover)
    return parser


def main(argv=None):
    args = build_parser().parse_args(argv)
    try:
        return args.func(args)
    except (storage.CheckError, ValueError, OSError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
