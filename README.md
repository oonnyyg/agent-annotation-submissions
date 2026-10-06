# sam-target-coverage

Sequencing alignment QC tool that computes per-target coverage using only
**truly aligned reference bases** (CIGAR `M`/`=`/`X`). Deletions (`D`) and
intron skips (`N`) advance the reference cursor without contributing
coverage, so gaps and introns are never miscounted as covered.

Pure Python standard library. No samtools, no BAM — SAM text input only.

## Usage

```
python -m sam_coverage --sam in.sam --bed targets.bed --config config.json \
    --out-csv targets.csv --out-summary summary.json
```

- `--sam`: SAM file with `@SQ` headers (reference lengths are taken from
  `SN`/`LN` tags).
- `--bed`: BED4 target file: `chrom`, `start`, `end`, `name`.
- `--config`: JSON object, currently one key: `{"min_mapq": 20}`
  (default 0).
- `--out-csv`: per-target CSV (mean depth, fraction at >=1x, depth
  histogram as `depth:bases;...`).
- `--out-summary`: JSON with splice-junction tallies and per-reason
  filtering counts.

## Coordinate conventions

- Targets use **0-based, half-open** `[start, end)` BED coordinates.
- SAM `POS` is 1-based; it is converted to the same 0-based half-open
  system internally.
- Reverse-strand alignments are used as-is; coordinates are **not**
  re-reversed.
- Targets and retained records referencing an unknown reference, or
  running past the reference length, are rejected. Target names must be
  unique; targets may overlap and are tallied independently.

## Filtering order

Records are dropped in this order (first matching reason is counted in
the summary):

1. `unmapped` — FLAG 0x4
2. `not_primary` — secondary (0x100) or supplementary (0x800)
3. `qc_fail` — FLAG 0x200
4. `duplicate` — FLAG 0x400
5. `low_mapq` — MAPQ below `min_mapq`
6. `unknown_reference` — RNAME not in the SAM header
7. `out_of_bounds` / `cigar_error` — alignment exceeds the reference or
   CIGAR query consumption does not match the SEQ length

Each retained record contributes independently: paired-end overlaps are
not merged, and mates are counted separately.

## CIGAR semantics

- `M`, `=`, `X`: add +1 depth per reference base.
- `D`, `N`: advance the reference position only (no coverage).
- `I`, `S`: consume query sequence only.
- `H`, `P`: consume neither.

Query consumption (`M`/`I`/`S`/`=`/`X`) is checked against the SEQ length.

## Coverage computation

Per target, aligned blocks are clipped to the target interval and applied
as +1/-1 sweep-line events, so memory is proportional to the number of
contributing records — never to chromosome length. Zero-depth bases are
included in the depth histogram. Outputs per target: mean depth, fraction
of bases at depth >= 1, and the full depth histogram.

## Splice junctions

`N` operations are tallied separately as junction support counts per
`(reference, start, end)` skipped interval — never confused with `D`, and
no transcript strand is inferred.

## Tests

```
python -m unittest discover -s tests
```
