"""Command line interface: JSON cameras + CSV matches -> JSON quality report."""

from __future__ import annotations

import argparse
import csv
import json
import math
import sys

from .geometry import Camera, ReconstructionError, reconstruct_point


def load_cameras(path) -> tuple[Camera, Camera]:
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    cams = []
    for key in ("left", "right"):
        spec = data[key]
        vals = [spec["fx"], spec["fy"], spec["cx"], spec["cy"], *spec["center"]]
        vals += [x for row in spec["rotation"] for x in row]
        vals += [spec.get("k1", 0.0), spec.get("k2", 0.0)]
        if not all(math.isfinite(x) for x in vals):
            raise ValueError(f"camera '{key}' contains non-finite values")
        cams.append(
            Camera(
                fx=spec["fx"],
                fy=spec["fy"],
                cx=spec["cx"],
                cy=spec["cy"],
                center=tuple(spec["center"]),
                rotation=tuple(tuple(r) for r in spec["rotation"]),
                k1=spec.get("k1", 0.0),
                k2=spec.get("k2", 0.0),
            )
        )
    return cams[0], cams[1]


def load_matches(path):
    with open(path, "r", encoding="utf-8", newline="") as fh:
        reader = csv.reader(fh)
        rows = []
        seen = set()
        for lineno, row in enumerate(reader, start=1):
            if not row or row[0].strip().startswith("#"):
                continue
            if lineno == 1 and row[0].strip().lower() in ("id", "point_id"):
                continue  # optional header
            if len(row) != 5:
                raise ValueError(f"line {lineno}: expected 5 columns, got {len(row)}")
            pid = row[0].strip()
            if pid in seen:
                raise ValueError(f"line {lineno}: duplicate point id '{pid}'")
            seen.add(pid)
            coords = [float(x) for x in row[1:]]
            if not all(math.isfinite(x) for x in coords):
                raise ValueError(f"line {lineno}: non-finite coordinate")
            rows.append((pid, (coords[0], coords[1]), (coords[2], coords[3])))
    return rows


def build_report(cam1, cam2, matches, min_angle_deg):
    points, failures = [], []
    for pid, px1, px2 in matches:
        try:
            result = reconstruct_point(cam1, cam2, px1, px2, min_angle_deg)
        except ReconstructionError as exc:
            failures.append({"id": pid, "reason": str(exc)})
            continue
        points.append({"id": pid, **result})
    distances = [p["ray_distance"] for p in points]
    residuals = [abs(r) for p in points for pair in p["residuals_px"] for r in pair]
    return {
        "summary": {
            "total": len(matches),
            "reconstructed": len(points),
            "failed": len(failures),
            "max_ray_distance": max(distances) if distances else None,
            "mean_ray_distance": (
                sum(distances) / len(distances) if distances else None
            ),
            "max_abs_residual_px": max(residuals) if residuals else None,
        },
        "points": points,
        "failures": failures,
    }


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(
        prog="stereo-ray",
        description="Reconstruct 3D points from matched stereo pixels.",
    )
    parser.add_argument("--cameras", required=True, help="JSON camera file")
    parser.add_argument("--matches", required=True, help="CSV matched pixels")
    parser.add_argument("--output", help="write JSON report here (default: stdout)")
    parser.add_argument(
        "--min-angle-deg",
        type=float,
        default=1.0,
        help="reject ray pairs closer to parallel than this (0, 90); default 1.0",
    )
    args = parser.parse_args(argv)

    try:
        cam1, cam2 = load_cameras(args.cameras)
        matches = load_matches(args.matches)
        report = build_report(cam1, cam2, matches, args.min_angle_deg)
    except (ValueError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    text = json.dumps(report, indent=2)
    if args.output:
        with open(args.output, "w", encoding="utf-8") as fh:
            fh.write(text + "\n")
    else:
        print(text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
