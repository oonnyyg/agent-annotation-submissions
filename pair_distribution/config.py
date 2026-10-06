"""Configuration loading and validation."""

from __future__ import annotations

import json
from dataclasses import dataclass

from .errors import PairDistributionError


@dataclass(frozen=True)
class PairSpec:
    """A species pair; ``a`` is the central species."""

    a: str
    b: str

    @property
    def label(self) -> str:
        return f"{self.a}-{self.b}"


@dataclass(frozen=True)
class Config:
    trajectory: str
    pairs: tuple[PairSpec, ...]
    r_max: float
    n_bins: int
    output_csv: str = "rdf.csv"
    summary_json: str = "summary.json"

    @property
    def dr(self) -> float:
        return self.r_max / self.n_bins


def load_config(path: str) -> Config:
    with open(path, "r", encoding="utf-8") as handle:
        raw = json.load(handle)
    if not isinstance(raw, dict):
        raise PairDistributionError("config root must be a JSON object")

    trajectory = raw.get("trajectory")
    if not isinstance(trajectory, str) or not trajectory:
        raise PairDistributionError("config requires a non-empty 'trajectory' path")

    rdf = raw.get("rdf")
    if not isinstance(rdf, dict):
        raise PairDistributionError("config requires an 'rdf' object")
    r_max = rdf.get("r_max")
    n_bins = rdf.get("n_bins")
    if not isinstance(r_max, (int, float)) or isinstance(r_max, bool) or r_max <= 0:
        raise PairDistributionError("'rdf.r_max' must be a positive number")
    if not isinstance(n_bins, int) or isinstance(n_bins, bool) or n_bins < 1:
        raise PairDistributionError("'rdf.n_bins' must be a positive integer")

    raw_pairs = raw.get("pairs")
    if not isinstance(raw_pairs, list) or not raw_pairs:
        raise PairDistributionError("config requires a non-empty 'pairs' list")
    pairs = []
    for entry in raw_pairs:
        if not isinstance(entry, dict):
            raise PairDistributionError("each pair must be an object with 'a' and 'b'")
        a, b = entry.get("a"), entry.get("b")
        if not isinstance(a, str) or not a or not isinstance(b, str) or not b:
            raise PairDistributionError("pair 'a' and 'b' must be non-empty strings")
        pairs.append(PairSpec(a=a, b=b))

    return Config(
        trajectory=trajectory,
        pairs=tuple(pairs),
        r_max=float(r_max),
        n_bins=n_bins,
        output_csv=str(raw.get("output_csv", "rdf.csv")),
        summary_json=str(raw.get("summary_json", "summary.json")),
    )
