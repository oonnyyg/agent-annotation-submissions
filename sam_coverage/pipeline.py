"""End-to-end pipeline: read inputs, filter, tally, emit reports."""

from __future__ import annotations

import csv
import json

from .core import CoverageAccumulator, JunctionTally, RecordFilter, walk_cigar
from .io import InputError, load_config, load_targets, parse_sam


def run(sam_path, bed_path, config_path, csv_path, summary_path):
    config = load_config(config_path)
    ref_lengths, records = parse_sam(sam_path)
    if not ref_lengths:
        raise InputError("SAM header contains no @SQ lines")
    targets = load_targets(bed_path, ref_lengths)

    filt = RecordFilter(config.min_mapq, ref_lengths)
    accumulators = {t.name: CoverageAccumulator(t) for t in targets}
    by_rname: dict[str, list[str]] = {}
    for t in targets:
        by_rname.setdefault(t.rname, []).append(t.name)
    junctions = JunctionTally()

    for rec in records:
        if filt.check(rec) is not None:
            continue
        try:
            aligned, skipped = walk_cigar(rec, ref_lengths)
        except InputError as exc:
            msg = str(exc)
            reason = "out_of_bounds" if "runs past" in msg else "cigar_error"
            filt.reject(reason)
            continue
        for name in by_rname.get(rec.rname, ()):
            accumulators[name].add_blocks(aligned)
        junctions.add(rec.rname, skipped)

    summaries = [accumulators[t.name].summarize() for t in targets]
    _write_csv(csv_path, summaries)
    summary = {
        "config": {"min_mapq": config.min_mapq},
        "records": {
            "total": filt.total,
            "kept": filt.kept,
            "filtered": filt.counts,
        },
        "junctions": junctions.rows(),
    }
    with open(summary_path, "w", encoding="utf-8") as fh:
        json.dump(summary, fh, indent=2)
        fh.write("\n")
    return summary


def _write_csv(path, summaries):
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow([
            "target", "rname", "start", "end", "length",
            "mean_depth", "covered_fraction", "depth_histogram",
        ])
        for s in summaries:
            t = s["target"]
            hist = ";".join(
                f"{depth}:{s['histogram'][depth]}"
                for depth in sorted(s["histogram"])
            )
            writer.writerow([
                t.name, t.rname, t.start, t.end, s["length"],
                f"{s['mean_depth']:.6f}", f"{s['covered_fraction']:.6f}", hist,
            ])
