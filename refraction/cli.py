"""Command-line interface: python -m refraction <input.json>.

Input JSON (all units metres and seconds):
    {
      "model": {"layers": [{"velocity": ..., "thickness": ...}, ...]},
      "offsets": [0.0, 50.0, ...],
      "range": [x_min, x_max],          // optional branch analysis
      "observed": [[offset, time], ...] // optional residual report
    }
"""
from __future__ import annotations

import argparse
import csv
import json
import sys

from .branches import branch_intervals
from .model import ModelError, VelocityModel
from .observed import residual_report
from .traveltime import all_paths, first_arrivals


def _path_label(path) -> str:
    if path.kind == "direct":
        return "direct"
    return f"refracted_interface_{path.interface}"


def _path_json(path, x: float) -> dict:
    segments = [
        {
            "layer": s.layer,
            "role": s.role,
            "length_m": s.length,
            "horizontal_m": s.horizontal,
            "time_s": s.time,
        }
        for s in path.segments(x)
    ]
    return {
        "path": _path_label(path),
        "kind": path.kind,
        "interface": path.interface,
        "time_s": path.time(x),
        "min_offset_m": path.min_offset,
        "segments": segments,
        "segment_time_sum_s": sum(s["time_s"] for s in segments),
    }


def write_csv(model: VelocityModel, offsets: list[float], handle) -> None:
    writer = csv.writer(handle)
    writer.writerow(
        [
            "offset_m",
            "path",
            "interface",
            "reachable",
            "is_first",
            "time_s",
            "layer",
            "role",
            "length_m",
            "horizontal_m",
            "segment_time_s",
        ]
    )
    paths = all_paths(model)
    for x in offsets:
        _, winners = first_arrivals(model, x)
        winner_ids = {id(p) for p in winners}
        for path in paths:
            label = _path_label(path)
            interface = path.interface if path.interface is not None else ""
            is_first = any(
                p.kind == path.kind and p.interface == path.interface
                for p in winners
            )
            if not path.reachable(x):
                writer.writerow(
                    [x, label, interface, "no", "no", "", "", "", "", "", ""]
                )
                continue
            first = "yes" if is_first else "no"
            for seg in path.segments(x):
                writer.writerow(
                    [
                        x,
                        label,
                        interface,
                        "yes",
                        first,
                        f"{path.time(x):.9g}",
                        seg.layer,
                        seg.role,
                        f"{seg.length:.9g}",
                        f"{seg.horizontal:.9g}",
                        f"{seg.time:.9g}",
                    ]
                )


def build_analysis(
    model: VelocityModel,
    offsets: list[float],
    offset_range: tuple[float, float] | None,
    observed: list[tuple[float, float]] | None,
) -> dict:
    arrivals = []
    for x in offsets:
        best, winners = first_arrivals(model, x)
        arrivals.append(
            {
                "offset_m": x,
                "first_arrival_s": best,
                "tied_paths": [_path_json(p, x) for p in winners],
            }
        )
    result = {
        "units": {"distance": "m", "time": "s"},
        "layers": [
            {"velocity_m_per_s": l.velocity, "thickness_m": l.thickness}
            if l.thickness != float("inf")
            else {"velocity_m_per_s": l.velocity, "thickness_m": "half-space"}
            for l in model.layers
        ],
        "arrivals": arrivals,
    }
    if offset_range is not None:
        intervals, never_first = branch_intervals(model, *offset_range)
        result["branches"] = {
            "range_m": list(offset_range),
            "intervals": [
                {
                    "start_m": iv.start,
                    "end_m": iv.end,
                    "paths": [_path_label(p) for p in iv.paths],
                }
                for iv in intervals
            ],
            "switch_points_m": [iv.end for iv in intervals[:-1]],
            "never_first_interfaces": [
                p.interface for p in never_first
            ],
        }
    if observed is not None:
        report = residual_report(model, observed)
        result["residuals"] = {
            "count": len(report.residuals),
            "rms_s": report.rms,
            "mean_s": report.mean,
            "max_abs_s": report.max_abs,
            "picks": [
                {
                    "offset_m": r.offset,
                    "observed_s": r.observed,
                    "modelled_s": r.modelled,
                    "residual_s": r.residual,
                }
                for r in report.residuals
            ],
        }
    return result


def _load_input(path: str) -> dict:
    with open(path, "r", encoding="utf-8") as handle:
        data = json.load(handle)
    if not isinstance(data, dict):
        raise ModelError("input JSON must be an object")
    return data


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m refraction",
        description="First-arrival travel-time analyser for horizontal "
        "layered models (metres and seconds).",
    )
    parser.add_argument("input", help="input JSON file")
    parser.add_argument("--csv", default="paths.csv", help="CSV output path")
    parser.add_argument(
        "--json", default="analysis.json", help="JSON analysis output path"
    )
    args = parser.parse_args(argv)

    try:
        data = _load_input(args.input)
        model = VelocityModel.from_dict(data.get("model"))
        offsets = [float(x) for x in data.get("offsets", [])]
        if any(x < 0 for x in offsets):
            raise ModelError("offsets must be non-negative")
        offset_range = None
        if "range" in data and data["range"] is not None:
            lo, hi = data["range"]
            offset_range = (float(lo), float(hi))
            if not 0.0 <= offset_range[0] < offset_range[1]:
                raise ModelError("range must satisfy 0 <= min < max")
        observed = None
        if "observed" in data and data["observed"] is not None:
            observed = [
                (float(x), float(t)) for x, t in data["observed"]
            ]
    except (ModelError, ValueError, TypeError, KeyError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    with open(args.csv, "w", newline="", encoding="utf-8") as handle:
        write_csv(model, offsets, handle)
    analysis = build_analysis(model, offsets, offset_range, observed)
    with open(args.json, "w", encoding="utf-8") as handle:
        json.dump(analysis, handle, indent=2)
        handle.write("\n")
    print(f"wrote {args.csv} and {args.json}")
    return 0
