import csv
import json
import math
import os
import random
import tempfile
import unittest

from cross_spectrum.config import ConfigError, load_config
from cross_spectrum.core import AnalysisError, analyze
from cross_spectrum.data import load_samples
from cross_spectrum.__main__ import main

FS = 256.0
FREQ = 16.0


def make_config(tmp, **overrides):
    cfg = {
        "csv_path": os.path.join(tmp, "data.csv"),
        "sample_rate": FS,
        "segment_length": 128,
        "overlap": 64,
        "nfft": 128,
        "window": "hann",
        "spectrum_output": os.path.join(tmp, "spectrum.csv"),
        "diagnostics_output": os.path.join(tmp, "diagnostics.json"),
    }
    cfg.update(overrides)
    path = os.path.join(tmp, "config.json")
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(cfg, fh)
    return path, cfg


def write_csv(path, rows):
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow(["index", "ch1", "ch2"])
        writer.writerows(rows)


def sine_rows(n, phase2, missing=()):
    rows = []
    for i in range(n):
        if i in missing:
            rows.append([i, "", ""])
            continue
        t = i / FS
        rows.append([
            i,
            f"{math.sin(2 * math.pi * FREQ * t):.12f}",
            f"{math.sin(2 * math.pi * FREQ * t + phase2):.12f}",
        ])
    return rows


def run_analysis(tmp, rows, **overrides):
    cfg_path, cfg = make_config(tmp, **overrides)
    write_csv(cfg["csv_path"], rows)
    config = load_config(cfg_path)
    samples = load_samples(cfg["csv_path"], "index", "ch1", "ch2")
    return analyze(samples, config)


class PhaseShiftTest(unittest.TestCase):
    def test_known_phase_shift_recovered(self):
        phase = 0.7
        with tempfile.TemporaryDirectory() as tmp:
            result = run_analysis(tmp, sine_rows(1024, phase))
        k = result.frequencies.index(FREQ)
        self.assertAlmostEqual(result.phase[k], phase, places=6)
        self.assertAlmostEqual(result.coherence[k], 1.0, places=6)
        self.assertGreater(result.valid_segments, 0)
        check = result.energy_check["ch1"]
        self.assertAlmostEqual(check["psd_integral"], check["mean_windowed_energy"], places=8)

    def test_negative_phase_shift(self):
        with tempfile.TemporaryDirectory() as tmp:
            result = run_analysis(tmp, sine_rows(1024, -1.2))
        k = result.frequencies.index(FREQ)
        self.assertAlmostEqual(result.phase[k], -1.2, places=6)


class UnrelatedSignalTest(unittest.TestCase):
    def test_independent_noise_low_coherence(self):
        rng1 = random.Random(1)
        rng2 = random.Random(2)
        rows = [[i, f"{rng1.gauss(0, 1):.12f}", f"{rng2.gauss(0, 1):.12f}"]
                for i in range(4096)]
        with tempfile.TemporaryDirectory() as tmp:
            result = run_analysis(tmp, rows)
        mid = [c for c in result.coherence[1:-1] if c is not None]
        self.assertLess(sum(mid) / len(mid), 0.2)


class MissingDataTest(unittest.TestCase):
    def test_segment_with_gap_skipped_not_spliced(self):
        # Samples 200..209 missing: exactly the segment starting at 192 is hit.
        missing = set(range(200, 210))
        with tempfile.TemporaryDirectory() as tmp:
            result = run_analysis(tmp, sine_rows(1024, 0.3, missing))
        # With step 64 and length 128, the gap hits segments at 128 and 192.
        self.assertEqual([s.start_index for s in result.skipped], [128, 192])
        # Remaining segments still recover the phase.
        k = result.frequencies.index(FREQ)
        self.assertAlmostEqual(result.phase[k], 0.3, places=6)

    def test_absent_rows_counted_as_missing(self):
        rows = sine_rows(512, 0.0)
        rows = [r for r in rows if not (50 <= r[0] < 60)]
        with tempfile.TemporaryDirectory() as tmp:
            result = run_analysis(tmp, rows)
        self.assertTrue(any("absent" in s.reason for s in result.skipped))

    def test_all_segments_invalid_fails(self):
        rows = [[i, "", ""] for i in range(64)]
        with tempfile.TemporaryDirectory() as tmp:
            with self.assertRaises(AnalysisError):
                run_analysis(tmp, rows)


class ConfigValidationTest(unittest.TestCase):
    def _expect_error(self, **overrides):
        with tempfile.TemporaryDirectory() as tmp:
            path, _ = make_config(tmp, **overrides)
            with self.assertRaises(ConfigError):
                load_config(path)

    def test_bad_values_rejected(self):
        self._expect_error(sample_rate=0)
        self._expect_error(segment_length=2)
        self._expect_error(segment_length=513)
        self._expect_error(overlap=-1)
        self._expect_error(segment_length=8, overlap=8)
        self._expect_error(segment_length=16, nfft=8)
        self._expect_error(window="blackman")


class CliTest(unittest.TestCase):
    def test_end_to_end_outputs(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg_path, cfg = make_config(tmp)
            write_csv(cfg["csv_path"], sine_rows(1024, 0.5))
            self.assertEqual(main([cfg_path]), 0)
            with open(cfg["spectrum_output"], encoding="utf-8") as fh:
                header = fh.readline().strip()
            self.assertIn("phase_rad", header)
            with open(cfg["diagnostics_output"], encoding="utf-8") as fh:
                diag = json.load(fh)
            self.assertGreater(diag["valid_segments"], 0)
            self.assertEqual(diag["skipped_segments"], [])


if __name__ == "__main__":
    unittest.main()
