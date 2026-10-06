"""Reading and validating CSV coordinates and JSON configuration."""

from __future__ import annotations

import csv
import json

from .core import Window, validate_radii


def read_points_csv(path):
    """Read x,y coordinates from a CSV file.

    Accepts an optional header row (first non-numeric row is skipped).
    Each data row must have exactly two numeric columns: x then y.
    """
    points = []
    with open(path, newline="", encoding="utf-8-sig") as fh:
        for lineno, row in enumerate(csv.reader(fh), start=1):
            if not row or all(not cell.strip() for cell in row):
                continue
            if len(row) != 2:
                raise ValueError(
                    f"{path}:{lineno}: expected exactly 2 columns (x,y)"
                )
            try:
                x = float(row[0])
                y = float(row[1])
            except ValueError:
                if not points and lineno == 1:
                    continue  # header row
                raise ValueError(
                    f"{path}:{lineno}: non-numeric coordinate"
                ) from None
            points.append((x, y))
    return points


def read_config(path):
    """Read the JSON configuration and build window/radii/sim settings.

    Required keys: ``width``, ``height``, ``simulations`` (positive int),
    ``seed`` (int).  Radius grid is given either explicitly via ``radii``
    (strictly increasing, starting at 0) or via ``n_radii`` (evenly spaced
    from 0 to half the shorter side).
    """
    with open(path, encoding="utf-8-sig") as fh:
        cfg = json.load(fh)
    window = Window(float(cfg["width"]), float(cfg["height"]))
    if "radii" in cfg:
        radii = validate_radii(cfg["radii"], window)
    elif "n_radii" in cfg:
        n = int(cfg["n_radii"])
        if n < 2:
            raise ValueError("n_radii must be at least 2")
        step = window.max_radius / (n - 1)
        radii = validate_radii([i * step for i in range(n)], window)
    else:
        raise ValueError("config must provide 'radii' or 'n_radii'")
    simulations = int(cfg["simulations"])
    if simulations < 1:
        raise ValueError("simulations must be a positive integer")
    seed = int(cfg["seed"])
    return {
        "window": window,
        "radii": radii,
        "simulations": simulations,
        "seed": seed,
    }
