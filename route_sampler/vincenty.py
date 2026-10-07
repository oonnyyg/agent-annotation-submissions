"""Vincenty inverse/direct solutions on an ellipsoid (stdlib only).

Angles are degrees, lengths are meters. Longitudes are normalized to
[-180, 180); azimuths to [0, 360).
"""

from __future__ import annotations

import math
from dataclasses import dataclass


class VincentyNotConverged(Exception):
    """Raised when Vincenty iteration fails to converge within the limit."""


def normalize_lon(lon: float) -> float:
    """Normalize longitude to the half-open interval [-180, 180)."""
    result = math.fmod(lon + 180.0, 360.0)
    if result < 0.0:
        result += 360.0
    result -= 180.0
    # fmod can yield 180.0 for inputs like -180; fold it to -180.
    if result >= 180.0:
        result -= 360.0
    return result


def normalize_azimuth(az: float) -> float:
    """Normalize azimuth in degrees to [0, 360)."""
    result = math.fmod(az, 360.0)
    if result < 0.0:
        result += 360.0
    return result


@dataclass(frozen=True)
class InverseResult:
    distance: float
    azimuth1: float
    azimuth2: float
    iterations: int


@dataclass(frozen=True)
class DirectResult:
    lat2: float
    lon2: float
    azimuth2: float
    iterations: int


def inverse(
    lat1: float,
    lon1: float,
    lat2: float,
    lon2: float,
    a: float,
    f: float,
    tol: float,
    max_iter: int,
) -> InverseResult:
    """Vincenty inverse: distance (m) and forward azimuths (deg)."""
    if lat1 == lat2 and normalize_lon(lon1) == normalize_lon(lon2):
        return InverseResult(0.0, 0.0, 0.0, 0)

    b = a * (1.0 - f)
    phi1 = math.radians(lat1)
    phi2 = math.radians(lat2)
    L = math.radians(normalize_lon(lon2) - normalize_lon(lon1))
    U1 = math.atan((1.0 - f) * math.tan(phi1))
    U2 = math.atan((1.0 - f) * math.tan(phi2))
    sinU1, cosU1 = math.sin(U1), math.cos(U1)
    sinU2, cosU2 = math.sin(U2), math.cos(U2)

    lam = L
    sin_sigma = cos_sigma = sigma = sin_alpha = cos2_alpha = cos2sigma_m = 0.0
    iterations = 0
    for iterations in range(1, max_iter + 1):
        sin_lam, cos_lam = math.sin(lam), math.cos(lam)
        sin_sigma = math.sqrt(
            (cosU2 * sin_lam) ** 2
            + (cosU1 * sinU2 - sinU1 * cosU2 * cos_lam) ** 2
        )
        if sin_sigma == 0.0:
            return InverseResult(0.0, 0.0, 0.0, iterations)
        cos_sigma = sinU1 * sinU2 + cosU1 * cosU2 * cos_lam
        sigma = math.atan2(sin_sigma, cos_sigma)
        sin_alpha = cosU1 * cosU2 * sin_lam / sin_sigma
        cos2_alpha = 1.0 - sin_alpha * sin_alpha
        if cos2_alpha == 0.0:
            cos2sigma_m = 0.0
        else:
            cos2sigma_m = cos_sigma - 2.0 * sinU1 * sinU2 / cos2_alpha
        C = f / 16.0 * cos2_alpha * (4.0 + f * (4.0 - 3.0 * cos2_alpha))
        lam_prev = lam
        lam = L + (1.0 - C) * f * sin_alpha * (
            sigma
            + C
            * sin_sigma
            * (
                cos2sigma_m
                + C * cos_sigma * (-1.0 + 2.0 * cos2sigma_m * cos2sigma_m)
            )
        )
        if abs(lam - lam_prev) <= tol:
            break
    else:
        raise VincentyNotConverged(
            f"inverse did not converge after {max_iter} iterations"
        )

    u2 = cos2_alpha * (a * a - b * b) / (b * b)
    A = 1.0 + u2 / 16384.0 * (
        4096.0 + u2 * (-768.0 + u2 * (320.0 - 175.0 * u2))
    )
    B = u2 / 1024.0 * (256.0 + u2 * (-128.0 + u2 * (74.0 - 47.0 * u2)))
    delta_sigma = (
        B
        * sin_sigma
        * (
            cos2sigma_m
            + B
            / 4.0
            * (
                cos_sigma * (-1.0 + 2.0 * cos2sigma_m * cos2sigma_m)
                - B
                / 6.0
                * cos2sigma_m
                * (-3.0 + 4.0 * sin_sigma * sin_sigma)
                * (-3.0 + 4.0 * cos2sigma_m * cos2sigma_m)
            )
        )
    )
    distance = b * A * (sigma - delta_sigma)

    azimuth1 = math.degrees(
        math.atan2(
            cosU2 * math.sin(lam),
            cosU1 * sinU2 - sinU1 * cosU2 * math.cos(lam),
        )
    )
    azimuth2 = math.degrees(
        math.atan2(
            cosU1 * math.sin(lam),
            -sinU1 * cosU2 + cosU1 * sinU2 * math.cos(lam),
        )
    )
    return InverseResult(
        distance,
        normalize_azimuth(azimuth1),
        normalize_azimuth(azimuth2),
        iterations,
    )


