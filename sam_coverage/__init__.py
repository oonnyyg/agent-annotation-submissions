"""SAM target coverage QC tool.

Computes target-region coverage from SAM alignments using only truly
aligned reference bases (M/=/X), never counting deletions (D) or
skipped intron regions (N) as coverage.
"""

__version__ = "0.1.0"
