"""Command line: replay a gyro log to attitude CSV and error summary."""

from __future__ import annotations

import argparse
import csv
import json
import math
import sys

from .integrator import GyroReplay
from .io import load_bias_json, load_gyro_csv, load_reference_csv
from .quaternion import Quaternion


def _parse_q0(text):
    parts = [float(p) for p in text.split(",")]
    if len(parts) != 4:
        raise argparse.ArgumentTypeError("q0 must be 'w,x,y,z'")
    return Quaternion(*parts).normalized()


def build_parser():
    p = argparse.ArgumentParser(
        prog="gyro-replay",
        description="Replay body-rate gyro CSV to 3D attitude (Hamilton, scalar-first).",
    )
    p.add_argument("gyro_csv", help="CSV: t,wx,wy,wz (rad/s, strictly increasing t)")
    p.add_argument("--bias-json", help="JSON fixed body bias {wx,wy,wz} or [wx,wy,wz]")
    p.add_argument("--q0", type=_parse_q0, default=None,
                   help="initial attitude 'w,x,y,z' (default: identity)")
    p.add_argument("--out", help="write attitude CSV: t,qw,qx,qy,qz (default: stdout)")
    p.add_argument("--reference-csv", help="CSV: t,qw,qx,qy,qz to compare against")
    p.add_argument("--error-json", help="write error summary JSON to this path")
    p.add_argument("--vector", help="body vector 'x,y,z' to rotate at each sample")
    return p


def main(argv=None):
    args = build_parser().parse_args(argv)
    times, rates = load_gyro_csv(args.gyro_csv)
    bias = load_bias_json(args.bias_json) if args.bias_json else (0.0, 0.0, 0.0)
    replay = GyroReplay(times, rates, bias=bias, q0=args.q0)

    vector = None
    if args.vector:
        vector = tuple(float(c) for c in args.vector.split(","))
        if len(vector) != 3:
            raise SystemExit("--vector must be 'x,y,z'")

    out_fh = open(args.out, "w", newline="", encoding="utf-8") if args.out else sys.stdout
    try:
        writer = csv.writer(out_fh)
        header = ["t", "qw", "qx", "qy", "qz"]
        if vector:
            header += ["vx_world", "vy_world", "vz_world"]
        writer.writerow(header)
        for t in times:
            q = replay.attitude(t)
            row = [f"{t:.9g}", f"{q.w:.12g}", f"{q.x:.12g}", f"{q.y:.12g}", f"{q.z:.12g}"]
            if vector:
                row += [f"{c:.12g}" for c in replay.rotate_to_world(t, vector)]
            writer.writerow(row)
    finally:
        if args.out:
            out_fh.close()

    if args.reference_csv:
        ref = load_reference_csv(args.reference_csv)
        errors = []
        for t, q_ref in sorted(ref.items()):
            if not (replay.t_start <= t <= replay.t_end):
                continue
            err = replay.attitude(t).attitude_error(q_ref)
            errors.append({"t": t, "angle_error_rad": err})
        if errors:
            vals = [e["angle_error_rad"] for e in errors]
            summary = {
                "count": len(vals),
                "max_rad": max(vals),
                "mean_rad": math.fsum(vals) / len(vals),
                "rms_rad": math.sqrt(math.fsum(v * v for v in vals) / len(vals)),
                "errors": errors,
            }
        else:
            summary = {"count": 0, "errors": []}
        text = json.dumps(summary, indent=2)
        if args.error_json:
            with open(args.error_json, "w", encoding="utf-8") as fh:
                fh.write(text + "\n")
        else:
            print(text, file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