def direct(
    lat1: float,
    lon1: float,
    azimuth1: float,
    distance: float,
    a: float,
    f: float,
    tol: float,
    max_iter: int,
) -> DirectResult:
    """Vincenty direct: destination point (deg) given start, azimuth, distance."""
    b = a * (1.0 - f)
    alpha1 = math.radians(azimuth1)
    sin_alpha1, cos_alpha1 = math.sin(alpha1), math.cos(alpha1)
    U1 = math.atan((1.0 - f) * math.tan(math.radians(lat1)))
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
    iterations = 0
    for iterations in range(1, max_iter + 1):
        cos2sigma_m = math.cos(2.0 * sigma1 + sigma)
        sin_sigma, cos_sigma = math.sin(sigma), math.cos(sigma)
        delta_sigma = (
            B
            * sin_sigma
            * (
                cos2sigma_m
                + B
                / 4.0
                * (
                    cos_sigma * (-1.0 + 2.0 * cos2sigma_m * cos2sigma_m)
                    - B
                    / 6.0
                    * cos2sigma_m
                    * (-3.0 + 4.0 * sin_sigma * sin_sigma)
                    * (-3.0 + 4.0 * cos2sigma_m * cos2sigma_m)
                )
            )
        )
        sigma_prev = sigma
        sigma = distance / (b * A) + delta_sigma
        if abs(sigma - sigma_prev) <= tol:
            break
    else:
        raise VincentyNotConverged(
            f"direct did not converge after {max_iter} iterations"
        )

    tmp = sinU1 * sin_sigma - cosU1 * cos_sigma * cos_alpha1
    lat2 = math.degrees(
        math.atan2(
            sinU1 * cos_sigma + cosU1 * sin_sigma * cos_alpha1,
            (1.0 - f) * math.sqrt(sin_alpha * sin_alpha + tmp * tmp),
        )
    )
    lam = math.atan2(
        sin_sigma * sin_alpha1,
        cosU1 * cos_sigma - sinU1 * sin_sigma * cos_alpha1,
    )
    C = f / 16.0 * cos2_alpha * (4.0 + f * (4.0 - 3.0 * cos2_alpha))
    L = lam - (1.0 - C) * f * sin_alpha * (
        sigma
        + C
        * sin_sigma
        * (
            cos2sigma_m
            + C * cos_sigma * (-1.0 + 2.0 * cos2sigma_m * cos2sigma_m)
        )
    )
    lon2 = normalize_lon(lon1 + math.degrees(L))
    azimuth2 = math.degrees(math.atan2(sin_alpha, -tmp))
    return DirectResult(lat2, lon2, normalize_azimuth(azimuth2), iterations)
