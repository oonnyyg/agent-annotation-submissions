"""Per-frame histograms, RDF normalization and coordination numbers."""

import math

from .trajectory import ConfigError


def check_r_max(frames, r_max):
    """r_max must not exceed half the shortest box edge of any frame."""
    worst = min(min(f.box) for f in frames) / 2.0
    if r_max > worst:
        raise ConfigError(
            f"r_max {r_max} exceeds half of the shortest box edge "
            f"({worst}) across all frames"
        )
    return worst


def min_image(delta, box):
    return tuple(d - L * round(d / L) for d, L in zip(delta, box))


def frame_histogram(frame, centers, targets, n_bins, r_max, exclude_self):
    """Count center-target pairs per radial bin for one frame.

    Bins are [lo, hi); r == r_max is excluded.
    """
    dr = r_max / n_bins
    hist = [0] * n_bins
    box = frame.box
    pos = frame.positions
    for ci in centers:
        pc = pos[ci]
        for ti in targets:
            if exclude_self and ti == ci:
                continue
            d = min_image(
                (pos[ti][0] - pc[0], pos[ti][1] - pc[1], pos[ti][2] - pc[2]),
                box,
            )
            r = math.sqrt(d[0] ** 2 + d[1] ** 2 + d[2] ** 2)
            if r >= r_max:
                continue
            hist[int(r / dr)] += 1
    return hist


def shell_volume(n_bins, r_max, k):
    dr = r_max / n_bins
    lo, hi = dr * k, dr * (k + 1)
    return 4.0 / 3.0 * math.pi * (hi ** 3 - lo ** 3)


def analyze_pair(frames, pair, n_bins, r_max):
    """Return per-frame and averaged results for one (a, b) species pair."""
    a, b = pair
    same = a == b
    per_frame = []
    for frame in frames:
        centers = [i for i, t in enumerate(frame.types) if t == a]
        targets = centers if same else [
            i for i, t in enumerate(frame.types) if t == b
        ]
        n_centers = len(centers)
        n_targets = len(targets)
        empty = n_centers == 0 or n_targets == 0 or (same and n_centers < 2)
        if empty:
            per_frame.append({
                "hist": [0] * n_bins,
                "g": [0.0] * n_bins,
                "cn": [0.0] * n_bins,
                "n_centers": n_centers,
                "n_targets": n_targets,
                "empty": True,
            })
            continue
        hist = frame_histogram(frame, centers, targets, n_bins, r_max, same)
        volume = frame.volume
        # Same-species pairs are ordered: each center sees n-1 targets.
        eff_targets = n_centers - 1 if same else n_targets
        density = eff_targets / volume
        g, cn = [], []
        cumulative = 0
        for k in range(n_bins):
            cumulative += hist[k]
            norm = n_centers * density * shell_volume(n_bins, r_max, k)
            g.append(hist[k] / norm)
            cn.append(cumulative / n_centers)
        per_frame.append({
            "hist": hist,
            "g": g,
            "cn": cn,
            "n_centers": n_centers,
            "n_targets": n_targets,
            "empty": False,
        })
    n_frames = len(frames)
    avg_g = [
        sum(pf["g"][k] for pf in per_frame) / n_frames for k in range(n_bins)
    ]
    avg_cn = [
        sum(pf["cn"][k] for pf in per_frame) / n_frames for k in range(n_bins)
    ]
    total_hist = [
        sum(pf["hist"][k] for pf in per_frame) for k in range(n_bins)
    ]
    return {
        "pair": pair,
        "per_frame": per_frame,
        "avg_g": avg_g,
        "avg_cn": avg_cn,
        "total_hist": total_hist,
    }
