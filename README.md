# Transit Exposure Model

Computes the light curve of a single opaque planet transiting a quadratic
limb-darkened star, comparing the **instantaneous** brightness with the
**exposure-averaged** brightness. Pure Python standard library; no
astronomy services are called.

## Model

- Star: unit-radius disk, centre intensity 1, quadratic limb darkening
  `I(mu) = 1 - u1 (1 - mu) - u2 (1 - mu)^2` with `mu = sqrt(1 - r^2)`.
  Coefficients must satisfy `u1, u2 >= 0` and `u1 + u2 <= 1`.
- Planet: opaque disk of radius `R` (planet/star radius ratio, `0 < R <= 1`)
  moving on a **straight line** at constant projected velocity `v`
  (stellar radii per hour): separation `d(t) = sqrt(b^2 + v^2 (t - t_mid)^2)`.
- Loss is the surface brightness integral over the overlap region of the
  two disks, divided by the total unocculted flux
  `F = pi (1 - u1/3 - u2/6)` — never a plain area ratio (that is only the
  uniform-brightness limit `u1 = u2 = 0`).

## Numerical method

- `I(r)` is a polynomial in `mu`, so the radial integrals of
  `r {1, mu, mu^2}` are analytic. The remaining integral over the polar
  angle around the star-planet axis is evaluated with adaptive Simpson
  quadrature. Radial moments are evaluated in factored `(width, sum)`
  form and the tangency cusp is removed by the substitution
  `theta = theta_max - u^2`, so the requested tolerance is genuinely
  reached instead of being faked by clamping.
- Exposure means integrate the instantaneous curve over the full exposure
  interval, split at all contact times (where the integrand has kinks),
  with per-segment tolerances proportional to segment length. Zero
  duration returns the instantaneous value.
- Non-convergence is reported in the JSON diagnostics and per output row;
  values are never truncated to `[0, 1]` to hide errors.

## Usage

```
python transit_cli.py config.json exposures.csv out.csv diagnostics.json
```

`config.json`:

```json
{
  "planet_radius_ratio": 0.1,
  "impact_parameter": 0.3,
  "t_mid_hours": 10.0,
  "velocity": 0.5,
  "limb_darkening": [0.3, 0.2],
  "tolerance": 1e-10
}
```

`exposures.csv` (header required): `center_hours,duration_hours`.

Outputs:

- `out.csv`: `center_hours, duration_hours, instantaneous, exposure_mean,
  converged` per exposure.
- `diagnostics.json`: total flux, external contact interval `(t1, t4)`
  (`null` when the planet misses or only grazes the star), all contact
  times, overall convergence flag, and quadrature statistics.

The CLI exits non-zero and prints a warning if any quadrature fails to
converge.

## Tests

```
python -m unittest test_transit -v
```

Covers the uniform-brightness limit against the Mandel-Agol area formula,
central transits, grazing/missing geometries, ingress/egress-straddling
exposures, long exposures, zero-duration exposures, input validation, and
non-convergence reporting.

## Limitations

- Straight-line, constant-velocity trajectory (no orbital curvature,
  acceleration, or planet motion during the exposure other than the
  linear ephemeris).
- Single planet, no starspots, no blending, no photometric noise.
- Adaptive Simpson is a finite approximation: accuracy is governed by
  `tolerance` and `max_depth`; failures are reported, not hidden.
