"""CLI: python -m composition_balance <transform|inverse> ..."""

from __future__ import annotations

import argparse
import csv
import json
import sys

from .core import (
    BalanceError,
    build_basis,
    clr,
    forward,
    inverse,
    pairwise_euclidean,
    parse_tree,
    replace_zeros,
    validate_and_close,
)


def _read_samples(path):
    with open(path, newline="", encoding="utf-8") as fh:
        rows = list(csv.reader(fh))
    if not rows:
        raise BalanceError("samples CSV is empty")
    header = rows[0]
    if not header or any(not h for h in header):
        raise BalanceError("samples CSV header must name every component")
    return header, rows[1:]


def _write_csv(path, header, rows):
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(header)
        writer.writerows(rows)


def _load_tree(path, names):
    with open(path, encoding="utf-8") as fh:
        data = json.load(fh)
    return parse_tree(data, names)


def cmd_transform(args):
    names, raw_rows = _read_samples(args.samples)
    root = _load_tree(args.tree, names)
    if not raw_rows:
        raise BalanceError("samples CSV has no data rows")

    closed = [validate_and_close(row, names) for row in raw_rows]
    preprocessed = [replace_zeros(row, args.delta) for row in closed]
    coords = [forward(row, root, names) for row in preprocessed]
    reconstructed = [inverse(c, root, names) for c in coords]

    coord_names = [f"balance_{k + 1}" for k in range(len(coords[0]))]
    _write_csv(args.preprocessed_out, names, preprocessed)
    _write_csv(args.coords_out, coord_names, coords)
    _write_csv(args.reconstructed_out, names, reconstructed)

    balance_dist = pairwise_euclidean(coords)
    clr_dist = pairwise_euclidean([clr(row) for row in preprocessed])
    pairs = []
    max_diff = 0.0
    for (i, j, db), (_, _, dc) in zip(balance_dist, clr_dist):
        diff = abs(db - dc)
        max_diff = max(max_diff, diff)
        pairs.append(
            {"sample_i": i, "sample_j": j,
             "balance_distance": db, "clr_distance": dc, "abs_diff": diff}
        )

    report = {
        "n_samples": len(preprocessed),
        "n_components": len(names),
        "components": names,
        "coordinates": coord_names,
        "delta": args.delta,
        "pairwise_distances": pairs,
        "max_balance_clr_distance_diff": max_diff,
        "distance_tolerance": args.tolerance,
        "distances_match": max_diff <= args.tolerance,
    }
    with open(args.report_out, "w", encoding="utf-8") as fh:
        json.dump(report, fh, indent=2)
        fh.write("\n")
    if max_diff > args.tolerance:
        raise BalanceError(
            f"balance/clr distance mismatch {max_diff} > {args.tolerance}"
        )


def cmd_inverse(args):
    with open(args.coords, newline="", encoding="utf-8") as fh:
        rows = list(csv.reader(fh))
    if len(rows) < 2:
        raise BalanceError("coordinates CSV needs a header and one data row")
    coords = [[float(v) for v in row] for row in rows[1:]]
    names = args.components
    root = _load_tree(args.tree, names)
    reconstructed = [inverse(c, root, names) for c in coords]
    _write_csv(args.out, names, reconstructed)


def main(argv=None):
    parser = argparse.ArgumentParser(prog="composition_balance")
    sub = parser.add_subparsers(dest="command", required=True)

    t = sub.add_parser("transform", help="forward transform of sample CSV")
    t.add_argument("--samples", required=True)
    t.add_argument("--tree", required=True)
    t.add_argument("--delta", type=float, default=1e-6,
                   help="positive replacement value for zeros")
    t.add_argument("--tolerance", type=float, default=1e-9)
    t.add_argument("--preprocessed-out", default="preprocessed.csv")
    t.add_argument("--coords-out", default="coordinates.csv")
    t.add_argument("--reconstructed-out", default="reconstructed.csv")
    t.add_argument("--report-out", default="report.json")
    t.set_defaults(func=cmd_transform)

    i = sub.add_parser("inverse", help="reconstruct compositions from coordinates")
    i.add_argument("--coords", required=True)
    i.add_argument("--tree", required=True)
    i.add_argument("--components", required=True, nargs="+",
                   help="component names in CSV column order")
    i.add_argument("--out", default="reconstructed.csv")
    i.set_defaults(func=cmd_inverse)

    args = parser.parse_args(argv)
    try:
        args.func(args)
    except BalanceError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
