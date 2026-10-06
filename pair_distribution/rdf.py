"""Radial distribution and coordination number computation.

Normalization conventions:
- Every frame is normalized on its own with the exact shell volume
  4/3*pi*(r_hi^3 - r_lo^3) and that frame's box volume, so frames with
  different box sizes never share a normalization constant.
- The reported g(r) is the unweighted mean of the per-frame g(r).
- The coordination number is the unweighted mean over frames of the
  per-center neighbour count inside r.
- Same-species pairs are counted as ordered pairs (i->j and j->i both
  count), excluding self-pairs; the target density uses (N-1)/V.
- Cross-species pairs use species a as centres and N_b/V as density.
"""

from __future__ import annotations

import math
from dataclasses import dataclass

from .config import PairSpec
from .errors import PairDistributionError
from .trajectory import Frame


@dataclass(frozen=True)
class FrameHistogram:
    """Per-frame result for one pair: raw bin counts."""

    frame_index: int
    counts: tuple[int, ...]
    n_centers: int
    n_targets: int
    volume: float


@dataclass(frozen=True)
class PairResult:
    label: str
    empty: bool
    reason: str
    histograms: tuple[FrameHistogram, ...]
    g_avg: tuple[float, ...]
    cn_avg: tuple[float, ...]

    @property
    def total_counts(self) -> tuple[int, ...]:
        n_bins = len(self.g_avg)
        totals = [0] * n_bins
        for hist in self.histograms:
            for k, value in enumerate(hist.counts):
                totals[k] += value
        return tuple(totals)


def _min_image(delta: float, length: float) -> float:
    return delta - length * round(delta / length)


def _frame_histogram(
    frame: Frame, pair: PairSpec, n_bins: int, dr: float, r_max: float
) -> FrameHistogram:
    centers = [pid for pid, (t, _) in frame.particles.items() if t == pair.a]
    same = pair.a == pair.b
    targets = centers if same else [
        pid for pid, (t, _) in frame.particles.items() if t == pair.b
    ]
    counts = [0] * n_bins
    box = frame.box
    for center in centers:
        cx, cy, cz = frame.particles[center][1]
        for target in targets:
            if same and target == center:
                continue
            tx, ty, tz = frame.particles[target][1]
            dx = _min_image(tx - cx, box[0])
            dy = _min_image(ty - cy, box[1])
            dz = _min_image(tz - cz, box[2])
            r = math.sqrt(dx * dx + dy * dy + dz * dz)
            if r >= r_max:
                continue
            k = int(r / dr)
            if 0 <= k < n_bins:
                counts[k] += 1
    return FrameHistogram(
        frame_index=frame.index,
        counts=tuple(counts),
        n_centers=len(centers),
        n_targets=len(targets),
        volume=frame.volume,
    )


def _frame_g(hist: FrameHistogram, same: bool, n_bins: int, dr: float) -> list[float]:
    if hist.n_centers == 0:
        return [0.0] * n_bins
    effective_targets = hist.n_centers - 1 if same else hist.n_targets
    if effective_targets <= 0:
        return [0.0] * n_bins
    density = effective_targets / hist.volume
    values = []
    for k in range(n_bins):
        r_lo = k * dr
        rhi = r_lo + dr
        shell = 4.0 / 3.0 * math.pi * (rhi ** 3 - r_lo ** 3)
        norm = hist.n_centers * density * shell
        values.append(hist.counts[k] / norm)
    return values


def _frame_cn(hist: FrameHistogram, n_bins: int) -> list[float]:
    if hist.n_centers == 0:
        return [0.0] * n_bins
    values = []
    running = 0
    for k in range(n_bins):
        running += hist.counts[k]
        values.append(running / hist.n_centers)
    return values


def check_r_max(frames: list[Frame], r_max: float) -> float:
    """Return the limiting half-min-edge and reject an excessive r_max."""
    limit = min(frame.min_edge for frame in frames) / 2.0
    if r_max > limit:
        raise PairDistributionError(
            f"r_max {r_max} exceeds half of the shortest box edge over all frames ({limit})"
        )
    return limit


def compute_pair(
    frames: list[Frame], pair: PairSpec, n_bins: int, dr: float, r_max: float
) -> PairResult:
    species = {ptype for frame in frames for ptype, _ in frame.particles.values()}
    same = pair.a == pair.b
    if pair.a not in species or pair.b not in species:
        missing = pair.a if pair.a not in species else pair.b
        return PairResult(pair.label, True, f"species '{missing}' not present", (), 
                          (0.0,) * n_bins, (0.0,) * n_bins)
    if same:
        count = sum(1 for t, _ in frames[0].particles.values() if t == pair.a)
        if count < 2:
            return PairResult(pair.label, True,
                              f"species '{pair.a}' has fewer than 2 particles", (),
                              (0.0,) * n_bins, (0.0,) * n_bins)

    histograms = tuple(
        _frame_histogram(frame, pair, n_bins, dr, r_max) for frame in frames
    )
    g_frames = [_frame_g(h, same, n_bins, dr) for h in histograms]
    cn_frames = [_frame_cn(h, n_bins) for h in histograms]
    n_frames = len(frames)
    g_avg = tuple(
        math.fsum(g_frames[f][k] for f in range(n_frames)) / n_frames
        for k in range(n_bins)
    )
    cn_avg = tuple(
        math.fsum(cn_frames[f][k] for f in range(n_frames)) / n_frames
        for k in range(n_bins)
    )
    return PairResult(pair.label, False, "", histograms, g_avg, cn_avg)
