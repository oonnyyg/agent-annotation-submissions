"""Command line interface: solve equilibria and inverse dosing problems.

Usage:
    python -m competitive_binding solve sites.json mixtures.csv \
        --csv-out results.csv --report report.json
    python -m competitive_binding inverse sites.json --class NAME \
        --target 0.5 --total-b 20 --limit-a 1000 --report inverse.json
"""

from __future__ import annotations

import argparse
import csv
import json
import sys

from .inverse import solve_inverse
from .solver import SiteClass, SolverError, solve_equilibrium


def load_sites(path):
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    entries = data.get("site_classes") if isinstance(data, dict) else data
    if not isinstance(entries, list) or not entries:
        raise ValueError("sites JSON must contain a non-empty 'site_classes' list")
    sites = []
    for i, entry in enumerate(entries):
        try:
            sites.append(
                SiteClass(
                    name=str(entry.get("name", f"class_{i}")),
                    total=entry["total"],
                    kd_a=entry["kd_a"],
                    kd_b=entry["kd_b"],
                )
            )
        except KeyError as exc:
            raise ValueError(f"site class #{i} missing key {exc}") from exc
    return sites


def load_mixtures(path):
    with open(path, "r", encoding="utf-8", newline="") as fh:
        rows = list(csv.DictReader(fh))
    if not rows:
        raise ValueError("mixtures CSV is empty")
    mixtures = []
    for i, row in enumerate(rows):
        try:
            total_a = float(row["total_a"])
            total_b = float(row["total_b"])
        except (KeyError, TypeError, ValueError) as exc:
            raise ValueError(
                f"mixtures CSV row {i + 1}: need numeric 'total_a' and "
                f"'total_b' columns ({exc})"
            ) from exc
        mixtures.append((total_a, total_b))
    return mixtures


def _budget(site_results, result):
    return {
        "total_a_nM": result.total_a,
        "total_b_nM": result.total_b,
        "free_a_nM": result.free_a,
        "free_b_nM": result.free_b,
        "bound_a_nM": sum(s.bound_a for s in site_results),
        "bound_b_nM": sum(s.bound_b for s in site_results),
        "residual_a_nM": result.residual_a,
        "residual_b_nM": result.residual_b,
        "iterations": result.iterations,
        "sites": [
            {
                "name": s.name,
                "total_nM": s.total,
                "empty_nM": s.empty,
                "bound_a_nM": s.bound_a,
                "bound_b_nM": s.bound_b,
                "occupancy_a": s.occupancy_a,
                "occupancy_b": s.occupancy_b,
            }
            for s in site_results
        ],
    }


def cmd_solve(args):
    sites = load_sites(args.sites)
    mixtures = load_mixtures(args.mixtures)
    results = []
    for total_a, total_b in mixtures:
        results.append(solve_equilibrium(sites, total_a, total_b, tol=args.tol))

    if args.csv_out:
        with open(args.csv_out, "w", encoding="utf-8", newline="") as fh:
            writer = csv.writer(fh)
            writer.writerow([
                "mixture", "site", "total_a_nM", "total_b_nM",
                "free_a_nM", "free_b_nM", "site_total_nM", "empty_nM",
                "bound_a_nM", "bound_b_nM", "occupancy_a", "occupancy_b",
            ])
            for idx, res in enumerate(results):
                for s in res.sites:
                    writer.writerow([
                        idx, s.name, res.total_a, res.total_b,
                        res.free_a, res.free_b, s.total, s.empty,
                        s.bound_a, s.bound_b, s.occupancy_a, s.occupancy_b,
                    ])

    report = {
        "units": "nM",
        "tolerance": args.tol,
        "mixtures": [_budget(res.sites, res) for res in results],
    }
    _emit(report, args.report)
    return 0


def cmd_inverse(args):
    sites = load_sites(args.sites)
    outcome = solve_inverse(
        sites,
        target_class=args.site_class,
        target_occupancy=args.target,
        fixed_total_b=args.total_b,
        limit_a=args.limit_a,
        tol=args.tol,
    )
    report = {
        "units": "nM",
        "success": outcome.success,
        "target_class": outcome.target_class,
        "target_occupancy": outcome.target_occupancy,
        "fixed_total_b_nM": outcome.fixed_total_b,
        "limit_a_nM": outcome.limit_a,
        "total_a_nM": outcome.total_a,
        "achieved_occupancy": outcome.achieved_occupancy,
        "iterations": outcome.iterations,
        "message": outcome.message,
    }
    if outcome.equilibrium is not None:
        report["equilibrium"] = _budget(
            outcome.equilibrium.sites, outcome.equilibrium
        )
    _emit(report, args.report)
    return 0 if outcome.success else 1


def _emit(report, path):
    text = json.dumps(report, indent=2)
    if path:
        with open(path, "w", encoding="utf-8") as fh:
            fh.write(text + "\n")
    else:
        print(text)


def build_parser():
    parser = argparse.ArgumentParser(
        prog="competitive_binding",
        description="Two-ligand competitive binding equilibrium (nM units).",
    )
    sub = parser.add_subparsers(dest="command", required=True)

    p_solve = sub.add_parser("solve", help="solve equilibria for CSV mixtures")
    p_solve.add_argument("sites", help="JSON file with site classes")
    p_solve.add_argument("mixtures", help="CSV with total_a,total_b columns")
    p_solve.add_argument("--csv-out", help="write per-site results CSV here")
    p_solve.add_argument("--report", help="write JSON budget report here")
    p_solve.add_argument("--tol", type=float, default=1e-10)
    p_solve.set_defaults(func=cmd_solve)

    p_inv = sub.add_parser("inverse", help="minimum total A for target occupancy")
    p_inv.add_argument("sites", help="JSON file with site classes")
    p_inv.add_argument("--class", dest="site_class", required=True,
                       help="name of the target site class")
    p_inv.add_argument("--target", type=float, required=True,
                       help="target occupancy of ligand A in [0, 1)")
    p_inv.add_argument("--total-b", type=float, required=True,
                       help="fixed total concentration of ligand B (nM)")
    p_inv.add_argument("--limit-a", type=float, required=True,
                       help="upper limit for total A (nM)")
    p_inv.add_argument("--report", help="write JSON report here")
    p_inv.add_argument("--tol", type=float, default=1e-10)
    p_inv.set_defaults(func=cmd_inverse)
    return parser


def main(argv=None):
    args = build_parser().parse_args(argv)
    try:
        return args.func(args)
    except SolverError as exc:
        print(f"solver failed: {exc}", file=sys.stderr)
        return 2
    except (ValueError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
