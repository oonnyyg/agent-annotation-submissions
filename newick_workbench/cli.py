"""Command line interface for the Newick distance workbench."""

from __future__ import annotations

import argparse
import sys

from .emit import matrix_csv, to_newick
from .ops import distance_matrix, midpoint_root, prune
from .parser import ParseError, parse_tree


def build_parser():
    parser = argparse.ArgumentParser(
        prog="newick-workbench",
        description=(
            "Filter leaves of a Newick tree, midpoint-root it, and export "
            "a leaf distance matrix. Distances are preserved exactly; no "
            "phylogenetic inference is performed."
        ),
    )
    parser.add_argument("tree", help="input Newick file (single tree)")
    parser.add_argument(
        "--keep",
        nargs="+",
        metavar="LEAF",
        help="leaf names to keep (at least two); others are pruned",
    )
    parser.add_argument(
        "--midpoint",
        action="store_true",
        help="midpoint-root the (pruned) tree before export",
    )
    parser.add_argument("--newick", metavar="PATH", help="write Newick here")
    parser.add_argument("--csv", metavar="PATH", help="write CSV distance matrix here")
    parser.add_argument(
        "--report",
        metavar="PATH",
        help="write removed internal labels here (default: stderr)",
    )
    return parser


def main(argv=None):
    args = build_parser().parse_args(argv)
    with open(args.tree, encoding="utf-8-sig") as handle:
        text = handle.read()
    try:
        root = parse_tree(text)
    except ParseError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    removed = []
    if args.keep:
        try:
            root, removed = prune(root, args.keep)
        except ValueError as exc:
            print(f"error: {exc}", file=sys.stderr)
            return 2
    if args.midpoint:
        root = midpoint_root(root)

    if removed:
        lines = ["removed internal labels:"] + [f"  {name}" for name in removed]
        report = "\n".join(lines) + "\n"
        if args.report:
            with open(args.report, "w", encoding="utf-8") as handle:
                handle.write(report)
        else:
            sys.stderr.write(report)

    newick = to_newick(root) + "\n"
    if args.newick:
        with open(args.newick, "w", encoding="utf-8") as handle:
            handle.write(newick)
    else:
        sys.stdout.write(newick)

    names, matrix = distance_matrix(root)
    if args.csv:
        with open(args.csv, "w", encoding="utf-8", newline="") as handle:
            handle.write(matrix_csv(names, matrix))
    return 0


if __name__ == "__main__":
    sys.exit(main())
