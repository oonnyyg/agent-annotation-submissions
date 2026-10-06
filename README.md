# Stereo Ray Reconstruction

Reconstructs 3D points from matched stereo pixels using only the Python
standard library. No image recognition is performed — the input CSV already
contains matched pixel correspondences.

## Coordinate conventions

- Pixels in image coordinates, metric quantities in metres.
- Each camera has positive focal lengths `fx`, `fy`, principal point
  `(cx, cy)`, optical centre `center` in world coordinates, and a
  right-handed orthonormal 3x3 `rotation` mapping **camera frame to world
  frame**. Positive depth is along the local third axis (`+z` in camera
  frame).
- Radial distortion (two coefficients, tangential ignored): the normalised
  radius is magnified by `1 + k1 * r^2 + k2 * r^4`. Both coefficients are
  non-negative. Observed pixels are **undistorted by Newton inversion**
  before rays are built — distorted pixels are never connected directly.

## Method

1. Undistort each pixel, form a unit ray in the camera frame to world frame.
2. Reject if optical centres coincide, or if the angle between the sight
   *lines* (parallel or anti-parallel) is below `--min-angle-deg`.
3. Compute the closest points of the two (generally skew) lines; the
   midpoint is the estimate, their separation the reported `ray_distance`.
4. Reject if either closest point lies on or behind its camera centre —
   no pseudo-solutions with deceptively small distances.
5. Reproject the estimate through the full distortion model into both
   images and report per-camera pixel residuals. Non-positive depth in
   either camera fails the point.

Per-point failures are recorded in `failures` and never silently dropped;
one bad point does not affect the others.

## Usage

```
python -m stereo_ray.cli --cameras cameras.json --matches matches.csv \
    [--min-angle-deg 1.0] [--output report.json]
```

- `cameras.json`: `{"left": {...}, "right": {...}}` with the fields above
  (`k1`, `k2` optional, default 0).
- `matches.csv`: `id,u_left,v_left,u_right,v_right` per line; an optional
  header row and `#` comments are allowed; ids must be unique.
- `--min-angle-deg`: ray-angle rejection threshold, must lie in (0, 90).

The JSON report contains a `summary` (counts, max/mean ray distance, max
absolute reprojection residual), per-point `points` (estimate, ray
distance, ray angle, depths, pixel residuals), and `failures` with reasons.

## Tolerances

- Rotation validity: column norms and orthogonality within `1e-6`,
  determinant must be positive.
- Newton undistortion: up to 50 iterations, step convergence `1e-15`.
- Exact synthetic correspondences reconstruct to ~`1e-9` m; distorted
  observations to ~`1e-7` m (see `tests/`).

## Tests

```
python -m unittest discover -s tests -v
```

Covers distorted points, skew (non-intersecting) rays, behind-camera
intersections, parallel rays, coincident centres, and threshold
configuration. `examples/` holds a runnable end-to-end dataset.
