# Parallel-Beam Reconstruction

A pure-standard-library Python tool that reconstructs a 2D numeric
matrix from synthetic parallel-beam projection data using filtered
backprojection (FBP). It exists to validate the FBP pipeline on
synthetic data; it does not process clinical images.

## Usage

```
python main.py --projections projections.csv --config config.json \
    --output reconstruction.csv --summary summary.json
```

### Input: projections CSV

One row per view angle: the first column is the angle in degrees, the
remaining columns are the equally spaced detector readings. Angles must
uniformly cover `[0, 180)` degrees (the right endpoint is excluded).
Missing angles, duplicate angles, angles outside `[0, 180)`, or rows
with inconsistent detector counts are rejected with a clear error.

### Input: config JSON

```json
{
  "detector_spacing": 0.5,
  "image": {"nx": 32, "ny": 32, "pixel_size": 0.25}
}
```

### Output

- `reconstruction.csv`: the reconstructed matrix, one row per `y`.
- `summary.json`: coordinate ranges, sampling spacings, angle coverage,
  filter configuration, and min/max statistics.

## Geometry

Both the detector array and the image grid are centered on the
geometric origin: index `i` maps to coordinate
`(i - (n - 1) / 2) * spacing`. Even-sized grids are **not** shifted by
half a sample, so an even grid has no sample exactly at zero.

## Fourier convention and ramp filter

- DFT pair: `X[k] = sum_n x[n] exp(-2j pi k n / N)` and
  `x[n] = (1/N) sum_k X[k] exp(+2j pi k n / N)`.
- Each projection is zero-padded to at least twice its length (rounded
  up to a power of two) before the DFT to reduce circular-convolution
  wrap-around.
- Bin `k` corresponds to frequency `f_k = k / (N * spacing)` in
  **cycles per unit length**; bins above `N/2` are negative
  frequencies.
- The filter multiplier is `H(f) = |f|` (pure ramp). After the inverse
  DFT the result is cropped back to the original detector positions.
  No normalization of the maximum (or any other rescaling) is applied.

## Backprojection

Each pixel center `(x, y)` is projected onto the detector coordinate
`s = x cos(theta) + y sin(theta)`. The filtered projection is linearly
interpolated at `s`; positions outside the detector range contribute
zero. Contributions are summed over all angles and multiplied by the
angular step in radians (`pi / n_angles`). Negative values are kept
(the ramp filter produces negative sidelobes; they are not clipped).

## Limits

- At most 64 angles and 64 detector readings per projection.
- At most a 64 x 64 image grid.
- Exceeding a limit aborts with an explicit error message.

## Discretization error

This is a finite, discrete approximation of the continuous FBP:

- The ramp is band-limited to the Nyquist frequency
  `1 / (2 * spacing)`, so the discrete kernel deviates slightly from
  the continuous `h(s)` away from the center tap.
- Zero-padding reduces but does not eliminate wrap-around; sharp
  projection edges produce small ringing.
- Linear interpolation in backprojection and the finite angular step
  add smoothing; using more angles and finer detectors reduces error.

For the analytic disk phantom (`fbp.disk_sinogram`, unit amplitude,
radius 2, 60 angles, 32 detectors at spacing 0.5) the reconstructed
center value is about 0.98 (exact: 1.0), with small negative values
(about -0.03) preserved outside the disk.

## Tests

```
python -m unittest test_fbp -v
```

The suite covers angle-grid validation (missing/duplicate/endpoint
angles), the centered (non-half-shifted) geometry, the ramp filter
kernel against its analytic values, linearity (no max normalization),
reconstruction amplitude scale and direction on the analytic disk
phantom, center symmetry of the reconstruction, preservation of
negative values, and the end-to-end CLI.
