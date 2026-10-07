"""Parallel-beam filtered backprojection (FBP) reconstruction.

Pure standard-library implementation intended for validating the FBP
pipeline on synthetic projection data. No clinical imaging support.

Conventions:
- Projections: CSV, one row per angle. First column is the angle in
  degrees, remaining columns are equally spaced detector readings.
  Angles must uniformly cover [0, 180) degrees (right endpoint excluded).
- Geometry: both the detector array and the image grid are centered on
  the geometric origin. Even-sized grids are NOT shifted by half a
  sample; index i maps to coordinate (i - (n - 1) / 2) * spacing.
- Fourier convention: DFT pair
      X[k] = sum_n x[n] * exp(-2j*pi*k*n/N)
      x[n] = (1/N) * sum_k X[k] * exp(+2j*pi*k*n/N)
  Frequencies are cycles per unit length: f_k = k / (N * spacing) with
  the second half of the spectrum taken as negative frequencies.
- Ramp filter: H(f) = |f|, applied on the spectrum of the projection
  zero-padded to at least twice its length. After the inverse DFT the
  result is cropped back to the original detector positions. No maximum
  normalization is applied.
- Backprojection: pixel centers are projected onto the detector
  coordinate, the filtered projection is linearly interpolated (zero
  outside the detector range), contributions are summed over angles and
  multiplied by the angular step in radians. Negative values are kept.
"""

from __future__ import annotations

import cmath
import csv
import json
import math

MAX_ANGLES = 64
MAX_DETECTORS = 64
MAX_IMAGE_DIM = 64


class ReconstructionError(ValueError):
    """Raised for invalid input data or configuration."""


def load_config(path):
    """Load the JSON configuration (detector spacing and image grid)."""
    try:
        with open(path, "r", encoding="utf-8") as handle:
            config = json.load(handle)
    except json.JSONDecodeError as exc:
        raise ReconstructionError(f"invalid JSON config: {exc}") from exc
    if not isinstance(config, dict):
        raise ReconstructionError("config must be a JSON object")
    try:
        spacing = float(config["detector_spacing"])
        nx = int(config["image"]["nx"])
        ny = int(config["image"]["ny"])
        pixel_size = float(config["image"]["pixel_size"])
    except (KeyError, TypeError, ValueError) as exc:
        raise ReconstructionError(
            "config requires 'detector_spacing' and "
            "'image': {'nx', 'ny', 'pixel_size'}"
        ) from exc
    if spacing <= 0 or pixel_size <= 0:
        raise ReconstructionError("spacings must be positive")
    if nx < 1 or ny < 1:
        raise ReconstructionError("image dimensions must be positive")
    if nx > MAX_IMAGE_DIM or ny > MAX_IMAGE_DIM:
        raise ReconstructionError(
            f"image grid exceeds {MAX_IMAGE_DIM}x{MAX_IMAGE_DIM} limit"
        )
    return {
        "detector_spacing": spacing,
        "nx": nx,
        "ny": ny,
        "pixel_size": pixel_size,
    }


def load_sinogram(path):
    """Load the projection CSV and validate angle coverage.

    Returns (angles_degrees, rows) sorted by angle, where rows are lists
    of detector readings (floats) of equal length.
    """
    angles = []
    rows = []
    with open(path, "r", encoding="utf-8", newline="") as handle:
        for record in csv.reader(handle):
            if not record or all(not cell.strip() for cell in record):
                continue
            try:
                angle = float(record[0])
                readings = [float(cell) for cell in record[1:]]
            except ValueError as exc:
                raise ReconstructionError(
                    f"non-numeric value in projections CSV: {exc}"
                ) from exc
            if not readings:
                raise ReconstructionError("projection row has no readings")
            angles.append(angle)
            rows.append(readings)

    if not angles:
        raise ReconstructionError("no projections found")
    if len(angles) > MAX_ANGLES:
        raise ReconstructionError(
            f"too many angles: {len(angles)} > {MAX_ANGLES}"
        )
    n_det = len(rows[0])
    if n_det > MAX_DETECTORS:
        raise ReconstructionError(
            f"too many detectors: {n_det} > {MAX_DETECTORS}"
        )
    for row in rows:
        if len(row) != n_det:
            raise ReconstructionError("inconsistent detector count per row")

    n_angles = len(angles)
    step = 180.0 / n_angles
    tol = 1e-6 * max(step, 1.0)
    expected = [k * step for k in range(n_angles)]
    used = [False] * n_angles
    order = [None] * n_angles
    for index, angle in enumerate(angles):
        wrapped = angle % 360.0
        if wrapped >= 180.0 - tol:
            raise ReconstructionError(
                f"angle {angle} outside [0, 180) degrees"
            )
        slot = round(wrapped / step)
        if slot >= n_angles or abs(wrapped - expected[slot]) > tol:
            raise ReconstructionError(
                f"angle {angle} does not lie on the uniform grid "
                f"(step {step} degrees)"
            )
        if used[slot]:
            raise ReconstructionError(f"duplicate angle {expected[slot]}")
        used[slot] = True
        order[slot] = index
    if not all(used):
        missing = [expected[k] for k in range(n_angles) if not used[k]]
        raise ReconstructionError(f"missing angles: {missing}")

    sorted_angles = [angles[order[k]] for k in range(n_angles)]
    sorted_rows = [rows[order[k]] for k in range(n_angles)]
    return sorted_angles, sorted_rows


