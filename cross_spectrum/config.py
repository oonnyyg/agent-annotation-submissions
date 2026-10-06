"""Configuration loading and validation for cross_spectrum."""

from __future__ import annotations

import json
from dataclasses import dataclass


class ConfigError(ValueError):
    """Raised when the JSON configuration is invalid."""


@dataclass(frozen=True)
class Config:
    csv_path: str
    sample_rate: float
    segment_length: int
    overlap: int
    nfft: int
    window: str
    index_column: str
    channel1_column: str
    channel2_column: str
    spectrum_output: str
    diagnostics_output: str

    @property
    def step(self) -> int:
        return self.segment_length - self.overlap


def load_config(path: str) -> Config:
    with open(path, "r", encoding="utf-8") as fh:
        raw = json.load(fh)
    if not isinstance(raw, dict):
        raise ConfigError("configuration root must be a JSON object")

    def require(key: str):
        if key not in raw:
            raise ConfigError(f"missing required key: {key!r}")
        return raw[key]

    csv_path = require("csv_path")
    sample_rate = require("sample_rate")
    segment_length = require("segment_length")
    overlap = require("overlap")
    nfft = require("nfft")
    window = raw.get("window", "hann")

    if not isinstance(sample_rate, (int, float)) or isinstance(sample_rate, bool) or sample_rate <= 0:
        raise ConfigError("sample_rate must be a positive number")
    if not isinstance(segment_length, int) or isinstance(segment_length, bool):
        raise ConfigError("segment_length must be an integer")
    if not 3 <= segment_length <= 512:
        raise ConfigError("segment_length must be between 3 and 512")
    if not isinstance(overlap, int) or isinstance(overlap, bool):
        raise ConfigError("overlap must be an integer")
    if not 0 <= overlap < segment_length:
        raise ConfigError("overlap must be non-negative and smaller than segment_length")
    if not isinstance(nfft, int) or isinstance(nfft, bool):
        raise ConfigError("nfft must be an integer")
    if nfft < segment_length:
        raise ConfigError("nfft must not be shorter than segment_length")
    if window not in ("hann", "rectangular"):
        raise ConfigError("window must be 'hann' or 'rectangular'")

    return Config(
        csv_path=str(csv_path),
        sample_rate=float(sample_rate),
        segment_length=segment_length,
        overlap=overlap,
        nfft=nfft,
        window=window,
        index_column=str(raw.get("index_column", "index")),
        channel1_column=str(raw.get("channel1_column", "ch1")),
        channel2_column=str(raw.get("channel2_column", "ch2")),
        spectrum_output=str(raw.get("spectrum_output", "spectrum.csv")),
        diagnostics_output=str(raw.get("diagnostics_output", "diagnostics.json")),
    )
