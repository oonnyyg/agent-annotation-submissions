"""Stereo ray reconstruction from matched pixels.

Reconstructs 3D points from matched pixel observations of two cameras,
using only the Python standard library. See README.md for conventions.
"""

from __future__ import annotations

import csv
import json
import math
import sys
from dataclasses import dataclass


def v_sub(a, b):
    return (a[0] - b[0], a[1] - b[1], a[2] - b[2])


def v_dot(a, b):
    return a[0] * b[0] + a[1] * b[1] + a[2] * b[2]


def v_norm(a):
    return math.sqrt(v_dot(a, a))


def v_scale(a, s):
    return (a[0] * s, a[1] * s, a[2] * s)


def v_add(a, b):
    return (a[0] + b[0], a[1] + b[1], a[2] + b[2])


def mat_vec(m, v):
    return tuple(v_dot(row, v) for row in m)


def mat_transpose(m):
    return tuple(tuple(m[r][c] for r in range(3)) for c in range(3))


@dataclass
class Camera:
    """Pinhole camera with two-term radial distortion.

    rotation maps camera-frame vectors to world frame (right-handed,
    orthonormal). center is the optical center in world coordinates.
    Positive depth is along the local third axis.
    """

    name: str
    fx: float
    fy: float
    cx: float
    cy: float
    k1: float
    k2: float
    center: tuple
    rotation: tuple  # 3x3, camera -> world

    @staticmethod
    def from_dict(name, data):
        fx = float(data["fx"])
        fy = float(data["fy"])
        k1 = float(data.get("k1", 0.0))
        k2 = float(data.get("k2", 0.0))
        if fx <= 0.0 or fy <= 0.0:
            raise ValueError(f"camera {name}: focal lengths must be positive")
        if k1 < 0.0 or k2 < 0.0:
            raise ValueError(f"camera {name}: distortion coefficients must be non-negative")
        center = tuple(float(x) for x in data["center"])
        rotation = tuple(tuple(float(x) for x in row) for row in data["rotation"])
        if len(center) != 3 or len(rotation) != 3 or any(len(r) != 3 for r in rotation):
            raise ValueError(f"camera {name}: center must be 3-vector, rotation 3x3")
        _check_rotation(name, rotation)
        values = [fx, fy, float(data["cx"]), float(data["cy"]), k1, k2] + list(center)
        values += [x for row in rotation for x in row]
        if not all(math.isfinite(x) for x in values):
            raise ValueError(f"camera {name}: all values must be finite")
        return Camera(
            name=name,
            fx=fx,
            fy=fy,
            cx=float(data["cx"]),
            cy=float(data["cy"]),
            k1=k1,
            k2=k2,
            center=center,
            rotation=rotation,
        )

    def undistort_normalized(self, xd, yd):
        """Invert radial distortion: distorted -> undistorted normalized.

        Model: xd = xu * (1 + k1 r^2 + k2 r^4), same for y. Solved with
        Newton iteration on the radial scale factor.
        """
        rd2 = xd * xd + yd * yd
        if rd2 == 0.0:
            return xd, yd
        rd = math.sqrt(rd2)
        ru = rd
        for _ in range(50):
            r2 = ru * ru
            f = ru * (1.0 + self.k1 * r2 + self.k2 * r2 * r2) - rd
            df = 1.0 + 3.0 * self.k1 * r2 + 5.0 * self.k2 * r2 * r2
            step = f / df
            ru -= step
            if abs(step) < 1e-15 * max(1.0, rd):
                break
        if ru <= 0.0:
            raise ValueError("undistortion failed to converge to a positive radius")
        scale = ru / rd
        return xd * scale, yd * scale

    def pixel_to_world_ray(self, u, v):
        """Observed pixel -> unit ray direction in world coordinates."""
        xd = (u - self.cx) / self.fx
        yd = (v - self.cy) / self.fy
        xu, yu = self.undistort_normalized(xd, yd)
        d_cam = (xu, yu, 1.0)
        n = v_norm(d_cam)
        d_cam = v_scale(d_cam, 1.0 / n)
        return mat_vec(self.rotation, d_cam)

    def project(self, point_world):
        """World point -> (u, v, depth). Raises if depth is non-positive."""
        pc = mat_vec(mat_transpose(self.rotation), v_sub(point_world, self.center))
        z = pc[2]
        if z <= 0.0:
            raise ValueError("point has non-positive depth in camera frame")
        xu = pc[0] / z
        yu = pc[1] / z
        r2 = xu * xu + yu * yu
        s = 1.0 + self.k1 * r2 + self.k2 * r2 * r2
        u = self.fx * xu * s + self.cx
        v = self.fy * yu * s + self.cy
        return u, v, z


def _check_rotation(name, m):
    for row in m:
        if abs(v_norm(row) - 1.0) > 1e-6:
            raise ValueError(f"camera {name}: rotation rows must be unit vectors")
    for i in range(3):
        for j in range(i + 1, 3):
            if abs(v_dot(m[i], m[j])) > 1e-6:
                raise ValueError(f"camera {name}: rotation rows must be orthogonal")
    cross = (
        m[0][1] * m[1][2] - m[0][2] * m[1][1],
        m[0][2] * m[1][0] - m[0][0] * m[1][2],
        m[0][0] * m[1][1] - m[0][1] * m[1][0],
    )
    if v_dot(cross, m[2]) < 0.0:
        raise ValueError(f"camera {name}: rotation must be right-handed")


# ---------------------------------------------------------------------------
# Triangulation
# ---------------------------------------------------------------------------

class GeometryError(Exception):
    """Raised when a point cannot be localized for geometric reasons."""


