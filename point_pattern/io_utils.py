"""Input parsing and output writing for the point_pattern CLI."""

from __future__ import annotations

import csv
import json
from pathlib import Path

from .core import PatternError, PointPattern, RadiusGrid


def load_points_csv(path: Path) -> list[tuple[float, float]]:
    """Read a CSV with a header row containing x and y columns."""
    with path.open("r", encoding="utf-8", newline="") as handle:
        reader = csv.DictReader(handle)
        if reader.fieldnames is None:
            raise PatternError(f"{path}: empty CSV file")
        fields = {name.strip().lower() for name in reader.fieldnames}
        if not {"x", "y"} <= fields:
            raise PatternError(f"{path}: CSV must have 'x' and 'y' columns")
        points: list[tuple[float, float]] = []
        for row_number, row in enumerate(reader, start=2):
            try:
                x = float(row["x"])
                y = float(row["y"])
            except (TypeError, ValueError) as exc:
                raise PatternError(
                    f"{path}: row {row_number} has non-numeric coordinates"
                ) from exc
            points.append((x, y))
    return points


def load_config(path: Path) -> dict:
    with path.open("r", encoding="utf-8") as handle:
        try:
            config = json.load(handle)
        except json.JSONDecodeError as exc:
            raise PatternError(f"{path}: invalid JSON: {exc}") from exc
    if not isinstance(config, dict):
        raise PatternError(f"{path}: config must be a JSON object")
    return config


def build_pattern_and_grid(
    config: dict, points: list[tuple[float, float]]
) -> tuple[PointPattern, RadiusGrid, int, int]:
    try:
        width = float(config["width"])
        height = float(config["height"])
    except (KeyError, TypeError, ValueError) as exc:
        raise PatternError("config requires numeric 'width' and 'height'") from exc
    pattern = PointPattern(width=width, height=height, points=tuple(points))

    if "radii" in config:
        raw = config["radii"]
        if not isinstance(raw, list):
            raise PatternError("'radii' must be a list of numbers")
        try:
            radii = tuple(float(r) for r in raw)
        except (TypeError, ValueError) as exc:
            raise PatternError("'radii' must contain only numbers") from exc
        grid = RadiusGrid(radii=radii, max_radius=min(width, height) / 2.0)
    else:
        count = config.get("n_radii", 50)
        if isinstance(count, bool) or not isinstance(count, int):
            raise PatternError("'n_radii' must be an integer")
        grid = RadiusGrid.uniform(width, height, count)

    n_sim = config.get("simulations", 199)
    if isinstance(n_sim, bool) or not isinstance(n_sim, int) or n_sim < 1:
        raise PatternError("'simulations' must be a positive integer")
    seed = config.get("seed", 12345)
    if isinstance(seed, bool) or not isinstance(seed, int):
        raise PatternError("'seed' must be an integer")
    return pattern, grid, n_sim, seed


def write_curves_csv(
    path: Path,
    radii: tuple[float, ...],
    counts: list[float],
    k_uncorrected: list[float],
    k_corrected: list[float],
    l_values: list[float],
) -> None:
    with path.open("w", encoding="utf-8", newline="") as handle:
        writer = csv.writer(handle)
        writer.writerow(
            [
                "radius",
                "ordered_pair_count",
                "k_uncorrected",
                "k_corrected",
                "l_corrected",
                "l_minus_r",
            ]
        )
        for r, count, ku, kc, lv in zip(
            radii, counts, k_uncorrected, k_corrected, l_values
        ):
            writer.writerow([r, int(count), ku, kc, lv, lv - r])


def write_test_json(path: Path, result: dict, radii: tuple[float, ...]) -> None:
    payload = {
        "description": (
            "Monte Carlo test of complete spatial randomness (i.i.d. uniform "
            "points in the same rectangle). Statistic: max over the radius "
            "grid of |L(r) - r|. P-value uses the plus-one correction; ties "
            "count toward the upper tail. The envelope is the pointwise "
            "min/max of simulated L curves and is NOT a simultaneous "
            "confidence band."
        ),
        "n_simulations": result["n_simulations"],
        "seed": result["seed"],
        "statistic_max_abs_l_minus_r": result["statistic"],
        "p_value_upper_tail": result["p_value"],
        "exceedances": result["exceedances"],
        "radii": list(radii),
        "envelope_min_l": result["envelope_min"],
        "envelope_max_l": result["envelope_max"],
    }
    with path.open("w", encoding="utf-8") as handle:
        json.dump(payload, handle, indent=2)
        handle.write("\n")
