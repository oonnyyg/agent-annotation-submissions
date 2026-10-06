"""Filtering, CIGAR walking and event-based coverage computation."""

from __future__ import annotations

from collections import defaultdict
from dataclasses import dataclass, field

from .model import InputError, SamHeader, SamRecord, Target, parse_cigar

# FLAG bits
FLAG_UNMAPPED = 0x4
FLAG_SECONDARY = 0x100
FLAG_QCFAIL = 0x200
FLAG_DUPLICATE = 0x400
FLAG_SUPPLEMENTARY = 0x800

# CIGAR op classes
REF_COVER_OPS = frozenset("M=X")
REF_ADVANCE_OPS = frozenset("DN")
QUERY_CONSUME_OPS = frozenset("MIS=X")


@dataclass
class FilterCounts:
    """Counts of records dropped at each filtering stage, in order."""

    total: int = 0
    unmapped: int = 0
    not_primary: int = 0  # secondary + supplementary
    qcfail_or_duplicate: int = 0
    low_mapq: int = 0
    invalid_record: int = 0  # unknown reference, out of bounds, bad CIGAR/SEQ
    kept: int = 0


@dataclass
class TargetResult:
    target: Target
    mean_depth: float
    fraction_covered_1x: float
    histogram: dict[int, int]  # depth -> number of target bases


@dataclass
class JunctionSupport:
    rname: str
    start: int  # 0-based half-open skipped interval
    end: int
    count: int = 0


@dataclass
class CoverageResult:
    targets: list[TargetResult] = field(default_factory=list)
    junctions: list[JunctionSupport] = field(default_factory=list)
    filters: FilterCounts = field(default_factory=FilterCounts)


def walk_cigar(record: SamRecord):
    """Yield coverage intervals and skipped (N) intervals for a record.

    Returns (cover_intervals, skip_intervals) as 0-based half-open
    reference coordinate pairs. Only M/=/X cover; D and N only advance
    the reference cursor; I and S consume only the query.
    """
    ops = parse_cigar(record.cigar)
    ref_pos = record.pos - 1  # SAM POS is 1-based
    query_consumed = 0
    cover: list[tuple[int, int]] = []
    skips: list[tuple[int, int]] = []
    for length, op in ops:
        if op in REF_COVER_OPS:
            cover.append((ref_pos, ref_pos + length))
            ref_pos += length
            query_consumed += length
        elif op == "D":
            ref_pos += length
        elif op == "N":
            skips.append((ref_pos, ref_pos + length))
            ref_pos += length
        elif op in ("I", "S"):
            query_consumed += length
        elif op in ("H", "P"):
            pass
        else:  # pragma: no cover - parse_cigar rejects unknown ops
            raise InputError(f"unsupported CIGAR op {op!r}")
    return cover, skips, query_consumed


def _seq_length(record: SamRecord) -> int:
    return 0 if record.seq == "*" else len(record.seq)


def compute_coverage(
    records: list[SamRecord],
    targets: list[Target],
    header: SamHeader,
    min_mapq: int,
) -> CoverageResult:
    result = CoverageResult()
    filters = result.filters

    # Events per target id: position -> delta. Junctions keyed by
    # (rname, start, end) of the N-skipped reference interval.
    events: dict[str, dict[int, int]] = {t.target_id: defaultdict(int) for t in targets}
    junction_counts: dict[tuple[str, int, int], int] = defaultdict(int)

    # Index targets by reference for efficient overlap checks.
    targets_by_ref: dict[str, list[Target]] = defaultdict(list)
    for target in targets:
        targets_by_ref[target.rname].append(target)

    for record in records:
        filters.total += 1
        if record.flag & FLAG_UNMAPPED:
            filters.unmapped += 1
            continue
        if record.flag & (FLAG_SECONDARY | FLAG_SUPPLEMENTARY):
            filters.not_primary += 1
            continue
        if record.flag & (FLAG_QCFAIL | FLAG_DUPLICATE):
            filters.qcfail_or_duplicate += 1
            continue
        if record.mapq < min_mapq:
            filters.low_mapq += 1
            continue
        # Validate the retained record against the reference dictionary.
        if record.rname not in header.lengths:
            filters.invalid_record += 1
            continue
        try:
            cover, skips, query_consumed = walk_cigar(record)
        except InputError:
            filters.invalid_record += 1
            continue
        ref_len = header.lengths[record.rname]
        ref_end = max((end for _, end in cover), default=record.pos - 1)
        ref_end = max(ref_end, max((end for _, end in skips), default=record.pos - 1))
        if record.pos < 1 or ref_end > ref_len:
            filters.invalid_record += 1
            continue
        if query_consumed != _seq_length(record):
            filters.invalid_record += 1
            continue

        filters.kept += 1
        for start, end in cover:
            for target in targets_by_ref.get(record.rname, ()):
                lo = max(start, target.start)
                hi = min(end, target.end)
                if lo < hi:
                    events[target.target_id][lo] += 1
                    events[target.target_id][hi] -= 1
        for start, end in skips:
            junction_counts[(record.rname, start, end)] += 1

    for target in targets:
        result.targets.append(_summarize_target(target, events[target.target_id]))
    result.junctions = [
        JunctionSupport(rname, start, end, count)
        for (rname, start, end), count in sorted(junction_counts.items())
    ]
    return result


def _summarize_target(target: Target, events: dict[int, int]) -> TargetResult:
    """Sweep events within one target; zero depth counts explicitly."""
    histogram: dict[int, int] = defaultdict(int)
    length = target.end - target.start
    depth = 0
    cursor = target.start
    for pos in sorted(events):
        if pos > cursor:
            histogram[depth] += pos - cursor
        depth += events[pos]
        cursor = pos
    if cursor < target.end:
        histogram[depth] += target.end - cursor
    histogram.setdefault(0, 0)  # ensure zero depth is represented
    total_depth = sum(d * n for d, n in histogram.items())
    covered = sum(n for d, n in histogram.items() if d >= 1)
    return TargetResult(
        target=target,
        mean_depth=total_depth / length,
        fraction_covered_1x=covered / length,
        histogram=dict(sorted(histogram.items())),
    )
