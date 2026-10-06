"""JSONL trajectory loading and validation."""

from __future__ import annotations

import json
import math
from dataclasses import dataclass

from .errors import PairDistributionError


@dataclass(frozen=True)
class Frame:
    """One trajectory frame with an orthorhombic box."""

    index: int
    box: tuple[float, float, float]
    # particle id -> (species, (x, y, z)); coordinates may lie outside the box
    particles: dict[int, tuple[str, tuple[float, float, float]]]

    @property
    def volume(self) -> float:
        return self.box[0] * self.box[1] * self.box[2]

    @property
    def min_edge(self) -> float:
        return min(self.box)


def _parse_box(raw: object, line_no: int) -> tuple[float, float, float]:
    if not isinstance(raw, list) or len(raw) != 3:
        raise PairDistributionError(f"line {line_no}: 'box' must be a list of 3 numbers")
    box = []
    for value in raw:
        if not isinstance(value, (int, float)) or isinstance(value, bool) or not math.isfinite(value):
            raise PairDistributionError(f"line {line_no}: box lengths must be finite numbers")
        if value <= 0:
            raise PairDistributionError(f"line {line_no}: box lengths must be positive")
        box.append(float(value))
    return (box[0], box[1], box[2])


def _parse_particles(raw: object, line_no: int) -> dict[int, tuple[str, tuple[float, float, float]]]:
    if not isinstance(raw, list):
        raise PairDistributionError(f"line {line_no}: 'particles' must be a list")
    particles: dict[int, tuple[str, tuple[float, float, float]]] = {}
    for entry in raw:
        if not isinstance(entry, dict):
            raise PairDistributionError(f"line {line_no}: each particle must be an object")
        pid = entry.get("id")
        ptype = entry.get("type")
        pos = entry.get("position")
        if not isinstance(pid, int) or isinstance(pid, bool):
            raise PairDistributionError(f"line {line_no}: particle 'id' must be an integer")
        if pid in particles:
            raise PairDistributionError(f"line {line_no}: duplicate particle id {pid}")
        if not isinstance(ptype, str) or not ptype:
            raise PairDistributionError(f"line {line_no}: particle {pid} has an invalid 'type'")
        if not isinstance(pos, list) or len(pos) != 3:
            raise PairDistributionError(f"line {line_no}: particle {pid} 'position' must have 3 numbers")
        coords = []
        for value in pos:
            if not isinstance(value, (int, float)) or isinstance(value, bool) or not math.isfinite(value):
                raise PairDistributionError(
                    f"line {line_no}: particle {pid} has a non-finite coordinate"
                )
            coords.append(float(value))
        particles[pid] = (ptype, (coords[0], coords[1], coords[2]))
    return particles


def load_trajectory(path: str) -> list[Frame]:
    """Load frames, enforcing identical particle ids and species across frames."""
    frames: list[Frame] = []
    reference: dict[int, str] | None = None
    with open(path, "r", encoding="utf-8") as handle:
        for line_no, line in enumerate(handle, start=1):
            text = line.strip()
            if not text:
                continue
            try:
                raw = json.loads(text)
            except json.JSONDecodeError as exc:
                raise PairDistributionError(f"line {line_no}: invalid JSON: {exc}") from exc
            if not isinstance(raw, dict):
                raise PairDistributionError(f"line {line_no}: frame must be a JSON object")
            box = _parse_box(raw.get("box"), line_no)
            particles = _parse_particles(raw.get("particles"), line_no)
            current = {pid: ptype for pid, (ptype, _) in particles.items()}
            if reference is None:
                reference = current
            elif current != reference:
                missing = sorted(set(reference) - set(current))
                extra = sorted(set(current) - set(reference))
                detail = []
                if missing:
                    detail.append(f"missing ids {missing}")
                if extra:
                    detail.append(f"unexpected ids {extra}")
                changed = sorted(
                    pid for pid in set(reference) & set(current) if reference[pid] != current[pid]
                )
                if changed:
                    detail.append(f"species changed for ids {changed}")
                raise PairDistributionError(
                    f"line {line_no}: particle set mismatch ({'; '.join(detail)})"
                )
            frames.append(Frame(index=len(frames), box=box, particles=particles))
    if not frames:
        raise PairDistributionError("trajectory contains no frames")
    return frames
