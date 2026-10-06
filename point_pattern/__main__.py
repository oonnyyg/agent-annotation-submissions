"""Command line interface: python -m point_pattern POINTS.csv CONFIG.json [OUT_DIR].

Writes ``kl_curves.csv`` and ``simulation_test.json`` into OUT_DIR
(default: current directory).  No plots are generated.
"""

from __future__ import annotations

import csv
import json
import os
import sys

from .core import (
    k_estimate,
    l_from_k,
    ordered_pair_distances,
    validate_points,
)
from .io_utils import read_config, read_points_csv
from .simulate import (
    deviation_statistic,
    envelope,
    simulate_l_curves,
    upper_tail_pvalue,
)


def main(argv=None):
    args = list(sys.argv[1:] if argv is None else argv)
    if len(args) not in (2, 3):
        print(
            "usage: python -m point_pattern POINTS.csv CONFIG.json [OUT_DIR]",
            file=sys.stderr,
        )
        return 2
    out_dir = args[2] if len(args) == 3 else "."
    os.makedirs(out_dir, exist_ok=True)

    cfg = read_config(args[1])
    window = cfg["window"]
    radii = cfg["radii"]
    points = validate_points(read_points_csv(args[0]), window)

    pairs = ordered_pair_distances(points)
    counts, k_corr = k_estimate(window, pairs, radii, correct=True)
    _, k_raw = k_estimate(window, pairs, radii, correct=False)
    l_obs = l_from_k(k_corr)

    curves = simulate_l_curves(
        window, radii, len(points), cfg["simulations"], cfg["seed"]
    )
    sim_stats = [deviation_statistic(c, radii) for c in curves]
    obs_stat = deviation_statistic(l_obs, radii)
    p_value = upper_tail_pvalue(obs_stat, sim_stats)
    env_min, env_max = envelope(curves)

    curves_path = f"{out_dir}/kl_curves.csv"
    with open(curves_path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(
            [
                "radius",
                "ordered_pair_count",
                "k_uncorrected",
                "k_translation_corrected",
                "l",
            ]
        )
        for r, c, ku, kc, lv in zip(radii, counts, k_raw, k_corr, l_obs):
            writer.writerow([r, c, ku, kc, lv])

    report = {
        "window": {"width": window.width, "height": window.height},
        "n_points": len(points),
        "radii": radii,
        "observed_statistic": obs_stat,
        "statistic": "max absolute deviation of L(r) - r over the radius grid",
        "simulations": cfg["simulations"],
        "seed": cfg["seed"],
        "p_value_upper_tail": p_value,
        "p_value_note": "plus-one corrected; ties count towards the tail",
        "l_envelope_min": env_min,
        "l_envelope_max": env_max,
        "envelope_note": (
            "pointwise min/max envelope of simulated L curves; "
            "not a simultaneous confidence band"
        ),
    }
    report_path = f"{out_dir}/simulation_test.json"
    with open(report_path, "w", encoding="utf-8") as fh:
        json.dump(report, fh, indent=2)
        fh.write("\n")

    print(f"wrote {curves_path}")
    print(f"wrote {report_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
