"""Command line interface for gyroscope attitude replay."""

from __future__ import annotations

import argparse
import csv
import json
import math
import sys
from pathlib import Path

from .core import (
    AttitudeReplay,
    Quaternion,
    load_bias_json,
    load_gyro_csv,
    load_reference_csv,
)


def _parse_floats(text: str, count: int, what: str) -> list[float]:
    try:
        values = [float(part) for part in text.split(",")]
    except ValueError:
        raise argparse.ArgumentTypeError(f"{what} must be comma-separated numbers")
    if len(values) != count:
        raise argparse.ArgumentTypeError(f"{what} needs exactly {count} numbers")
    if not all(math.isfinite(v) for v in values):
        raise argparse.ArgumentTypeError(f"{what} must be finite")
    return values


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="gyro_replay",
        description="Replay gyroscope records into 3D attitude (Hamilton, "
                    "scalar-first, body-to-world).",
    )
    p.add_argument("gyro_csv", help="CSV: time,wx,wy,wz (rad/s, body frame)")
    p.add_argument("--q0", required=True,
                   help="initial quaternion 'w,x,y,z' (body to world)")
    p.add_argument("--bias-json", help="JSON file with fixed body bias")
    p.add_argument("--time", type=float,
                   help="query time in seconds (must be inside the record)")
    p.add_argument("--vector",
                   help="body-frame vector 'x,y,z' to rotate into world frame")
    p.add_argument("--reference-csv",
                   help="CSV: time,qw,qx,qy,qz reference attitudes")
    p.add_argument("--attitude-out",
                   help="write attitude CSV (time,qw,qx,qy,qz) at gyro times")
    p.add_argument("--errors-out",
                   help="write JSON error summary vs the reference CSV")
    return p


def _error_summary(replay: AttitudeReplay,
                   reference: list[tuple[float, Quaternion]]) -> dict:
    samples = []
    for t, q_ref in reference:
        err = replay.error_against(t, q_ref)
        samples.append({"time": t, "error_rad": err})
    errors = [s["error_rad"] for s in samples]
    n = len(errors)
    return {
        "count": n,
        "max_rad": max(errors),
        "mean_rad": sum(errors) / n,
        "rms_rad": math.sqrt(sum(e * e for e in errors) / n),
        "samples": samples,
    }


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        rows = load_gyro_csv(args.gyro_csv)
        q0 = Quaternion.from_sequence(_parse_floats(args.q0, 4, "--q0"))
        bias = load_bias_json(args.bias_json) if args.bias_json else (0.0, 0.0, 0.0)
        replay = AttitudeReplay(rows, q0, bias)

        if args.time is not None:
            q = replay.attitude(args.time)
            print(f"attitude(t={args.time}) = "
                  f"({q.w:.12g}, {q.x:.12g}, {q.y:.12g}, {q.z:.12g})")
            if args.vector:
                vec = _parse_floats(args.vector, 3, "--vector")
                wx, wy, wz = replay.rotate_to_world(args.time, vec)
                print(f"world vector = ({wx:.12g}, {wy:.12g}, {wz:.12g})")
        elif args.vector:
            print("warning: --vector ignored without --time", file=sys.stderr)

        if args.attitude_out:
            with open(args.attitude_out, "w", newline="", encoding="utf-8") as fh:
                writer = csv.writer(fh)
                writer.writerow(["time", "qw", "qx", "qy", "qz"])
                for t, q in replay.iter_attitudes():
                    writer.writerow([f"{t:.12g}", f"{q.w:.12g}", f"{q.x:.12g}",
                                     f"{q.y:.12g}", f"{q.z:.12g}"])
            print(f"wrote attitudes to {args.attitude_out}")

        if args.reference_csv:
            reference = load_reference_csv(args.reference_csv)
            if not reference:
                raise ValueError("reference CSV is empty")
            summary = _error_summary(replay, reference)
            text = json.dumps(summary, indent=2)
            if args.errors_out:
                Path(args.errors_out).write_text(text + "\n", encoding="utf-8")
                print(f"wrote error summary to {args.errors_out}")
            else:
                print(text)
        elif args.errors_out:
            print("warning: --errors-out ignored without --reference-csv",
                  file=sys.stderr)
    except (ValueError, OSError, argparse.ArgumentTypeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
