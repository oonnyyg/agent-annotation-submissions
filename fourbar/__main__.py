"""Command line entry point: python -m fourbar input.json -o out_prefix

Input JSON schema:
{
  "links": {"r1": 4.0, "r2": 1.0, "r3": 3.0, "r4": 2.5},
  "branch": 1,                      // +1 or -1, assembly branch sign
  "angles": [0.0, 0.1, ...],        // input angles, radians
  "omegas": [1.0, 1.0, ...],        // input angular velocities, rad/s
  "probe": {"u": 1.5, "v": 0.5}     // optional coupler-local probe point
}

Outputs:
  <prefix>.csv            one row per input angle
  <prefix>.diagnostics.json  run summary and per-angle diagnostics
"""

from __future__ import annotations

import argparse
import csv
import json
import sys

from .core import FourBar, STATUS_OK, solve_configuration

CSV_FIELDS = [
    "theta", "omega2", "status",
    "Bx", "By", "Cx", "Cy",
    "vBx", "vBy", "vCx", "vCy",
    "omega3", "omega4",
    "probe_x", "probe_y", "probe_vx", "probe_vy",
    "len_res_coupler", "len_res_output",
    "vel_res_coupler", "vel_res_output",
    "message",
]


def _load_input(path: str) -> dict:
    with open(path, "r", encoding="utf-8-sig") as fh:
        data = json.load(fh)
    for key in ("links", "angles", "omegas"):
        if key not in data:
            raise ValueError(f"input JSON missing required key {key!r}")
    if len(data["angles"]) != len(data["omegas"]):
        raise ValueError("angles and omegas must have the same length")
    return data


def _num(value: float | None) -> str:
    if value is None:
        return ""
    return repr(float(value))


def _row(result) -> dict:
    def pt(p, i):
        return _num(p[i]) if p is not None else ""

    return {
        "theta": _num(result.theta),
        "omega2": _num(result.omega2),
        "status": result.status,
        "Bx": pt(result.B, 0), "By": pt(result.B, 1),
        "Cx": pt(result.C, 0), "Cy": pt(result.C, 1),
        "vBx": pt(result.vB, 0), "vBy": pt(result.vB, 1),
        "vCx": pt(result.vC, 0), "vCy": pt(result.vC, 1),
        "omega3": _num(result.omega3),
        "omega4": _num(result.omega4),
        "probe_x": pt(result.probe, 0), "probe_y": pt(result.probe, 1),
        "probe_vx": pt(result.probe_velocity, 0),
        "probe_vy": pt(result.probe_velocity, 1),
        "len_res_coupler": pt(result.length_residuals, 0),
        "len_res_output": pt(result.length_residuals, 1),
        "vel_res_coupler": pt(result.velocity_residuals, 0),
        "vel_res_output": pt(result.velocity_residuals, 1),
        "message": result.message,
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="fourbar", description=__doc__.splitlines()[0])
    parser.add_argument("input", help="input JSON file")
    parser.add_argument("-o", "--output", default="fourbar_out",
                        help="output file prefix (default: fourbar_out)")
    args = parser.parse_args(argv)

    data = _load_input(args.input)
    links = data["links"]
    link = FourBar(r1=links["r1"], r2=links["r2"], r3=links["r3"], r4=links["r4"])
    branch = int(data.get("branch", 1))
    probe_data = data.get("probe")
    probe = (float(probe_data["u"]), float(probe_data["v"])) if probe_data else None

    results = [
        solve_configuration(link, float(theta), float(omega), branch, probe)
        for theta, omega in zip(data["angles"], data["omegas"])
    ]

    csv_path = args.output + ".csv"
    with open(csv_path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(fh, fieldnames=CSV_FIELDS)
        writer.writeheader()
        for result in results:
            writer.writerow(_row(result))

    counts: dict[str, int] = {}
    for result in results:
        counts[result.status] = counts.get(result.status, 0) + 1
    diagnostics = {
        "links": {"r1": link.r1, "r2": link.r2, "r3": link.r3, "r4": link.r4},
        "branch": branch,
        "units": {"angle": "rad", "angular_velocity": "rad/s"},
        "n_angles": len(results),
        "status_counts": counts,
        "all_ok": all(r.status == STATUS_OK for r in results),
        "max_abs_length_residual": max(
            (max(abs(r.length_residuals[0]), abs(r.length_residuals[1]))
             for r in results if r.length_residuals is not None),
            default=None,
        ),
        "max_abs_velocity_residual": max(
            (max(abs(r.velocity_residuals[0]), abs(r.velocity_residuals[1]))
             for r in results if r.velocity_residuals is not None),
            default=None,
        ),
        "issues": [
            {"index": i, "theta": r.theta, "status": r.status, "message": r.message}
            for i, r in enumerate(results) if r.status != STATUS_OK
        ],
    }
    json_path = args.output + ".diagnostics.json"
    with open(json_path, "w", encoding="utf-8") as fh:
        json.dump(diagnostics, fh, indent=2)
        fh.write("\n")

    print(f"wrote {csv_path} and {json_path}")
    if not diagnostics["all_ok"]:
        print(f"warning: {len(diagnostics['issues'])} angle(s) not OK "
              f"({counts})", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
