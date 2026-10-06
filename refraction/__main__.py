"""Command line interface: python -m refraction <input.json>.

Input JSON (all units metres and seconds):
  {
    "layers": [{"velocity": 500, "thickness": 10}, {"velocity": 2000}],
    "offsets": [0, 25, 50],
    "range": {"min": 0, "max": 500},          // optional, for dominance
    "observed": [{"offset": 25, "time": 0.05}] // optional, residuals
  }
"""

from __future__ import annotations

import argparse
import csv
import json
import math
import sys

from .analysis import dominance_intervals, first_arrivals
from .model import VelocityModel
from .paths import compute_paths


def _path_name(path) -> str:
    if path.kind == "direct":
        return "direct"
    return f"refracted_layer_{path.refractor}"


def _write_csv(csv_path: str, paths, arrivals) -> None:
    with open(csv_path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(
            ["offset_m", "path", "reachable", "is_first_arrival",
             "layer", "horizontal_m", "vertical_m", "time_s", "total_time_s"]
        )
        for arrival in arrivals:
            first_set = {id(p) for p in arrival.paths}
            for path in paths:
                reachable = path.reachable(arrival.offset)
                is_first = reachable and id(path) in first_set
                if not reachable:
                    writer.writerow(
                        [arrival.offset, _path_name(path), "no", "no",
                         "", "", "", "", ""]
                    )
                    continue
                total = path.travel_time(arrival.offset)
                for seg in path.segments_at(arrival.offset):
                    writer.writerow(
                        [f"{arrival.offset:.6f}", _path_name(path), "yes",
                         "yes" if is_first else "no", seg.layer,
                         f"{seg.horizontal:.6f}", f"{seg.vertical:.6f}",
                         f"{seg.time:.9f}", f"{total:.9f}"]
                    )


def _build_analysis(model, paths, arrivals, dom, observed) -> dict:
    stations = []
    for arrival in arrivals:
        stations.append(
            {
                "offset_m": arrival.offset,
                "first_arrival_time_s": arrival.time,
                "paths": [
                    {
                        "path": _path_name(p),
                        "refractor_layer": p.refractor,
                        "critical_distance_m": p.critical_distance,
                        "segments": [
                            {
                                "layer": s.layer,
                                "horizontal_m": s.horizontal,
                                "vertical_m": s.vertical,
                                "time_s": s.time,
                            }
                            for s in p.segments_at(arrival.offset)
                        ],
                    }
                    for p in arrival.paths
                ],
            }
        )
    result = {
        "velocities_m_per_s": model.velocities,
        "stations": stations,
    }
    if dom is not None:
        result["dominance"] = dom
    if observed:
        residuals = []
        sq = 0.0
        for pick in observed:
            offset = float(pick["offset"])
            obs_time = float(pick["time"])
            match = first_arrivals([offset], paths)[0]
            residual = obs_time - match.time
            residuals.append(
                {"offset_m": offset, "observed_s": obs_time,
                 "model_s": match.time, "residual_s": residual}
            )
            sq += residual * residual
        result["residuals"] = {
            "picks": residuals,
            "rms_s": math.sqrt(sq / len(residuals)),
        }
    return result


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m refraction",
        description="First-arrival refraction analysis (metres and seconds).",
    )
    parser.add_argument("input", help="input JSON file")
    parser.add_argument("--csv", default="paths.csv", help="path detail CSV")
    parser.add_argument("--json", default="analysis.json", help="analysis JSON")
    args = parser.parse_args(argv)

    with open(args.input, encoding="utf-8-sig") as fh:
        data = json.load(fh)
    try:
        model = VelocityModel.from_dict(data)
    except ValueError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    offsets = [float(x) for x in data.get("offsets", [])]
    paths = compute_paths(model)
    try:
        arrivals = first_arrivals(offsets, paths)
    except ValueError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    dom = None
    rng = data.get("range")
    if rng is not None:
        dom = dominance_intervals(paths, float(rng["min"]), float(rng["max"]))

    _write_csv(args.csv, paths, arrivals)
    analysis = _build_analysis(model, paths, arrivals, dom, data.get("observed"))
    with open(args.json, "w", encoding="utf-8") as fh:
        json.dump(analysis, fh, indent=2)
    print(f"wrote {args.csv} and {args.json}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