def centered_coordinates(count, spacing):
    """Coordinates centered on the geometric origin (no half-shift)."""
    origin = (count - 1) / 2.0
    return [(i - origin) * spacing for i in range(count)]


def _dft(values, inverse=False):
    """Naive O(N^2) DFT; sizes here are at most a few hundred."""
    n = len(values)
    sign = 1.0 if inverse else -1.0
    out = []
    for k in range(n):
        total = 0j
        for j, value in enumerate(values):
            total += value * cmath.exp(sign * 2j * math.pi * k * j / n)
        if inverse:
            total /= n
        out.append(total)
    return out


def next_power_of_two(value):
    result = 1
    while result < value:
        result *= 2
    return result


def ramp_filter_projection(readings, spacing, pad_to=None):
    """Filter one projection with the discrete ramp H(f) = |f|.

    The projection is zero-padded to at least twice its length (rounded
    up to a power of two), transformed, multiplied by |f| in cycles per
    unit length, inverse transformed, and cropped back to the original
    detector positions. No normalization of the maximum is applied.
    """
    n = len(readings)
    if pad_to is None:
        pad_to = next_power_of_two(2 * n)
    if pad_to < 2 * n:
        raise ReconstructionError(
            f"padding length {pad_to} smaller than 2 * {n}"
        )
    padded = [complex(v) for v in readings] + [0j] * (pad_to - n)
    spectrum = _dft(padded)
    filtered = []
    for k, value in enumerate(spectrum):
        freq = k / (pad_to * spacing)
        if k > pad_to // 2:
            freq -= 1.0 / spacing
        filtered.append(value * abs(freq))
    time_domain = _dft(filtered, inverse=True)
    return [time_domain[i].real for i in range(n)]


def _interpolate(coords, values, s):
    """Linear interpolation of values at s; zero outside the range."""
    n = len(coords)
    if s <= coords[0] or s >= coords[-1]:
        return 0.0
    spacing = coords[1] - coords[0]
    position = (s - coords[0]) / spacing
    left = int(math.floor(position))
    if left >= n - 1:
        return 0.0
    frac = position - left
    return values[left] * (1.0 - frac) + values[left + 1] * frac


def backproject(filtered_rows, angles_deg, det_coords, xs, ys):
    """Sum interpolated contributions and scale by the angular step."""
    image = [[0.0 for _ in xs] for _ in ys]
    for angle, filtered in zip(angles_deg, filtered_rows):
        theta = math.radians(angle)
        cos_t = math.cos(theta)
        sin_t = math.sin(theta)
        for iy, y in enumerate(ys):
            row = image[iy]
            for ix, x in enumerate(xs):
                s = x * cos_t + y * sin_t
                row[ix] += _interpolate(det_coords, filtered, s)
    step_rad = math.pi / len(angles_deg)
    return [[value * step_rad for value in row] for row in image]


def reconstruct(angles_deg, sinogram, config):
    """Run the full FBP pipeline and return (image, summary)."""
    n_det = len(sinogram[0])
    spacing = config["detector_spacing"]
    pad_to = next_power_of_two(2 * n_det)
    det_coords = centered_coordinates(n_det, spacing)
    xs = centered_coordinates(config["nx"], config["pixel_size"])
    ys = centered_coordinates(config["ny"], config["pixel_size"])

    filtered_rows = [
        ramp_filter_projection(row, spacing, pad_to) for row in sinogram
    ]
    image = backproject(filtered_rows, angles_deg, det_coords, xs, ys)

    summary = {
        "image": {
            "nx": config["nx"],
            "ny": config["ny"],
            "pixel_size": config["pixel_size"],
            "x_range": [xs[0], xs[-1]],
            "y_range": [ys[0], ys[-1]],
        },
        "detector": {
            "count": n_det,
            "spacing": spacing,
            "coordinate_range": [det_coords[0], det_coords[-1]],
        },
        "angles": {
            "count": len(angles_deg),
            "step_degrees": 180.0 / len(angles_deg),
            "range_degrees": [0.0, 180.0],
            "right_endpoint_included": False,
        },
        "filter": {
            "name": "ramp",
            "multiplier": "abs(frequency)",
            "frequency_units": "cycles per unit length",
            "padded_length": pad_to,
            "zero_padding_factor": pad_to / n_det,
            "max_normalization": False,
        },
        "statistics": {
            "min": min(min(row) for row in image),
            "max": max(max(row) for row in image),
        },
    }
    return image, summary


def disk_sinogram(angles_deg, n_det, spacing, radius, amplitude=1.0):
    """Analytic projections of a centered uniform disk.

    p(s, theta) = 2 * amplitude * sqrt(radius^2 - s^2) for |s| <= radius.
    """
    det_coords = centered_coordinates(n_det, spacing)
    rows = []
    for _angle in angles_deg:
        row = []
        for s in det_coords:
            if abs(s) < radius:
                row.append(2.0 * amplitude * math.sqrt(radius * radius - s * s))
            else:
                row.append(0.0)
        rows.append(row)
    return rows


def write_matrix_csv(path, image):
    with open(path, "w", encoding="utf-8", newline="") as handle:
        writer = csv.writer(handle)
        for row in image:
            writer.writerow([f"{value:.10g}" for value in row])


def write_summary_json(path, summary):
    with open(path, "w", encoding="utf-8") as handle:
        json.dump(summary, handle, indent=2, sort_keys=True)
        handle.write("\n")
