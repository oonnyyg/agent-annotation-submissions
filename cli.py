"""Command line interface for pedigree genotype inference.

Usage:
    python cli.py input.json [--json out.json] [--csv out.csv]
"""

from __future__ import annotations

import argparse
import csv
import json
import sys

from pedigree import GENOTYPES, PedigreeError, infer


def build_output(spec, result):
    individuals = spec["individuals"]
    rows = []
    for entry in individuals:
        iid = entry["id"]
        post = result["posterior"][iid]
        rows.append(
            {
                "id": iid,
                "observation": entry.get("observation") or "",
                "P(AA)": post["AA"],
                "P(AB)": post["AB"],
                "P(BB)": post["BB"],
            }
        )
    output = {
        "evidence_probability": result["evidence_probability"],
        "posterior": result["posterior"],
        "rows": rows,
    }
    if "offspring_prediction" in result:
        output["offspring_prediction"] = result["offspring_prediction"]
    return output


def write_csv(path, output):
    fieldnames = ["id", "observation"] + [f"P({g})" for g in GENOTYPES]
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(fh, fieldnames=fieldnames)
        writer.writeheader()
        for row in output["rows"]:
            writer.writerow(row)
        pred = output.get("offspring_prediction")
        if pred is not None:
            dist = pred["distribution"]
            writer.writerow(
                {
                    "id": "offspring(%s,%s)" % tuple(pred["parents"]),
                    "observation": "",
                    "P(AA)": dist["AA"],
                    "P(AB)": dist["AB"],
                    "P(BB)": dist["BB"],
                }
            )


def main(argv=None):
    parser = argparse.ArgumentParser(
        description=(
            "Exact genotype probability inference for a single biallelic "
            "locus on a small pedigree (teaching tool; no disease or "
            "breeding-value inference)."
        )
    )
    parser.add_argument("input", help="path to the pedigree JSON file")
    parser.add_argument("--json", dest="json_out", help="write JSON posterior to file")
    parser.add_argument("--csv", dest="csv_out", help="write CSV summary to file")
    args = parser.parse_args(argv)

    with open(args.input, "r", encoding="utf-8") as fh:
        spec = json.load(fh)

    try:
        result = infer(spec)
    except PedigreeError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    output = build_output(spec, result)

    if args.json_out:
        with open(args.json_out, "w", encoding="utf-8") as fh:
            json.dump(output, fh, indent=2, ensure_ascii=False)
            fh.write("\n")
    if args.csv_out:
        write_csv(args.csv_out, output)

    print(json.dumps(output, indent=2, ensure_ascii=False))
    return 0


if __name__ == "__main__":
    sys.exit(main())
