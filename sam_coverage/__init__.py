"""SAM target coverage QC tool.

Computes per-target coverage from SAM alignments using only reference
bases that are truly aligned (CIGAR M/=/X), so deletions and intron
skips (N) never count as coverage.
"""

__version__ = "0.1.0"
