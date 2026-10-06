"""JSON and CSV input/output for the layered soil storage tool."""

from __future__ import annotations

import csv
import io as _io
import json
import math

from .model import (
    Layer,
    invert_water_table,
    layer_index_at,
    profile_storage,
    theta_at_depth,
)


def _finite_number(value, name: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValueError(f"{name} must be a number")
    if not math.isfinite(value):
        raise ValueError(f"{name} must be finite")
    return float(value)


def layers_from_config(config: dict) -> list:
    raw = config.get("layers")
    if not isinstance(raw, list) or not raw:
        raise ValueError("config requires a non-empty 'layers' list")
    layers = []
    for i, item in enumerate(raw):
        if not isinstance(item, dict):
            raise ValueError(f"layers[{i}] must be an object")
        layers.append(Layer(
            thickness=_finite_number(item.get("thickness"), f"layers[{i}].thickness"),
            theta_r=_finite_number(item.get("theta_r"), f"layers[{i}].theta_r"),
            theta_s=_finite_number(item.get("theta_s"), f"layers[{i}].theta_s"),
            alpha=_finite_number(item.get("alpha"), f"layers[{i}].alpha"),
            n=_finite_number(item.get("n"), f"layers[{i}].n"),
        ))
    return layers


def load_config(text: str) -> dict:
    config = json.loads(text)
    if not isinstance(config, dict):
        raise ValueError("config must be a JSON object")
    layers_from_config(config)  # validate early
    tol = config.get("tolerance", 1e-9)
    _finite_number(tol, "tolerance")
    if tol <= 0:
        raise ValueError("tolerance must be positive")
    return config


def _storage_dict(result) -> dict:
    return {
        "water_table_cm": result.water_table,
        "total_storage_cm": result.total,
        "tolerance": result.tolerance,
        "converged": result.converged,
        "layers": [
            {
                "layer_index": p.layer_index,
                "top_cm": p.top,
                "bottom_cm": p.bottom,
                "storage_cm": p.storage,
            }
            for p in result.layers
        ],
    }


def _inversion_dict(res) -> dict:
    return {
        "solved": res.solved,
        "water_table_cm": res.water_table,
        "target_storage_cm": res.target,
        "achieved_storage_cm": res.achieved,
        "residual_cm": res.residual,
        "achievable_min_cm": res.min_storage,
        "achievable_max_cm": res.max_storage,
        "message": res.message,
    }


def run_config(config: dict) -> dict:
    """Execute every request embedded in a JSON config and build a report."""
    layers = layers_from_config(config)
    tol = config.get("tolerance", 1e-9)
    report = {"layers": [
        {
            "layer_index": i,
            "thickness_cm": layer.thickness,
            "theta_r": layer.theta_r,
            "theta_s": layer.theta_s,
            "alpha_per_cm": layer.alpha,
            "n": layer.n,
            "m": layer.m,
        }
        for i, layer in enumerate(layers)
    ]}
    wt = config.get("water_table")
    if wt is not None:
        wt = _finite_number(wt, "water_table")
        report["storage"] = _storage_dict(profile_storage(layers, wt, tol))
    samples = config.get("samples")
    if samples:
        if wt is None:
            raise ValueError("'samples' requires 'water_table'")
        depths = sorted({_finite_number(d, "sample") for d in samples})
        report["samples"] = [
            {
                "depth_cm": d,
                "layer_index": layer_index_at(layers, d),
                "theta": theta_at_depth(layers, wt, d),
            }
            for d in depths
        ]
    inv = config.get("invert")
    if inv is not None:
        target = _finite_number(inv.get("target"), "invert.target")
        max_depth = _finite_number(inv.get("max_depth"), "invert.max_depth")
        report["inversion"] = _inversion_dict(
            invert_water_table(layers, target, max_depth, tol))
    return report


def dumps_report(report: dict) -> str:
    return json.dumps(report, indent=2, ensure_ascii=False) + "\n"


# ---------------------------------------------------------------- CSV batch

def run_csv_batch(config: dict, csv_text: str) -> str:
    """Process a CSV of batch requests and return a result CSV string.

    Input columns: ``kind,depth_cm`` where kind is ``water_table`` or
    ``sample``. Water table rows request a full storage computation;
    sample rows request point water contents evaluated for every water
    table in the batch. Input order does not affect results.
    """
    layers = layers_from_config(config)
    tol = config.get("tolerance", 1e-9)
    total_depth = sum(layer.thickness for layer in layers)
    reader = csv.DictReader(_io.StringIO(csv_text))
    if reader.fieldnames is None or not {"kind", "depth_cm"} <= set(reader.fieldnames):
        raise ValueError("CSV requires columns: kind,depth_cm")
    water_tables = set()
    samples = set()
    for row in reader:
        kind = (row.get("kind") or "").strip().lower()
        try:
            depth = float((row.get("depth_cm") or "").strip())
        except ValueError:
            raise ValueError(f"invalid depth_cm value: {row.get('depth_cm')!r}")
        if not math.isfinite(depth):
            raise ValueError("depth_cm must be finite")
        if kind == "water_table":
            if depth < 0:
                raise ValueError("water table depth must be non-negative")
            water_tables.add(depth)
        elif kind == "sample":
            if not (0.0 <= depth <= total_depth):
                raise ValueError(
                    f"sample depth {depth} outside profile [0, {total_depth}]")
            samples.add(depth)
        else:
            raise ValueError(f"unknown kind: {kind!r}")
    water_tables = sorted(water_tables)
    samples = sorted(samples)

    out = _io.StringIO()
    writer = csv.writer(out, lineterminator="\n")
    writer.writerow(["request", "water_table_cm", "depth_cm",
                     "layer_index", "value"])
    storages = {}
    for wt in water_tables:
        result = profile_storage(layers, wt, tol)
        storages[wt] = result
        writer.writerow(["total", f"{wt:g}", "", "", f"{result.total:.12g}"])
        for part in result.layers:
            writer.writerow(["layer", f"{wt:g}", "", part.layer_index,
                             f"{part.storage:.12g}"])
    for wt in water_tables:
        for depth in samples:
            idx = layer_index_at(layers, depth)
            theta = theta_at_depth(layers, wt, depth)
            writer.writerow(["sample", f"{wt:g}", f"{depth:g}", idx,
                             f"{theta:.12g}"])
    return out.getvalue()
