"""Command-line interface: JSON posterior output and CSV summary."""
from __future__ import annotations

import argparse
import csv
import json
import sys

from .core import GENOTYPES, PedigreeError, infer, load_pedigree, predict_offspring


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="pedigree-infer",
        description=(
            "Exact genotype inference for a single biallelic locus on a "
            "small pedigree (teaching tool; no disease/breeding-value claims)."
        ),
    )
    parser.add_argument("pedigree_json", help="path to the pedigree JSON file")
    parser.add_argument(
        "--predict",
        nargs=2,
        metavar=("FATHER", "MOTHER"),
        help="predict genotype distribution of a hypothetical offspring",
    )
    parser.add_argument(
        "--csv",
        metavar="PATH",
        help="also write a CSV summary of posterior distributions",
    )
    return parser


def run(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        with open(args.pedigree_json, encoding="utf-8") as fh:
            pedigree = load_pedigree(json.load(fh))
        result = infer(pedigree)
        prediction = None
        if args.predict:
            prediction = predict_offspring(
                pedigree, result, args.predict[0], args.predict[1]
            )
    except (PedigreeError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    output = {
        "evidence_probability": result.evidence,
        "contradictory_observations": result.contradictory,
        "posterior": result.posterior,
    }
    if result.contradictory:
        output["warning"] = (
            "observations are mutually contradictory under the model; "
            "posteriors are not reported"
        )
    if args.predict:
        output["offspring_prediction"] = {
            "parents": list(args.predict),
            "distribution": prediction,
        }
    json.dump(output, sys.stdout, indent=2, ensure_ascii=False)
    print()

    if args.csv:
        _write_csv(args.csv, result.posterior, args.predict, prediction)
    return 0


def _write_csv(path, posterior, predict, prediction) -> None:
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["individual", *[f"P({g})" for g in GENOTYPES]])
        for name, dist in posterior.items():
            writer.writerow([name, *[f"{dist[g]:.6f}" for g in GENOTYPES]])
        if predict and prediction:
            writer.writerow(
                [
                    f"offspring({predict[0]}x{predict[1]})",
                    *[f"{prediction[g]:.6f}" for g in GENOTYPES],
                ]
            )


if __name__ == "__main__":
    raise SystemExit(run())
