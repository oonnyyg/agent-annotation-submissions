"""Vincenty inverse and direct solutions on a reference ellipsoid.

All angles are degrees, distances are metres. Longitudes are normalised
to [-180, 180), bearings to [0, 360).
"""

from __future__ import annotations

import math
from dataclasses import dataclass


class VincentyError(Exception):
    """Raised when Vincenty's iteration fails to converge."""


@dataclass(frozen=True)
class Ellipsoid:
    """Reference ellipsoid defined by semi-major axis and flattening."""

    a: float  # semi-major axis in metres
    f: float  # flattening, 0 <= f < 0.01

    def __post_init__(self) -> None:
        if not math.isfinite(self.a) or self.a <= 0.0:
            raise ValueError("semi-major axis must be a positive finite number")
        if not math.isfinite(self.f) or not (0.0 <= self.f < 0.01):
            raise ValueError("flattening must satisfy 0 <= f < 0.01")

    @property
    def b(self) -> float:
        return self.a * (1.0 - self.f)


WGS84 = Ellipsoid(a=6378137.0, f=1.0 / 298.257223563)


def normalize_lon(lon: float) -> float:
    """Normalise longitude to [-180, 180)."""
    return (lon + 180.0) % 360.0 - 180.0


def normalize_bearing(bearing: float) -> float:
    """Normalise bearing to [0, 360)."""
    return bearing % 360.0


def _check_lat_lon(lat: float, lon: float) -> None:
    if not (math.isfinite(lat) and math.isfinite(lon)):
        raise ValueError("latitude/longitude must be finite")
    if not -90.0 <= lat <= 90.0:
        raise ValueError("latitude out of range [-90, 90]")
    if abs(lat) == 90.0:
        raise ValueError("pole waypoints are not supported")
    if not -180.0 <= lon < 180.0:
        raise ValueError("longitude out of range [-180, 180)")


@dataclass(frozen=True)
class InverseResult:
    distance: float  # metres
    initial_bearing: float  # degrees in [0, 360); nan if zero-length
    final_bearing: float


@dataclass(frozen=True)
class DirectResult:
    lat: float
    lon: float  # normalised to [-180, 180)
    final_bearing: float


def inverse(ell: Ellipsoid, lat1: float, lon1: float, lat2: float, lon2: float,
            tol: float = 1e-12, max_iter: int = 200) -> InverseResult:
    """Vincenty inverse solution between two points."""
    _check_lat_lon(lat1, lon1)
    _check_lat_lon(lat2, lon2)
    dlon_deg = normalize_lon(lon2 - lon1)
    if lat1 == lat2 and dlon_deg == 0.0:
        return InverseResult(0.0, math.nan, math.nan)

    a, b, f = ell.a, ell.b, ell.f
    phi1, phi2 = math.radians(lat1), math.radians(lat2)
    L = math.radians(dlon_deg)
    U1 = math.atan((1.0 - f) * math.tan(phi1))
    U2 = math.atan((1.0 - f) * math.tan(phi2))
    sinU1, cosU1 = math.sin(U1), math.cos(U1)
    sinU2, cosU2 = math.sin(U2), math.cos(U2)

    lam = L
    cos2_alpha = 0.0
    cos2_sigma_m = 0.0
    sin_sigma = 0.0
    cos_sigma = 0.0
    sigma = 0.0
    for _ in range(max_iter):
        sin_lam, cos_lam = math.sin(lam), math.cos(lam)
        sin_sigma = math.sqrt(
            (cosU2 * sin_lam) ** 2
            + (cosU1 * sinU2 - sinU1 * cosU2 * cos_lam) ** 2
        )
        if sin_sigma == 0.0:
            return InverseResult(0.0, math.nan, math.nan)
        cos_sigma = sinU1 * sinU2 + cosU1 * cosU2 * cos_lam
        sigma = math.atan2(sin_sigma, cos_sigma)
        sin_alpha = cosU1 * cosU2 * sin_lam / sin_sigma
        cos2_alpha = 1.0 - sin_alpha * sin_alpha
        if cos2_alpha == 0.0:
            cos2_sigma_m = 0.0  # equatorial line
        else:
            cos2_sigma_m = cos_sigma - 2.0 * sinU1 * sinU2 / cos2_alpha
        C = f / 16.0 * cos2_alpha * (4.0 + f * (4.0 - 3.0 * cos2_alpha))
        lam_prev = lam
        lam = L + (1.0 - C) * f * sin_alpha * (
            sigma
            + C * sin_sigma
            * (cos2_sigma_m + C * cos_sigma * (-1.0 + 2.0 * cos2_sigma_m**2))
        )
        if abs(lam - lam_prev) <= tol:
            break
    else:
        raise VincentyError(
            f"inverse solution did not converge after {max_iter} iterations"
        )

    u2 = cos2_alpha * (a * a - b * b) / (b * b)
    A = 1.0 + u2 / 16384.0 * (
        4096.0 + u2 * (-768.0 + u2 * (320.0 - 175.0 * u2))
    )
    B = u2 / 1024.0 * (256.0 + u2 * (-128.0 + u2 * (74.0 - 47.0 * u2)))
    delta_sigma = B * sin_sigma * (
        cos2_sigma_m
        + B
        / 4.0
        * (
            cos_sigma * (-1.0 + 2.0 * cos2_sigma_m**2)
            - B
            / 6.0
            * cos2_sigma_m
            * (-3.0 + 4.0 * sin_sigma**2)
            * (-3.0 + 4.0 * cos2_sigma_m**2)
        )
    )
    distance = b * A * (sigma - delta_sigma)

    alpha1 = math.atan2(
        cosU2 * math.sin(lam),
        cosU1 * sinU2 - sinU1 * cosU2 * math.cos(lam),
    )
    alpha2 = math.atan2(
        cosU1 * math.sin(lam),
        -sinU1 * cosU2 + cosU1 * sinU2 * math.cos(lam),
    )
    return InverseResult(
        distance,
        normalize_bearing(math.degrees(alpha1)),
        normalize_bearing(math.degrees(alpha2)),
    )


