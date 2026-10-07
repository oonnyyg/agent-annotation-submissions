"""Geometry configuration loading and validation."""

from __future__ import annotations

import json
from dataclasses import dataclass

MAX_ANGLES = 64
MAX_DETECTORS = 64
MAX_IMAGE_DIM = 64


class ConfigError(ValueError):
    """Raised when the geometry configuration is invalid."""


@dataclass(frozen=True)
class Geometry:
    """Reconstruction geometry.

    All coordinates are centred on the geometric centre of the detector
    array and of the image grid.  Even-sized grids are NOT shifted by half
    a sample: detector ``i`` sits at ``(i - (n-1)/2) * spacing`` and pixel
    ``(ix, iy)`` at ``((ix - (nx-1)/2) * dx, (iy - (ny-1)/2) * dy)``.
    """

    detector_count: int
    detector_spacing: float
    nx: int
    ny: int
    dx: float
    dy: float

    def detector_positions(self) -> list[float]:
        c = (self.detector_count - 1) / 2.0
        return [(i - c) * self.detector_spacing for i in range(self.detector_count)]

    def x_range(self) -> tuple[float, float]:
        c = (self.nx - 1) / 2.0 * self.dx
        return (-c, c)

    def y_range(self) -> tuple[float, float]:
        c = (self.ny - 1) / 2.0 * self.dy
        return (-c, c)

    def pixel_centers(self) -> list[tuple[float, float]]:
        cx = (self.nx - 1) / 2.0
        cy = (self.ny - 1) / 2.0
        return [
            ((ix - cx) * self.dx, (iy - cy) * self.dy)
            for iy in range(self.ny)
            for ix in range(self.nx)
        ]


def _positive_int(value: object, name: str) -> int:
    if not isinstance(value, int) or isinstance(value, bool) or value <= 0:
        raise ConfigError(f"{name} must be a positive integer, got {value!r}")
    return value


def _positive_float(value: object, name: str) -> float:
    if not isinstance(value, (int, float)) or isinstance(value, bool) or value <= 0:
        raise ConfigError(f"{name} must be a positive number, got {value!r}")
    return float(value)


def load_config(path: str) -> Geometry:
    with open(path, "r", encoding="utf-8") as fh:
        raw = json.load(fh)
    if not isinstance(raw, dict):
        raise ConfigError("configuration root must be a JSON object")
    try:
        detector_count = _positive_int(raw["detector_count"], "detector_count")
        detector_spacing = _positive_float(raw["detector_spacing"], "detector_spacing")
        image = raw["image"]
        nx = _positive_int(image["nx"], "image.nx")
        ny = _positive_int(image["ny"], "image.ny")
        dx = _positive_float(image["dx"], "image.dx")
        dy = _positive_float(image["dy"], "image.dy")
    except KeyError as exc:
        raise ConfigError(f"missing configuration key: {exc}") from exc
    if detector_count > MAX_DETECTORS:
        raise ConfigError(
            f"detector_count {detector_count} exceeds limit {MAX_DETECTORS}"
        )
    if nx > MAX_IMAGE_DIM or ny > MAX_IMAGE_DIM:
        raise ConfigError(
            f"image grid {nx}x{ny} exceeds limit {MAX_IMAGE_DIM}x{MAX_IMAGE_DIM}"
        )
    return Geometry(detector_count, detector_spacing, nx, ny, dx, dy)
