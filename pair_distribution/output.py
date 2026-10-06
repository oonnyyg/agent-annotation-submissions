"""CSV and summary JSON writers."""

import csv
import json


def write_csv(path, results, n_bins, r_max):
    dr = r_max / n_bins
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow([
            "pair_a", "pair_b", "frame", "bin", "r_lo", "r_hi",
            "count", "g_r", "coordination",
        ])
        for res in results:
            a, b = res["pair"]
            for k in range(n_bins):
                lo, hi = dr * k, dr * (k + 1)
                writer.writerow([
                    a, b, "mean", k, f"{lo:.12g}", f"{hi:.12g}",
                    res["total_hist"][k],
                    f"{res['avg_g'][k]:.12g}",
                    f"{res['avg_cn'][k]:.12g}",
                ])
            for fi, pf in enumerate(res["per_frame"]):
                for k in range(n_bins):
                    lo, hi = dr * k, dr * (k + 1)
                    writer.writerow([
                        a, b, fi, k, f"{lo:.12g}", f"{hi:.12g}",
                        pf["hist"][k],
                        f"{pf['g'][k]:.12g}",
                        f"{pf['cn'][k]:.12g}",
                    ])


def write_summary(path, cfg, frames, results, r_max_limit):
    species = sorted({t for f in frames for t in f.types})
    pairs_summary = []
    for res in results:
        a, b = res["pair"]
        empty_frames = sum(1 for pf in res["per_frame"] if pf["empty"])
        pairs_summary.append({
            "pair": [a, b],
            "same_species": a == b,
            "total_counts": res["total_hist"],
            "empty_frames": empty_frames,
            "empty_reason": (
                "species missing or fewer than two same-species particles"
                if empty_frames else None
            ),
        })
    summary = {
        "n_frames": len(frames),
        "n_particles": len(frames[0].ids),
        "species": species,
        "r_max": cfg["r_max"],
        "n_bins": cfg["n_bins"],
        "r_max_limit_half_shortest_edge": r_max_limit,
        "normalization": {
            "rdf": "per-frame: count / (n_centers * target_density * "
                   "shell_volume); frames averaged with equal weight",
            "target_density": "N_target / V_frame for cross pairs; "
                              "(N - 1) / V_frame for same-species pairs",
            "coordination": "per-frame cumulative count / n_centers, "
                            "averaged over frames",
        },
        "pairs": pairs_summary,
    }
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(summary, fh, indent=2, ensure_ascii=False)
        fh.write("\n")
