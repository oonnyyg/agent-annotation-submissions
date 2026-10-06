"""Data model and input parsing for SAM, BED and JSON config."""

from __future__ import annotations

import json
import re
from dataclasses import dataclass, field


class InputError(ValueError):
    """Raised for malformed or inconsistent input data."""


@dataclass(frozen=True)
class Target:
    """A BED target region in 0-based half-open coordinates."""

    target_id: str
    rname: str
    start: int
    end: int


@dataclass(frozen=True)
class Config:
    """JSON configuration."""

    min_mapq: int = 0


@dataclass
class SamRecord:
    """Parsed SAM alignment record."""

    qname: str
    flag: int
    rname: str
    pos: int  # 1-based leftmost position as stored in SAM
    mapq: int
    cigar: str
    seq: str


@dataclass
class SamHeader:
    """SAM header reference dictionary."""

    lengths: dict[str, int] = field(default_factory=dict)


def load_config(path: str) -> Config:
    with open(path, "r", encoding="utf-8") as handle:
        data = json.load(handle)
    if not isinstance(data, dict):
        raise InputError("config JSON must be an object")
    min_mapq = data.get("min_mapq", 0)
    if not isinstance(min_mapq, int) or min_mapq < 0:
        raise InputError("config 'min_mapq' must be a non-negative integer")
    return Config(min_mapq=min_mapq)


def load_targets(path: str, header: SamHeader) -> list[Target]:
    """Load BED targets, validating references, bounds and unique ids."""
    targets: list[Target] = []
    seen_ids: set[str] = set()
    with open(path, "r", encoding="utf-8") as handle:
        for lineno, raw in enumerate(handle, 1):
            line = raw.strip()
            if not line or line.startswith("#"):
                continue
            parts = line.split("\t")
            if len(parts) < 3:
                parts = line.split()
            if len(parts) < 3:
                raise InputError(f"BED line {lineno}: expected at least 3 columns")
            rname = parts[0]
            try:
                start = int(parts[1])
                end = int(parts[2])
            except ValueError as exc:
                raise InputError(f"BED line {lineno}: invalid coordinates") from exc
            target_id = parts[3] if len(parts) > 3 else f"{rname}:{start}-{end}"
            if target_id in seen_ids:
                raise InputError(f"BED line {lineno}: duplicate target id {target_id!r}")
            if rname not in header.lengths:
                raise InputError(
                    f"BED line {lineno}: unknown reference {rname!r}"
                )
            if start < 0 or end <= start or end > header.lengths[rname]:
                raise InputError(
                    f"BED line {lineno}: target {target_id!r} out of bounds "
                    f"for {rname!r} (length {header.lengths[rname]})"
                )
            seen_ids.add(target_id)
            targets.append(Target(target_id, rname, start, end))
    return targets


_CIGAR_RE = re.compile(r"(\d+)([MIDNSHP=X])")


def parse_cigar(cigar: str) -> list[tuple[int, str]]:
    if cigar == "*":
        return []
    ops: list[tuple[int, str]] = []
    pos = 0
    for match in _CIGAR_RE.finditer(cigar):
        if match.start() != pos:
            raise InputError(f"invalid CIGAR string: {cigar!r}")
        ops.append((int(match.group(1)), match.group(2)))
        pos = match.end()
    if pos != len(cigar) or not ops:
        raise InputError(f"invalid CIGAR string: {cigar!r}")
    return ops


def parse_sam(path: str):
    """Yield (header, records) by streaming a SAM file."""
    header = SamHeader()
    records: list[SamRecord] = []
    with open(path, "r", encoding="utf-8") as handle:
        for lineno, raw in enumerate(handle, 1):
            line = raw.rstrip("\n")
            if not line:
                continue
            if line.startswith("@"):
                if line.startswith("@SQ"):
                    name = None
                    length = None
                    for field_text in line.split("\t")[1:]:
                        if field_text.startswith("SN:"):
                            name = field_text[3:]
                        elif field_text.startswith("LN:"):
                            length = int(field_text[3:])
                    if name is None or length is None:
                        raise InputError(f"SAM line {lineno}: @SQ missing SN/LN")
                    header.lengths[name] = length
                continue
            fields = line.split("\t")
            if len(fields) < 11:
                raise InputError(f"SAM line {lineno}: fewer than 11 fields")
            try:
                flag = int(fields[1])
                pos = int(fields[3])
                mapq = int(fields[4])
            except ValueError as exc:
                raise InputError(f"SAM line {lineno}: invalid numeric field") from exc
            records.append(
                SamRecord(
                    qname=fields[0],
                    flag=flag,
                    rname=fields[2],
                    pos=pos,
                    mapq=mapq,
                    cigar=fields[5],
                    seq=fields[9],
                )
            )
    return header, records
