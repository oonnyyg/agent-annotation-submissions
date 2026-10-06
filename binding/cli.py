"""Command line interface: solve and invert subcommands."""
from __future__ import annotations

import argparse
import json
import sys

from .inverse import invert_dose
from .io import load_experiments, load_sites, result_report, write_results_csv
from .solver import ConvergenceError, solve_equilibrium


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="binding",
        description="Competitive binding equilibrium of two ligands (nM).",
    )
    sub = parser.add_subparsers(dest="command", required=True)
    solve = sub.add_parser("solve", help="solve equilibrium for CSV dose rows")
    solve.add_argument("sites", help="JSON file with site classes")
    solve.add_argument("experiments", help="CSV with total_a,total_b columns")
    solve.add_argument("--out", help="write per-class results CSV here")
    solve.add_argument("--report", help="write JSON mass-balance report here")
    invert = sub.add_parser(
        "invert", help="minimal total_a for target occupancy of a class"
    )
    invert.add_argument("sites", help="JSON file with site classes")
    invert.add_argument("--class", dest="target_class", required=True)
    invert.add_argument("--target", type=float, required=True,
                        help="target occupancy of ligand A in [0, 1)")
    invert.add_argument("--total-b", type=float, required=True)
    invert.add_argument("--limit", type=float, required=True,
                        help="upper bound on total_a, nM")
    invert.add_argument("--report", help="write JSON report here")
    return parser


def main(argv=None) -> int:
    args = build_parser().parse_args(argv)
    try:
        if args.command == "solve":
            return _cmd_solve(args)
        return _cmd_invert(args)
    except ConvergenceError as exc:
        print(
            f"error: equilibrium did not converge: {exc} "
            f"(residuals R_a={exc.residuals[0]:.6e}, "
            f"R_b={exc.residuals[1]:.6e} nM)",
            file=sys.stderr,
        )
        return 2
    except (ValueError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2


def _cmd_solve(args) -> int:
    sites = load_sites(args.sites)
    experiments = load_experiments(args.experiments)
    results = [solve_equilibrium(sites, e) for e in experiments]
    for r in results:
        print(
            f"total_a={r.experiment.total_a:g} total_b={r.experiment.total_b:g} "
            f"-> free_a={r.free_a:.6g} free_b={r.free_b:.6g} nM "
            f"(iters={r.iterations})"
        )
        for c in r.classes:
            print(
                f"  {c.name}: free={c.frac_free:.6g} occ_a={c.occ_a:.6g} "
                f"occ_b={c.occ_b:.6g} bound_a={c.bound_a:.6g} "
                f"bound_b={c.bound_b:.6g} nM"
            )
    if args.out:
        write_results_csv(args.out, results)
        print(f"wrote {args.out}")
    if args.report:
        report = {"experiments": [result_report(r) for r in results]}
        with open(args.report, "w", encoding="utf-8") as fh:
            json.dump(report, fh, indent=2)
        print(f"wrote {args.report}")
    return 0


def _cmd_invert(args) -> int:
    sites = load_sites(args.sites)
    res = invert_dose(
        sites, args.target_class, args.target, args.total_b, args.limit
    )
    print(res.message)
    report = {
        "target_class": res.target_class,
        "target_occupancy": res.target_occupancy,
        "total_b": res.total_b,
        "limit_a": res.limit_a,
        "achieved": res.achieved,
        "total_a": res.total_a,
        "occupancy_at_solution": res.occupancy_at_solution,
        "occupancy_at_limit": res.occupancy_at_limit,
    }
    if res.equilibrium is not None:
        report["equilibrium"] = result_report(res.equilibrium)
    if args.report:
        with open(args.report, "w", encoding="utf-8") as fh:
            json.dump(report, fh, indent=2)
        print(f"wrote {args.report}")
    else:
        print(json.dumps(report, indent=2))
    return 0 if res.achieved else 1


if __name__ == "__main__":
    sys.exit(main())
