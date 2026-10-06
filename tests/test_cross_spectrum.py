"""Tests for the cross_spectrum package (standard-library unittest)."""

import json
import math
import os
import subprocess
import sys
import tempfile
import unittest

from cross_spectrum.core import (
    Config,
    ConfigError,
    DataError,
    compute_cross_spectrum,
    load_config,
    read_csv_channels,
)

FS = 256.0


def sine(freq, phase=0.0, n=2048, fs=FS):
    return [math.sin(2.0 * math.pi * freq * i / fs + phase) for i in range(n)]


def make_config(**kw):
    base = dict(
        sample_rate=FS, segment_length=256, overlap=128, nfft=256, window="hann"
    )
    base.update(kw)
    return Config(**base)


def peak_index(result, psd):
    # ignore DC
    return max(range(1, len(psd)), key=lambda k: psd[k])


class PhaseShiftTest(unittest.TestCase):
    def test_known_phase_shift_recovered(self):
        ch1 = sine(16.0)
        ch2 = sine(16.0, phase=math.pi / 3)
        result = compute_cross_spectrum(ch1, ch2, make_config())
        k = peak_index(result, result.psd1)
        self.assertAlmostEqual(result.frequencies[k], 16.0)
        self.assertAlmostEqual(result.phase[k], math.pi / 3, places=6)
        self.assertAlmostEqual(result.coherence[k], 1.0, places=9)
        # both channels carry the same power
        self.assertAlmostEqual(result.psd1[k], result.psd2[k], places=9)

    def test_negative_phase_shift(self):
        ch1 = sine(16.0)
        ch2 = sine(16.0, phase=-math.pi / 2)
        result = compute_cross_spectrum(ch1, ch2, make_config())
        k = peak_index(result, result.psd1)
        self.assertAlmostEqual(result.phase[k], -math.pi / 2, places=6)


class UnrelatedSignalsTest(unittest.TestCase):
    def test_low_coherence_for_independent_noise(self):
        # deterministic pseudo-noise via LCG, different seeds per channel
        def noise(seed, n=4096):
            out = []
            state = seed
            for _ in range(n):
                state = (1103515245 * state + 12345) % (2**31)
                out.append(state / 2**31 - 0.5)
            return out

        result = compute_cross_spectrum(
            noise(1), noise(987654321), make_config()
        )
        coh = [c for c in result.coherence[1:] if c is not None]
        self.assertTrue(coh)
        self.assertLess(max(coh), 0.5)


class MissingDataTest(unittest.TestCase):
    def test_segment_with_missing_sample_is_skipped(self):
        ch1 = sine(16.0)
        ch2 = sine(16.0, phase=0.5)
        # corrupt one sample inside the second segment (no overlap)
        ch1[300] = None
        result = compute_cross_spectrum(ch1, ch2, make_config(overlap=0))
        self.assertEqual(result.stats.skipped_missing, 1)
        self.assertEqual(result.stats.skipped_missing_starts, [256])
        self.assertEqual(result.stats.used, result.stats.total_candidates - 1)
        k = peak_index(result, result.psd1)
        self.assertAlmostEqual(result.phase[k], 0.5, places=6)

    def test_all_segments_missing_fails(self):
        ch1 = [None] * 512
        ch2 = sine(16.0, n=512)
        with self.assertRaises(DataError):
            compute_cross_spectrum(ch1, ch2, make_config())

    def test_trailing_partial_segment_dropped(self):
        ch1 = sine(16.0, n=300)
        ch2 = sine(16.0, n=300)
        result = compute_cross_spectrum(ch1, ch2, make_config())
        # only one full 256-sample segment fits
        self.assertEqual(result.stats.total_candidates, 1)
        self.assertEqual(result.stats.used, 1)


class NullHandlingTest(unittest.TestCase):
    def test_zero_channel_gives_null_coherence_and_phase(self):
        ch1 = sine(16.0)
        ch2 = [0.0] * len(ch1)
        result = compute_cross_spectrum(ch1, ch2, make_config())
        k = peak_index(result, result.psd1)
        self.assertIsNone(result.coherence[k])
        self.assertIsNone(result.phase[k])


class EnergyCheckTest(unittest.TestCase):
    def test_spectral_integral_matches_windowed_energy(self):
        ch1 = sine(16.0)
        ch2 = sine(16.0, phase=1.0)
        result = compute_cross_spectrum(ch1, ch2, make_config())
        self.assertTrue(result.energy_check["consistent"])
        self.assertLess(result.energy_check["relative_difference"], 1e-9)


class ConfigValidationTest(unittest.TestCase):
    def _write(self, tmp, cfg):
        path = os.path.join(tmp, "cfg.json")
        with open(path, "w", encoding="utf-8") as fh:
            json.dump(cfg, fh)
        return path

    def test_invalid_configs_rejected(self):
        good = {
            "sample_rate": 100,
            "segment_length": 16,
            "overlap": 4,
            "nfft": 32,
            "window": "rect",
        }
        bad_cases = [
            {"sample_rate": 0},
            {"sample_rate": -5},
            {"segment_length": 2},
            {"segment_length": 513},
            {"overlap": -1},
            {"overlap": 16},
            {"nfft": 8},
            {"window": "blackman"},
        ]
        with tempfile.TemporaryDirectory() as tmp:
            for patch in bad_cases:
                cfg = dict(good)
                cfg.update(patch)
                with self.assertRaises(ConfigError, msg=str(patch)):
                    load_config(self._write(tmp, cfg))


class CliTest(unittest.TestCase):
    def test_end_to_end_cli(self):
        with tempfile.TemporaryDirectory() as tmp:
            csv_path = os.path.join(tmp, "data.csv")
            cfg_path = os.path.join(tmp, "cfg.json")
            spec_path = os.path.join(tmp, "spec.csv")
            diag_path = os.path.join(tmp, "diag.json")
            ch1 = sine(16.0)
            ch2 = sine(16.0, phase=math.pi / 4)
            ch1[50] = None  # missing sample in first segment
            with open(csv_path, "w", encoding="utf-8", newline="") as fh:
                fh.write("ch1,ch2\n")
                for a, b in zip(ch1, ch2):
                    fh.write(f"{'' if a is None else a},{b}\n")
            with open(cfg_path, "w", encoding="utf-8") as fh:
                json.dump(
                    {
                        "sample_rate": FS,
                        "segment_length": 256,
                        "overlap": 128,
                        "nfft": 256,
                        "window": "hann",
                    },
                    fh,
                )
            proc = subprocess.run(
                [
                    sys.executable,
                    "-m",
                    "cross_spectrum",
                    csv_path,
                    cfg_path,
                    "--spectrum",
                    spec_path,
                    "--diagnostics",
                    diag_path,
                ],
                capture_output=True,
                text=True,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            with open(diag_path, encoding="utf-8") as fh:
                diag = json.load(fh)
            self.assertEqual(diag["segments"]["skipped_missing"], 1)
            self.assertTrue(diag["energy_check"]["consistent"])
            with open(spec_path, encoding="utf-8") as fh:
                lines = fh.read().strip().splitlines()
            self.assertEqual(len(lines), 256 // 2 + 1 + 1)  # header + bins


if __name__ == "__main__":
    unittest.main()
