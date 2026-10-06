# Stereo Ray Reconstruction

Reconstructs 3D points from matched stereo pixels using only the Python
standard library. It does **not** do image recognition: input matches are
already-established pixel correspondences.

## Usage

```
python stereo_reconstruct.py cameras.json matches.csv report.json \
    [--angle-threshold DEG]
```

- `cameras.json` — camera rig description (see `examples/cameras.json`).
- `matches.csv` — one matched point per row:
  `point_id,left_u,left_v,right_u,right_v` (header line optional; point ids
  must be unique).
- `report.json` — quality report with per-point status, 3D estimate,
  ray gap, and reprojection residuals.
- `--angle-threshold` — minimum ray angle in degrees, must be in
  `(0, 90)`. Default `1.0`.

## Coordinate conventions

- World coordinates are in meters; pixels are in image pixel units.
- Each camera gives positive focal lengths `fx`, `fy`, principal point
  `(cx, cy)`, optical center `center` in world coordinates, and a 3x3
  right-handed orthonormal `rotation` mapping **camera-frame** vectors
  **to world** frame.
- Positive depth is along the camera's local third axis (+z). A point is
  visible only if its camera-frame z is strictly positive.
- Radial distortion (two terms, no tangential): normalized distorted
  coordinates satisfy `xd = xu * (1 + k1*r^2 + k2*r^4)` with
  `r^2 = xu^2 + yu^2`; `k1, k2 >= 0`. Observed pixels are undistorted by
  Newton iteration on the radial scale before rays are built — distorted
  pixels are never used directly.

## Method

1. Undistort each observed pixel, build a unit ray in camera coordinates,
   rotate it into the world frame.
2. Reject the point if the camera centers coincide, or if the ray angle is
   within `--angle-threshold` of 0° (parallel) or 180° (antiparallel).
3. Compute the closest points of the two (generally skew) lines. Reject if
   either closest point lies behind or at its camera center (non-positive
   ray parameter).
4. The midpoint of the closest-point segment is the 3D estimate; the
   segment length is reported as `closest_distance` (meters).
5. The estimate is reprojected through the same camera model (including
   distortion) into both images. Non-positive depth during reprojection
   fails the point. Per-pixel residuals (`reprojected - observed`, in
   pixels) are reported for both cameras.

A geometric failure of one point is recorded as `"status": "failed"` with
a reason and never affects other points; no points are dropped from the
report.

## Tolerances

- Rotation validation: row norms and orthogonality within `1e-6`.
- Coincident centers: baseline `< 1e-12` m.
- Behind-camera check: ray parameter `<= 1e-12` (unit rays, so meters).
- Undistortion Newton solve: up to 50 iterations, step convergence at
  `1e-15` relative.

## Tests

```
python -m unittest test_stereo_reconstruct -v
```

Covers distortion inversion round-trips, skew-ray midpoint/gap, and
rejection of parallel/antiparallel rays, coincident centers, and
behind-camera intersections.