def direct(ell: Ellipsoid, lat1: float, lon1: float, bearing: float,
           distance: float, tol: float = 1e-12,
           max_iter: int = 200) -> DirectResult:
    """Vincenty direct solution from a point along a bearing."""
    _check_lat_lon(lat1, lon1)
    if not math.isfinite(bearing) or not math.isfinite(distance):
        raise ValueError("bearing and distance must be finite")
    if distance < 0.0:
        raise ValueError("distance must be non-negative")

    a, b, f = ell.a, ell.b, ell.f
    alpha1 = math.radians(bearing)
    sin_alpha1, cos_alpha1 = math.sin(alpha1), math.cos(alpha1)
    phi1 = math.radians(lat1)
    U1 = math.atan((1.0 - f) * math.tan(phi1))
    sinU1, cosU1 = math.sin(U1), math.cos(U1)
    sigma1 = math.atan2(math.tan(U1), cos_alpha1)
    sin_alpha = cosU1 * sin_alpha1
    cos2_alpha = 1.0 - sin_alpha * sin_alpha
    u2 = cos2_alpha * (a * a - b * b) / (b * b)
    A = 1.0 + u2 / 16384.0 * (
        4096.0 + u2 * (-768.0 + u2 * (320.0 - 175.0 * u2))
    )
    B = u2 / 1024.0 * (256.0 + u2 * (-128.0 + u2 * (74.0 - 47.0 * u2)))

    sigma = distance / (b * A)
    for _ in range(max_iter):
        cos2_sigma_m = math.cos(2.0 * sigma1 + sigma)
        sin_sigma, cos_sigma = math.sin(sigma), math.cos(sigma)
        delta_sigma = B * sin_sigma * (
            cos2_sigma_m
            + B
            / 4.0
            * (
                cos_sigma * (-1.0 + 2.0 * cos2_sigma_m**2)
                - B
                / 6.0
                * cos2_sigma_m
                * (-3.0 + 4.0 * sin_sigma**2)
                * (-3.0 + 4.0 * cos2_sigma_m**2)
            )
        )
        sigma_prev = sigma
        sigma = distance / (b * A) + delta_sigma
        if abs(sigma - sigma_prev) <= tol:
            break
    else:
        raise VincentyError(
            f"direct solution did not converge after {max_iter} iterations"
        )

    cos2_sigma_m = math.cos(2.0 * sigma1 + sigma)
    sin_sigma, cos_sigma = math.sin(sigma), math.cos(sigma)
    tmp = sinU1 * sin_sigma - cosU1 * cos_sigma * cos_alpha1
    phi2 = math.atan2(
        sinU1 * cos_sigma + cosU1 * sin_sigma * cos_alpha1,
        (1.0 - f) * math.sqrt(sin_alpha * sin_alpha + tmp * tmp),
    )
    lam = math.atan2(
        sin_sigma * sin_alpha1,
        cosU1 * cos_sigma - sinU1 * sin_sigma * cos_alpha1,
    )
    C = f / 16.0 * cos2_alpha * (4.0 + f * (4.0 - 3.0 * cos2_alpha))
    L = lam - (1.0 - C) * f * sin_alpha * (
        sigma
        + C * sin_sigma
        * (cos2_sigma_m + C * cos_sigma * (-1.0 + 2.0 * cos2_sigma_m**2))
    )
    lon2 = normalize_lon(lon1 + math.degrees(L))
    alpha2 = math.atan2(sin_alpha, -tmp)
    return DirectResult(
        math.degrees(phi2), lon2, normalize_bearing(math.degrees(alpha2))
    )
