"""Parsing and validation of SAM, BED and JSON config inputs."""

from __future__ import annotations

import json
import re
from dataclasses import dataclass, field


class InputError(ValueError):
    """Raised for any malformed or inconsistent input."""


@dataclass(frozen=True)
class Target:
    name: str
    rname: str
    start: int  # 0-based, inclusive
    end: int    # 0-based, exclusive

    @property
    def length(self) -> int:
        return self.end - self.start


@dataclass
class SamRecord:
    qname: str
    flag: int
    rname: str
    pos: int        # 1-based leftmost position as in SAM
    mapq: int
    cigar: str
    seq: str
    line_no: int = 0


@dataclass
class Config:
    min_mapq: int = 0


_CIGAR_OP_RE = re.compile(r"(\d+)([MIDNSHP=X])")


def load_config(path: str) -> Config:
    with open(path, "r", encoding="utf-8") as fh:
        raw = json.load(fh)
    if not isinstance(raw, dict):
        raise InputError("config JSON must be an object")
    unknown = set(raw) - {"min_mapq"}
    if unknown:
        raise InputError(f"unknown config keys: {sorted(unknown)}")
    min_mapq = raw.get("min_mapq", 0)
    if not isinstance(min_mapq, int) or isinstance(min_mapq, bool) or min_mapq < 0:
        raise InputError("min_mapq must be a non-negative integer")
    return Config(min_mapq=min_mapq)


def load_targets(path: str, ref_lengths: dict[str, int]) -> list[Target]:
    targets: list[Target] = []
    seen: set[str] = set()
    with open(path, "r", encoding="utf-8") as fh:
        for line_no, line in enumerate(fh, 1):
            line = line.rstrip("\n")
            if not line or line.startswith("#"):
                continue
            parts = line.split("\t")
            if len(parts) < 4:
                parts = line.split()
            if len(parts) < 4:
                raise InputError(
                    f"BED line {line_no}: need at least 4 columns "
                    "(chrom, start, end, name)"
                )
            chrom, start_s, end_s, name = parts[0], parts[1], parts[2], parts[3]
            try:
                start, end = int(start_s), int(end_s)
            except ValueError:
                raise InputError(f"BED line {line_no}: non-integer coordinates") from None
            if chrom not in ref_lengths:
                raise InputError(f"BED line {line_no}: unknown reference '{chrom}'")
            if start < 0 or end <= start or end > ref_lengths[chrom]:
                raise InputError(
                    f"BED line {line_no}: target {name} [{start}, {end}) out of "
                    f"bounds for {chrom} (length {ref_lengths[chrom]})"
                )
            if name in seen:
                raise InputError(f"BED line {line_no}: duplicate target name '{name}'")
            seen.add(name)
            targets.append(Target(name=name, rname=chrom, start=start, end=end))
    return targets


def parse_sam(path: str):
    """Yield (header_dict, record) — returns via generator of records.

    Returns a tuple (ref_lengths, records_iterable). ref_lengths maps
    reference name to length from @SQ SN/LN tags.
    """
    fh = open(path, "r", encoding="utf-8")
    ref_lengths: dict[str, int] = {}
    for line in fh:
        if not line.startswith("@"):
            break
        if line.startswith("@SQ"):
            tags = dict(
                field.split(":", 1) for field in line.rstrip("\n").split("\t")[1:]
            )
            try:
                name, length = tags["SN"], int(tags["LN"])
            except (KeyError, ValueError):
                fh.close()
                raise InputError(f"malformed @SQ line: {line.rstrip()}") from None
            if length <= 0:
                fh.close()
                raise InputError(f"@SQ line for {name}: non-positive length")
            ref_lengths[name] = length
    else:
        fh.close()
        return ref_lengths, iter(())

    def _records(first_line):
        line_no = 0
        try:
            line = first_line
            while True:
                line_no += 1
                stripped = line.rstrip("\n")
                if stripped:
                    yield _parse_record(stripped, line_no)
                line = fh.readline()
                if not line:
                    break
        finally:
            fh.close()

    return ref_lengths, _records(line)


def _parse_record(line: str, line_no: int) -> SamRecord:
    fields = line.split("\t")
    if len(fields) < 11:
        raise InputError(f"SAM record line {line_no}: fewer than 11 fields")
    try:
        flag = int(fields[1])
        pos = int(fields[3])
        mapq = int(fields[4])
    except ValueError:
        raise InputError(
            f"SAM record line {line_no}: non-integer FLAG/POS/MAPQ"
        ) from None
    cigar = fields[5]
    if cigar != "*" and not re.fullmatch(r"(\d+[MIDNSHP=X])+", cigar):
        raise InputError(f"SAM record line {line_no}: invalid CIGAR '{cigar}'")
    return SamRecord(
        qname=fields[0],
        flag=flag,
        rname=fields[2],
        pos=pos,
        mapq=mapq,
        cigar=cigar,
        seq=fields[9],
        line_no=line_no,
    )


def parse_cigar(cigar: str) -> list[tuple[int, str]]:
    return [(int(n), op) for n, op in _CIGAR_OP_RE.findall(cigar)]
