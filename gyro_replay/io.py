"""Load gyro CSV logs, bias JSON, and reference attitude CSV."""

from __future__ import annotations

import csv
import json
import math

from .quaternion import Quaternion


def _finite(value, what):
    try:
        x = float(value)
    except ValueError:
        raise ValueError(f"{what}: not a number: {value!r}") from None
    if not math.isfinite(x):
        raise ValueError(f"{what}: value must be finite, got {x!r}")
    return x


def load_gyro_csv(path):
    """Read (t, wx, wy, wz) rows; times strictly increasing, >= 2 rows.

    Row i's rate applies over [t_i, t_{i+1}); the last row only fixes the
    end time. Returns (times, rates) with len(rates) == len(times) - 1.
    """
    times = []
    rates = []
    with open(path, newline="", encoding="utf-8") as fh:
        reader = csv.reader(fh)
        rows = [r for r in reader if r and not r[0].lstrip().startswith("#")]
    if rows and rows[0][0].strip().lower() in ("t", "time", "time_s"):
        rows = rows[1:]
    if len(rows) < 2:
        raise ValueError("gyro CSV needs at least two rows")
    for i, row in enumerate(rows):
        if len(row) != 4:
            raise ValueError(f"row {i + 1}: expected 4 columns (t,wx,wy,wz)")
        t = _finite(row[0], f"row {i + 1} time")
        if times and t <= times[-1]:
            raise ValueError(f"row {i + 1}: times must be strictly increasing")
        times.append(t)
        rates.append(tuple(_finite(c, f"row {i + 1} rate") for c in row[1:4]))
    rates.pop()  # last row only determines the end time
    return times, rates


def load_bias_json(path):
    """Read fixed body-frame bias {"wx","wy","wz"} (or a 3-list), rad/s."""
    with open(path, encoding="utf-8") as fh:
        data = json.load(fh)
    if isinstance(data, dict):
        vals = (data["wx"], data["wy"], data["wz"])
    elif isinstance(data, (list, tuple)) and len(data) == 3:
        vals = data
    else:
        raise ValueError("bias JSON must be {wx,wy,wz} or a 3-element list")
    return tuple(_finite(str(v), "bias") for v in vals)


def load_reference_csv(path):
    """Read (t, qw, qx, qy, qz) reference attitudes keyed by time."""
    ref = {}
    with open(path, newline="", encoding="utf-8") as fh:
        reader = csv.reader(fh)
        rows = [r for r in reader if r and not r[0].lstrip().startswith("#")]
    if rows and rows[0][0].strip().lower() in ("t", "time", "time_s"):
        rows = rows[1:]
    for i, row in enumerate(rows):
        if len(row) != 5:
            raise ValueError(f"reference row {i + 1}: expected 5 columns")
        t = _finite(row[0], f"reference row {i + 1} time")
        q = Quaternion(*(_finite(c, f"reference row {i + 1}") for c in row[1:5]))
        ref[t] = q.normalized()
    return ref
