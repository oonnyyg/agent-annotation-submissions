# transit-exposure-model

Computes the light curve of a single opaque planet transiting a
limb-darkened star, using only the Python standard library. It reports both
the instantaneous flux and the average over each finite exposure, and never
approximates the blocked light by a pure area ratio when limb darkening is
present.

## Model

- **Star**: unit-radius disk, central intensity 1, quadratic limb darkening
  `I(mu) = 1 - u1*(1 - mu) - u2*(1 - mu)^2` with `mu = sqrt(1 - r^2)`.
  Coefficients must satisfy `u1, u2 >= 0` and `u1 + u2 <= 1`.
- **Trajectory**: the planet moves on a straight line at constant projected
  speed. Its sky position is `(v*(t - t0), b)`, so the center separation is
  `hypot(v*(t - t0), b)`. Time is in hours, velocity in stellar radii per
  hour. The planet radius ratio must be in `(0, 1]`; all inputs must be
  finite.
- **Blocked flux**: the overlap integral of the stellar brightness over the
  planet disk is reduced exactly to a 1-D radial integral: for each radius
  `r` from the stellar center, the arc of the circle of radius `r` lying
  inside the planet disk has length `2*phi(r)*r`, weighted by `I(r)`. The
  loss is this integral divided by the analytic unocculted flux
  `pi*(1 - u1/3 - u2/6)`. No overlap returns flux 1; partial coverage and
  central crossings are handled by the same formula.
- **Exposures**: a zero-duration exposure returns the instantaneous flux.
  Otherwise the light curve is integrated over the full exposure interval
  and divided by the duration — never sampled only at the center or
  endpoints. The interval is split at all contact times it contains, so
  exposures straddling ingress or egress are integrated correctly.
- **Contact interval**: external contact occurs when the separation equals
  `1 + radius_ratio`. The interval is reported only when its duration is
  positive; a tangent or non-overlapping trajectory yields `null`.

## Numerical integration

All quadrature uses adaptive Simpson integration with a configurable
positive tolerance (`tolerance` in the JSON parameters, default `1e-8`).
Integrands have square-root singularities at circle-intersection endpoints,
so the radial integral is split at its kink points and the tolerance is
allocated per panel. If the recursion exhausts its depth budget without
meeting the tolerance, an `IntegrationError` is raised and the CLI reports
`"converged": false` in the diagnostics — results are never silently
clamped into `[0, 1]` to hide a failed integration.

## Usage

```
python -m transit_exposure params.json exposures.csv \
    --out-csv lightcurve.csv --out-json diagnostics.json
```

`params.json`:

```json
{"radius_ratio": 0.1, "impact": 0.3, "t_closest": 5.0, "velocity": 1.0,
 "u1": 0.3, "u2": 0.2, "tolerance": 1e-9}
```

`exposures.csv` (header required):

```
t_center,duration
5.0,0.0
5.0,0.5
```

The output CSV adds `instantaneous` and `exposure_mean` columns; the JSON
diagnostics include convergence status, the total stellar flux, and the
external contact interval. See `examples/` for a runnable sample.

## Tests

```
python -m unittest discover -s tests
```

Coverage includes the uniform-brightness limit (loss equals the analytic
circle-overlap area fraction), grazing and central geometries, zero-length
and transit-straddling exposures, a long-exposure consistency check against
an independent time integration, and quadrature non-convergence reporting.
