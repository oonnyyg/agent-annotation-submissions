"""CLI: python -m fourbar input.json --csv out.csv --diag diag.json

Input JSON (all angles in radians, lengths in any consistent unit):
{
  "ground": 4.0, "crank": 1.0, "coupler": 3.0, "rocker": 2.5,
  "branch": 1,                      // +1 or -1, sign of cross(A->O2, A->B)
  "angles": [0.0, 0.1, ...],        // input angles, rad, CCW
  "omega": 1.0,                     // constant input angular velocity, rad/s
  "omegas": [1.0, ...],             // optional per-angle omega, overrides omega
  "probe": {"u": 1.5, "v": 0.2}     // optional coupler-local probe point
}
"""

from __future__ import annotations

import argparse
import csv
import json
import math
import sys

from .core import Linkage, analyze


def _fmt(x: float | None) -> str:
    return "" if x is None else f"{x:.12g}"


def load_spec(path: str) -> dict:
    with open(path, "r", encoding="utf-8") as fh:
        spec = json.load(fh)
    for key in ("ground", "crank", "coupler", "rocker", "angles"):
        if key not in spec:
            raise SystemExit(f"missing required key: {key}")
    return spec


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="fourbar", description=__doc__)
    parser.add_argument("input", help="input JSON file")
    parser.add_argument("--csv", default="fourbar.csv", help="output CSV path")
    parser.add_argument("--diag", default="fourbar_diag.json", help="diagnostics JSON path")
    args = parser.parse_args(argv)

    spec = load_spec(args.input)
    link = Linkage(
        ground=float(spec["ground"]),
        crank=float(spec["crank"]),
        coupler=float(spec["coupler"]),
        rocker=float(spec["rocker"]),
    )
    branch = int(spec.get("branch", 1))
    angles = [float(t) for t in spec["angles"]]
    omegas = spec.get("omegas")
    if omegas is None:
        omegas = [float(spec.get("omega", 1.0))] * len(angles)
    if len(omegas) != len(angles):
        raise SystemExit("omegas length must match angles length")
    probe = None
    if "probe" in spec:
        probe = (float(spec["probe"]["u"]), float(spec["probe"]["v"]))

    results = [
        analyze(link, t, float(w), branch, probe)
        for t, w in zip(angles, omegas)
    ]

    with open(args.csv, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow([
            "theta", "omega_in", "status", "branch",
            "ax", "ay", "bx", "by",
            "omega_coupler", "omega_rocker",
            "bvx", "bvy", "px", "py", "pvx", "pvy",
        ])
        for r in results:
            a = r.a or (None, None)
            b = r.b or (None, None)
            bv = r.b_vel or (None, None)
            pp = r.probe_pos or (None, None)
            pv = r.probe_vel or (None, None)
            writer.writerow([
                _fmt(r.theta), _fmt(r.omega_in), r.status, r.branch,
                _fmt(a[0]), _fmt(a[1]), _fmt(b[0]), _fmt(b[1]),
                _fmt(r.omega_coupler), _fmt(r.omega_rocker),
                _fmt(bv[0]), _fmt(bv[1]),
                _fmt(pp[0]), _fmt(pp[1]), _fmt(pv[0]), _fmt(pv[1]),
            ])

    diag = {
        "units": {"angle": "rad", "angular_velocity": "rad/s"},
        "branch": branch,
        "linkage": {
            "ground": link.ground, "crank": link.crank,
            "coupler": link.coupler, "rocker": link.rocker,
        },
        "summary": {
            "total": len(results),
            "ok": sum(1 for r in results if r.status == "ok"),
            "singular": sum(1 for r in results if r.status == "singular"),
            "unreachable": sum(1 for r in results if r.status == "unreachable"),
            "ambiguous": sum(1 for r in results if r.status == "ambiguous"),
        },
        "entries": [
            {
                "theta": r.theta,
                "status": r.status,
                "detail": r.detail,
                "length_residuals": r.length_residuals,
                "velocity_residuals": r.velocity_residuals,
            }
            for r in results
        ],
    }
    with open(args.diag, "w", encoding="utf-8") as fh:
        json.dump(diag, fh, indent=2, ensure_ascii=False)

    bad = [r for r in results if r.status != "ok"]
    for r in bad:
        print(f"theta={r.theta:.6g}: {r.status} {r.detail}", file=sys.stderr)
    print(f"wrote {args.csv} and {args.diag} "
          f"({len(results)} configs, {len(bad)} non-ok)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
