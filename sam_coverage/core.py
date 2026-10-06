"""Filtering, CIGAR walking, coverage events and junction tallying."""

from __future__ import annotations

from collections import defaultdict

from .io import InputError, SamRecord, Target, parse_cigar

F_UNMAPPED = 0x4
F_SECONDARY = 0x100
F_QCFAIL = 0x200
F_DUPLICATE = 0x400
F_SUPPLEMENTARY = 0x800

REF_CONSUMING = {"M", "D", "N", "=", "X"}
QUERY_CONSUMING = {"M", "I", "S", "=", "X"}
COVERAGE_OPS = {"M", "=", "X"}


class RecordFilter:
    """Applies FLAG and MAPQ filtering, tracking drop reasons."""

    REASONS = (
        "unmapped",
        "not_primary",
        "qc_fail",
        "duplicate",
        "low_mapq",
        "unknown_reference",
        "out_of_bounds",
        "cigar_error",
    )

    def __init__(self, min_mapq: int, ref_lengths: dict[str, int]):
        self.min_mapq = min_mapq
        self.ref_lengths = ref_lengths
        self.counts = {reason: 0 for reason in self.REASONS}
        self.total = 0
        self.kept = 0

    def check(self, rec: SamRecord) -> str | None:
        """Return None if kept, else the drop reason."""
        self.total += 1
        flag = rec.flag
        if flag & F_UNMAPPED:
            reason = "unmapped"
        elif flag & (F_SECONDARY | F_SUPPLEMENTARY):
            reason = "not_primary"
        elif flag & F_QCFAIL:
            reason = "qc_fail"
        elif flag & F_DUPLICATE:
            reason = "duplicate"
        elif rec.mapq < self.min_mapq:
            reason = "low_mapq"
        elif rec.rname not in self.ref_lengths:
            reason = "unknown_reference"
        else:
            reason = None
        if reason is None:
            self.kept += 1
        else:
            self.counts[reason] += 1
        return reason

    def reject(self, reason: str) -> None:
        """Count a post-filter rejection (bounds / CIGAR problems)."""
        self.kept -= 1
        self.counts[reason] += 1


def walk_cigar(rec: SamRecord, ref_lengths: dict[str, int]):
    """Walk a kept record's CIGAR.

    Returns (aligned_blocks, skipped_blocks) in 0-based half-open
    reference coordinates. aligned_blocks come from M/=/X ops;
    skipped_blocks come from N ops only. Raises InputError on query
    consumption mismatch or out-of-bounds reference use.
    """
    ref_len = ref_lengths[rec.rname]
    ref_pos = rec.pos - 1  # to 0-based; never re-reversed for reverse strand
    seq = rec.seq
    seq_len = 0 if seq == "*" else len(seq)
    query_consumed = 0
    aligned: list[tuple[int, int]] = []
    skipped: list[tuple[int, int]] = []
    for length, op in parse_cigar(rec.cigar):
        if length <= 0:
            raise InputError(
                f"SAM record line {rec.line_no}: zero-length CIGAR op"
            )
        if op in REF_CONSUMING:
            end = ref_pos + length
            if ref_pos < 0 or end > ref_len:
                raise InputError(
                    f"SAM record line {rec.line_no}: alignment runs past "
                    f"reference {rec.rname} (length {ref_len})"
                )
            if op in COVERAGE_OPS:
                aligned.append((ref_pos, end))
            elif op == "N":
                skipped.append((ref_pos, end))
            # D only advances the reference cursor
            ref_pos = end
        if op in QUERY_CONSUMING:
            query_consumed += length
        # H and P consume neither query nor reference
    if seq_len and query_consumed != seq_len:
        raise InputError(
            f"SAM record line {rec.line_no}: CIGAR consumes {query_consumed} "
            f"query bases but SEQ has {seq_len}"
        )
    return aligned, skipped


class CoverageAccumulator:
    """Sweep-line coverage over a target interval using delta events.

    Memory is O(number of contributing records), not O(reference length).
    """

    def __init__(self, target: Target):
        self.target = target
        self.events: dict[int, int] = {}

    def add_blocks(self, blocks: list[tuple[int, int]]) -> None:
        start, end = self.target.start, self.target.end
        for b_start, b_end in blocks:
            lo = max(b_start, start)
            hi = min(b_end, end)
            if lo < hi:
                self.events[lo] = self.events.get(lo, 0) + 1
                self.events[hi] = self.events.get(hi, 0) - 1

    def summarize(self) -> dict:
        length = self.target.length
        depth_sum = 0
        covered_bases = 0
        histogram: dict[int, int] = {}
        depth = 0
        prev = self.target.start
        for pos in sorted(self.events):
            if pos > prev:
                span = pos - prev
                depth_sum += depth * span
                histogram[depth] = histogram.get(depth, 0) + span
                if depth >= 1:
                    covered_bases += span
            depth += self.events[pos]
            prev = pos
        if prev < self.target.end:
            span = self.target.end - prev
            histogram[0] = histogram.get(0, 0) + span
        if depth != 0:
            raise AssertionError("coverage events do not balance")
        return {
            "target": self.target,
            "length": length,
            "mean_depth": depth_sum / length,
            "covered_fraction": covered_bases / length,
            "histogram": histogram,
        }


class JunctionTally:
    """Counts spliced N-skips per (reference, start, end) interval."""

    def __init__(self):
        self._counts: dict[tuple[str, int, int], int] = defaultdict(int)

    def add(self, rname: str, blocks: list[tuple[int, int]]) -> None:
        for start, end in blocks:
            self._counts[(rname, start, end)] += 1

    def rows(self) -> list[dict]:
        return [
            {"rname": rname, "start": start, "end": end, "count": count}
            for (rname, start, end), count in sorted(self._counts.items())
        ]