def closest_points_on_lines(p1, d1, p2, d2):
    """Closest points of two lines (unit directions). Returns (q1, q2, dist)."""
    w0 = v_sub(p1, p2)
    b = v_dot(d1, d2)
    d = v_dot(d1, w0)
    e = v_dot(d2, w0)
    denom = 1.0 - b * b
    if denom <= 0.0:
        raise GeometryError("rays are parallel")
    s = (b * e - d) / denom
    t = (e - b * d) / denom
    q1 = v_add(p1, v_scale(d1, s))
    q2 = v_add(p2, v_scale(d2, t))
    return q1, q2, v_norm(v_sub(q1, q2)), s, t


def triangulate(cam1, px1, cam2, px2, angle_threshold_deg):
    """Reconstruct one point. Returns a result dict."""
    if not (0.0 < angle_threshold_deg < 90.0):
        raise ValueError("angle threshold must be in (0, 90) degrees")
    if v_norm(v_sub(cam1.center, cam2.center)) < 1e-12:
        raise GeometryError("camera centers coincide")

    d1 = cam1.pixel_to_world_ray(*px1)
    d2 = cam2.pixel_to_world_ray(*px2)

    cos_angle = max(-1.0, min(1.0, v_dot(d1, d2)))
    angle_deg = math.degrees(math.acos(cos_angle))
    if angle_deg < angle_threshold_deg or 180.0 - angle_deg < angle_threshold_deg:
        raise GeometryError(
            f"rays too close to parallel/antiparallel: angle {angle_deg:.3f} deg"
        )

    q1, q2, gap, s1, s2 = closest_points_on_lines(cam1.center, d1, cam2.center, d2)
    if s1 <= 1e-12 or s2 <= 1e-12:
        raise GeometryError("closest point lies behind or at a camera center")

    midpoint = v_scale(v_add(q1, q2), 0.5)

    residuals = {}
    for cam, px, tag in ((cam1, px1, "left"), (cam2, px2, "right")):
        u, v, _z = cam.project(midpoint)
        residuals[tag] = {
            "reprojected_pixel": [u, v],
            "residual_pixels": [u - px[0], v - px[1]],
            "residual_norm_pixels": math.hypot(u - px[0], v - px[1]),
        }

    return {
        "point": list(midpoint),
        "ray_angle_deg": angle_deg,
        "closest_distance": gap,
        "ray_parameters": [s1, s2],
        "left": residuals["left"],
        "right": residuals["right"],
    }


# ---------------------------------------------------------------------------
# I/O and CLI
# ---------------------------------------------------------------------------

def load_cameras(path):
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    cams = data["cameras"]
    if "left" not in cams or "right" not in cams:
        raise ValueError("camera JSON must define 'left' and 'right' cameras")
    return Camera.from_dict("left", cams["left"]), Camera.from_dict("right", cams["right"])


def read_matches(path):
    """CSV rows: point_id,left_u,left_v,right_u,right_v (header optional)."""
    rows = []
    seen = set()
    with open(path, "r", encoding="utf-8", newline="") as fh:
        for record in csv.reader(fh):
            if not record or all(not c.strip() for c in record):
                continue
            if record[0].strip().lower() in ("point_id", "id"):
                continue  # header
            if len(record) != 5:
                raise ValueError(f"bad CSV row (need 5 columns): {record!r}")
            pid = record[0].strip()
            if pid in seen:
                raise ValueError(f"duplicate point id: {pid}")
            seen.add(pid)
            coords = [float(x) for x in record[1:]]
            if not all(math.isfinite(x) for x in coords):
                raise ValueError(f"non-finite pixel coordinate for point {pid}")
            rows.append((pid, (coords[0], coords[1]), (coords[2], coords[3])))
    return rows


def run(cameras_path, csv_path, report_path, angle_threshold_deg):
    cam_left, cam_right = load_cameras(cameras_path)
    matches = read_matches(csv_path)

    results = []
    n_ok = 0
    for pid, px_left, px_right in matches:
        entry = {"id": pid, "observed_left": list(px_left), "observed_right": list(px_right)}
        try:
            rec = triangulate(cam_left, px_left, cam_right, px_right, angle_threshold_deg)
        except (GeometryError, ValueError) as exc:
            entry["status"] = "failed"
            entry["reason"] = str(exc)
        else:
            entry["status"] = "ok"
            entry.update(rec)
            n_ok += 1
        results.append(entry)

    report = {
        "config": {
            "cameras": cameras_path,
            "matches": csv_path,
            "angle_threshold_deg": angle_threshold_deg,
        },
        "summary": {
            "total": len(results),
            "ok": n_ok,
            "failed": len(results) - n_ok,
        },
        "points": results,
    }
    with open(report_path, "w", encoding="utf-8") as fh:
        json.dump(report, fh, indent=2)
        fh.write("\n")
    return report


def main(argv=None):
    import argparse

    parser = argparse.ArgumentParser(
        description="Reconstruct 3D points from matched stereo pixels."
    )
    parser.add_argument("cameras", help="camera JSON file")
    parser.add_argument("matches", help="CSV of matched pixels")
    parser.add_argument("report", help="output JSON quality report")
    parser.add_argument(
        "--angle-threshold",
        type=float,
        default=1.0,
        help="min ray angle in degrees, must be in (0, 90); default 1.0",
    )
    args = parser.parse_args(argv)
    if not (0.0 < args.angle_threshold < 90.0):
        parser.error("--angle-threshold must be in (0, 90) degrees")
    report = run(args.cameras, args.matches, args.report, args.angle_threshold)
    s = report["summary"]
    print(f"reconstructed {s['ok']}/{s['total']} points "
          f"({s['failed']} failed) -> {args.report}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
