# Quality-Guided 2D Phase Unwrapping

Pure Python standard library tool that recovers region-wise continuous
phase from a wrapped phase grid, and explicitly reports adjacency
constraints that cannot be satisfied instead of smoothing over them.

## Input

Three CSV grids of identical shape:

- **phase**: wrapped phase in radians. Values are normalized on load
  into the half-open interval `[-pi, pi)` (see boundary rule below).
  Every valid pixel must be finite.
- **quality**: non-negative quality map; higher means more reliable.
  Every valid pixel must be finite and `>= 0`.
- **mask**: `0` marks a valid pixel; any non-zero value masks the pixel
  out of all computation. Masked pixels never participate in edges,
  regions, or diagnostics.

## Algorithm

1. **Edges.** Only 4-neighbour (up/down/left/right) adjacencies between
   valid pixels are considered. Edge weight is the smaller of the two
   endpoint qualities.
2. **Maximum spanning forest.** Edges are processed in descending
   weight; ties are broken by the row-major flat indices of the two
   endpoints, so the result never depends on container iteration order.
   Kruskal's algorithm yields a maximum spanning forest.
3. **Roots.** Each connected component is rooted at its highest-quality
   pixel; ties choose the row-major smallest position. The root keeps
   its normalized input phase as its unwrapped value.
4. **Propagation.** For every tree edge the principal phase difference
   is computed once, directed from the smaller flat index to the larger
   one; traversing the edge backwards uses its negation. Unwrapped
   values propagate along tree edges from the root.
5. **Cycle correspondence.** Each pixel keeps an integer cycle count
   `k` with `unwrapped = phase + 2*pi*k`.

Isolated valid pixels form their own single-pixel regions. Nothing ever
connects across masked pixels, and absolute cycle counts between
different regions are undetermined; only within-region differences are
meaningful.

## Consistency diagnostics

After unwrapping, every original adjacency edge (tree or not) is
checked: the unwrapped difference is compared against the principal
phase difference, and any non-zero integer cycle residual is reported
with both endpoint positions, the cycle count, and whether the edge is
a tree edge. A successfully built forest does not imply all constraints
are satisfiable; contradictory loops are kept in the result and flagged
in the diagnostics rather than hidden.

## Command line

```
python -m phase_unwrap --phase phase.csv --quality quality.csv --mask mask.csv \
    --out-unwrapped unwrapped.csv --out-regions regions.csv \
    --out-diagnostics diagnostics.json
```

Outputs:

- `unwrapped.csv`: unwrapped phase grid (empty cells where masked).
- `regions.csv`: integer region id per pixel (`-1` where masked).
- `diagnostics.json`: region count, root positions, conflict list with
  per-edge cycle residuals, and a `consistent` flag.

## Boundary rule

Phase normalization uses the half-open interval `[-pi, pi)`: a value of
exactly `+pi` maps to `-pi`. Principal differences therefore also lie
in `[-pi, pi)`, which keeps the cycle residual computation symmetric
and unambiguous at the wrap boundary.

## Tests

```
python -m unittest discover -s tests
```

Covers cross-wrap ramps, mask-split regions, isolated pixels,
contradictory loops, deterministic tie-breaking, and input validation.
