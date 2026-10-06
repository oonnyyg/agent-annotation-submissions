"""Command line interface: python -m composition_balance ..."""

from __future__ import annotations

import argparse
import csv
import json
import sys

from . import core

DEFAULT_DELTA = 1e-6


def _read_samples(path: str) -> tuple[list[str], list[str], list[list[float]]]:
    with open(path, newline="", encoding="utf-8") as handle:
        reader = csv.reader(handle)
        rows = [row for row in reader if row]
    if len(rows) < 2:
        raise core.CompositionError("samples CSV needs a header and at least one data row")
    header = rows[0]
    if header and header[0].strip().lower() in {"sample", "id", "sample_id", "name"}:
        names, ids, data = header[1:], [], []
        for line_no, row in enumerate(rows[1:], start=2):
            if len(row) != len(header):
                raise core.CompositionError(f"line {line_no}: expected {len(header)} fields")
            ids.append(row[0])
            data.append(_parse_floats(row[1:], line_no))
    else:
        names, ids, data = header, [f"sample_{i}" for i in range(len(rows) - 1)], []
        for line_no, row in enumerate(rows[1:], start=2):
            if len(row) != len(header):
                raise core.CompositionError(f"line {line_no}: expected {len(header)} fields")
            data.append(_parse_floats(row, line_no))
    return names, ids, data


def _parse_floats(fields: list[str], line_no: int) -> list[float]:
    values = []
    for field in fields:
        try:
            values.append(float(field))
        except ValueError:
            raise core.CompositionError(f"line {line_no}: not a number: {field!r}") from None
    return values


def _load_tree(path: str, names: list[str]) -> core.Node:
    with open(path, encoding="utf-8") as handle:
        data = json.load(handle)
    return core.parse_tree(data, names)


def _write_csv(path: str, header: list[str], ids: list[str], rows: list[list[float]]) -> None:
    with open(path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(["sample"] + header)
        for sample_id, row in zip(ids, rows):
            writer.writerow([sample_id] + [f"{value:.17g}" for value in row])


def _read_coordinates(path: str) -> tuple[list[str], list[list[float]]]:
    with open(path, newline="", encoding="utf-8") as handle:
        rows = [row for row in csv.reader(handle) if row]
    if len(rows) < 2:
        raise core.CompositionError("coordinates CSV needs a header and at least one data row")
    ids, data = [], []
    for line_no, row in enumerate(rows[1:], start=2):
        ids.append(row[0])
        data.append(_parse_floats(row[1:], line_no))
    return ids, data


def _cmd_transform(args: argparse.Namespace) -> int:
    names, ids, raw = _read_samples(args.samples)
    root = _load_tree(args.tree, names)
    closed = core.close_rows(raw)
    adjusted, zero_counts = core.replace_zeros(closed, args.delta)
    coords = core.balance_coordinates(adjusted, root, names)
    rebuilt = core.inverse_balances(coords, root, names)

    nodes = root.internal_preorder()
    balance_names = [
        f"balance_{i}:{'+'.join(node.left.leaves())}|{'+'.join(node.right.leaves())}"
        for i, node in enumerate(nodes)
    ]
    _write_csv(args.preprocessed_out, names, ids, adjusted)
    _write_csv(args.coordinates_out, balance_names, ids, coords)
    _write_csv(args.reconstructed_out, names, ids, rebuilt)

    balance_dist = core.pairwise_distances(coords)
    clr_dist = core.pairwise_distances(core.clr_coordinates(adjusted))
    dist_diff = max(
        (abs(a - b) for ra, rb in zip(balance_dist, clr_dist) for a, b in zip(ra, rb)),
        default=0.0,
    )
    recon_err = max(
        (abs(a - b) for ra, rb in zip(adjusted, rebuilt) for a, b in zip(ra, rb)),
        default=0.0,
    )
    report = {
        "samples": len(ids),
        "components": names,
        "delta": args.delta,
        "zero_replacements_per_sample": dict(zip(ids, zero_counts)),
        "total_zero_replacements": sum(zero_counts),
        "balance_order": balance_names,
        "max_reconstruction_error": recon_err,
        "max_balance_vs_clr_distance_difference": dist_diff,
        "pairwise_balance_distances": {
            "sample_ids": ids,
            "matrix": balance_dist,
        },
    }
    with open(args.report_out, "w", encoding="utf-8") as handle:
        json.dump(report, handle, indent=2)
        handle.write("\n")
    print(
        f"transformed {len(ids)} samples x {len(names)} components; "
        f"max reconstruction error {recon_err:.3e}; "
        f"max balance-vs-clr distance difference {dist_diff:.3e}"
    )
    return 0


def _cmd_inverse(args: argparse.Namespace) -> int:
    with open(args.tree, encoding="utf-8") as handle:
        tree_data = json.load(handle)
    names = _leaf_names(tree_data)
    root = core.parse_tree(tree_data, names)
    ids, coords = _read_coordinates(args.coordinates)
    rebuilt = core.inverse_balances(coords, root, names)
    _write_csv(args.out, names, ids, rebuilt)
    print(f"reconstructed {len(ids)} compositions into {args.out}")
    return 0


def _leaf_names(data: object) -> list[str]:
    if isinstance(data, str):
        return [data]
    if isinstance(data, dict):
        if "name" in data:
            return _leaf_names(data["name"])
        return _leaf_names(data["left"]) + _leaf_names(data["right"])
    raise core.TreeError(f"unsupported tree node {data!r}")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="composition_balance",
        description="Reversible balance (ILR) transform for compositional data.",
    )
    sub = parser.add_subparsers(dest="command", required=True)

    t = sub.add_parser("transform", help="forward transform with zero replacement")
    t.add_argument("--samples", required=True, help="input samples CSV")
    t.add_argument("--tree", required=True, help="binary partition tree JSON")
    t.add_argument("--delta", type=float, default=DEFAULT_DELTA,
                   help="zero replacement value (default: %(default)s)")
    t.add_argument("--preprocessed-out", default="preprocessed.csv")
    t.add_argument("--coordinates-out", default="balances.csv")
    t.add_argument("--reconstructed-out", default="reconstructed.csv")
    t.add_argument("--report-out", default="quality_report.json")
    t.set_defaults(func=_cmd_transform)

    i = sub.add_parser("inverse", help="reconstruct compositions from coordinates")
    i.add_argument("--coordinates", required=True, help="balance coordinates CSV")
    i.add_argument("--tree", required=True, help="binary partition tree JSON")
    i.add_argument("--out", default="inverse.csv")
    i.set_defaults(func=_cmd_inverse)

    args = parser.parse_args(argv)
    try:
        return args.func(args)
    except (core.CompositionError, core.TreeError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
