"""Data model and validation for the discrete mass transport problem."""
from __future__ import annotations

from dataclasses import dataclass, field

MAX_SITES_PER_SIDE = 32


class TransportError(ValueError):
    """Raised when input data or configuration is invalid."""


@dataclass(frozen=True)
class Site:
    """A grid position carrying a non-negative integer mass."""

    site_id: str
    x: int
    y: int
    mass: int


@dataclass(frozen=True)
class Distribution:
    """A named set of sites; total mass must be positive when solving."""

    sites: tuple[Site, ...]

    @property
    def total_mass(self) -> int:
        return sum(site.mass for site in self.sites)


def _validate_sites(sites: list[Site], side: str) -> tuple[Site, ...]:
    if not sites:
        raise TransportError(f"{side}: distribution must contain at least one site")
    if len(sites) > MAX_SITES_PER_SIDE:
        raise TransportError(
            f"{side}: at most {MAX_SITES_PER_SIDE} sites allowed, got {len(sites)}"
        )
    seen: set[str] = set()
    for site in sites:
        if not site.site_id:
            raise TransportError(f"{side}: site id must be a non-empty string")
        if site.site_id in seen:
            raise TransportError(f"{side}: duplicate site id {site.site_id!r}")
        seen.add(site.site_id)
        if site.mass < 0:
            raise TransportError(
                f"{side}: site {site.site_id!r} has negative mass {site.mass}"
            )
    return tuple(sites)


@dataclass(frozen=True)
class Problem:
    """A fully validated transport problem ready for the solver."""

    sources: tuple[Site, ...]
    targets: tuple[Site, ...]
    cost_metric: str = "manhattan"  # "manhattan" | "sqeuclidean"
    disabled: frozenset[tuple[str, str]] = field(default_factory=frozenset)

    def __post_init__(self) -> None:
        object.__setattr__(self, "sources", _validate_sites(list(self.sources), "sources"))
        object.__setattr__(self, "targets", _validate_sites(list(self.targets), "targets"))
        if self.cost_metric not in ("manhattan", "sqeuclidean"):
            raise TransportError(
                f"cost_metric must be 'manhattan' or 'sqeuclidean', got {self.cost_metric!r}"
            )
        source_ids = {s.site_id for s in self.sources}
        target_ids = {t.site_id for t in self.targets}
        for pair in self.disabled:
            if len(pair) != 2:
                raise TransportError(f"disabled entry must be a pair, got {pair!r}")
            sid, tid = pair
            if sid not in source_ids:
                raise TransportError(f"disabled pair references unknown source {sid!r}")
            if tid not in target_ids:
                raise TransportError(f"disabled pair references unknown target {tid!r}")
        total_source = sum(s.mass for s in self.sources)
        total_target = sum(t.mass for t in self.targets)
        if total_source <= 0:
            raise TransportError("total mass must be positive")
        if total_source != total_target:
            raise TransportError(
                f"total source mass {total_source} != total target mass {total_target}"
            )

    def unit_cost(self, source: Site, target: Site) -> int:
        dx = source.x - target.x
        dy = source.y - target.y
        if self.cost_metric == "manhattan":
            return abs(dx) + abs(dy)
        return dx * dx + dy * dy
