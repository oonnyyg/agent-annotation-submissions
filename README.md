# composition_balance

Reversible balance-coordinate transform for compositional (sum-constrained)
data, using only the Python standard library. Compositions are never
compared as ordinary independent variables: all distances are computed in
log-ratio coordinates, which respect the simplex geometry.

## Usage

Forward transform (samples CSV + binary tree JSON):

```
python -m composition_balance transform \
    --samples examples/samples.csv --tree examples/tree.json \
    --delta 1e-6 \
    --preprocessed-out preprocessed.csv \
    --coords-out coordinates.csv \
    --reconstructed-out reconstructed.csv \
    --report-out report.json
```

Inverse transform (coordinates CSV back to compositions):

```
python -m composition_balance inverse \
    --coords coordinates.csv --tree examples/tree.json \
    --components a b c d --out reconstructed.csv
```

## Data model

- **Component order**: the header row of the samples CSV defines the
  component names and column order. The tree's leaves must match these
  names one-to-one (order in the tree is free; column order stays as in
  the CSV). For `inverse`, `--components` supplies that same order.
- **Preprocessing**: each row must be finite and non-negative and is
  divided by its sum; all-zero rows are rejected. Zeros are replaced by
  `--delta` (a positive number, default `1e-6`); positive values are
  scaled down proportionally so the row still sums to 1. Rows without
  zeros are left unchanged. `zeros * delta` must be strictly less than 1.
- **Tree JSON**: a leaf is a component-name string; an internal node is
  `{"left": ..., "right": ...}` with exactly two non-empty subtrees.

## Coordinates

For each internal node (emitted in **pre-order**, columns `balance_1`,
`balance_2`, ...), with `r`/`s` leaves on the left/right side and `g` the
geometric mean of a side's components:

```
balance = sqrt(r*s / (r+s)) * ln(g_left / g_right)
```

The same tree defines an orthonormal basis, so the transform is an
isometry: pairwise Euclidean distances between samples are identical in
balance coordinates and in centred log-ratio (clr) coordinates. The
report JSON lists every pair's distance computed both ways plus the
absolute difference. Swapping `left`/`right` at a node flips that
coordinate's sign but changes neither reconstructions nor distances.

## Inverse transform

`inverse` maps coordinates back through the basis transpose and a
max-shifted softmax (overflow-safe), yielding a positive composition
summing to 1. It reconstructs the **zero-replaced** composition: original
row totals are not recovered, and replacement values are not turned back
into zeros.

## Tolerances

- Roundtrip (forward then inverse) agrees with the preprocessed
  composition to about `1e-12`.
- Balance vs. clr distance differences are reported per pair and must be
  `<= --tolerance` (default `1e-9`); otherwise the command fails.

## Tests

```
python -m unittest discover -s tests
```

Covers unbalanced trees, zero replacement, sign flips under branch swap,
distance equivalence, and forward/inverse roundtrips.
