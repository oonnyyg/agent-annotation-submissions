"""End-to-end and unit tests for the Morse decoder."""

from __future__ import annotations

import json
import os
import tempfile
import unittest

from morse_decoder.cli import main
from morse_decoder.segment import (
    ConfigError,
    SegmentConfig,
    normalize_samples,
    segment_audio,
    validate_segment_config,
)

from .helpers import SAMPLE_RATE, encode_text, silence, tone, write_wav

DOT = 0.06
CONFIG = {
    "window_seconds": 0.01,
    "onset_threshold": 0.3,
    "offset_threshold": 0.15,
    "dot_seconds": DOT,
    "tolerance": 0.4,
}


class DecodeTests(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.dir = self.tmp.name
        self.cfg_path = os.path.join(self.dir, "config.json")
        with open(self.cfg_path, "w", encoding="utf-8") as fh:
            json.dump(CONFIG, fh)

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def run_cli(self, samples, name="in.wav"):
        wav_path = os.path.join(self.dir, name)
        write_wav(wav_path, samples)
        seg_out = os.path.join(self.dir, name + ".segments.json")
        txt_out = os.path.join(self.dir, name + ".txt")
        code = main([wav_path, self.cfg_path,
                     "--segments-out", seg_out, "--text-out", txt_out])
        diag = text = None
        if os.path.exists(seg_out):
            with open(seg_out, encoding="utf-8") as fh:
                diag = json.load(fh)
        if os.path.exists(txt_out):
            with open(txt_out, encoding="utf-8") as fh:
                text = fh.read().strip()
        return code, diag, text

    def test_decodes_hello_world(self):
        code, diag, text = self.run_cli(encode_text("HELLO WORLD", DOT))
        self.assertEqual(code, 0)
        self.assertEqual(text, "HELLO WORLD")
        self.assertEqual(diag["text"], "HELLO WORLD")
        self.assertEqual(len(diag["characters"]), 11)
        first = diag["characters"][0]
        self.assertEqual(first["char"], "H")
        self.assertEqual(first["code"], "....")
        self.assertAlmostEqual(first["start"], 0.0, places=2)
        total = len(encode_text("HELLO WORLD", DOT)) / SAMPLE_RATE
        self.assertLess(diag["characters"][-1]["end"], total)

    def test_amplitude_variation(self):
        for amp in (0.5, 0.7, 1.0):
            code, _, text = self.run_cli(
                encode_text("SOS", DOT, amplitude=amp), name=f"a{amp}.wav"
            )
            self.assertEqual((code, text), (0, "SOS"))

    def test_consecutive_character_boundaries(self):
        code, diag, _ = self.run_cli(encode_text("EISH TMO", DOT))
        self.assertEqual(code, 0)
        self.assertEqual(diag["text"], "EISH TMO")
        codes = [c["code"] for c in diag["characters"] if c["char"] != " "]
        self.assertEqual(codes, [".", "..", "...", "....", "-", "--", "---"])

    def test_all_silence_returns_empty_text(self):
        code, diag, text = self.run_cli(silence(1.0))
        self.assertEqual(code, 0)
        self.assertEqual(text, "")
        self.assertEqual(diag["characters"], [])

    def test_unknown_code_reports_error(self):
        samples = (tone(DOT) + silence(DOT)) * 5 + tone(3 * DOT)
        code, diag, _ = self.run_cli(samples)
        self.assertEqual(code, 1)
        self.assertEqual(diag["text"], "")
        self.assertIn("unknown Morse code", diag["errors"][0]["reason"])

    def test_ambiguous_tone_reports_error(self):
        code, diag, _ = self.run_cli(tone(2 * DOT))
        self.assertEqual(code, 1)
        self.assertIn("ambiguous", diag["errors"][0]["reason"])
        self.assertEqual(diag["errors"][0]["start"], 0.0)

    def test_refuses_to_overwrite_outputs(self):
        code, _, _ = self.run_cli(encode_text("A", DOT))
        self.assertEqual(code, 0)
        code, _, _ = self.run_cli(encode_text("A", DOT))
        self.assertEqual(code, 2)


class SegmentTests(unittest.TestCase):
    def test_hysteresis_holds_state_between_thresholds(self):
        cfg = SegmentConfig(window_seconds=0.01, onset_threshold=0.5,
                            offset_threshold=0.2)
        samples = ([0.0] * 10 + [0.35] * 10 + [0.9] * 10
                   + [0.35] * 10 + [0.0] * 10)
        segs = segment_audio(samples, 1000, cfg)
        tones = [s for s in segs if s.is_tone]
        self.assertEqual(len(tones), 1)
        self.assertEqual(tones[0].start, 0.02)
        self.assertEqual(tones[0].end, 0.04)

    def test_partial_final_window_is_processed(self):
        cfg = SegmentConfig(window_seconds=0.01, onset_threshold=0.5,
                            offset_threshold=0.2)
        segs = segment_audio([0.9] * 25, 1000, cfg)
        self.assertEqual(len(segs), 1)
        self.assertAlmostEqual(segs[0].end, 0.025)

    def test_window_larger_than_fifth_of_dot_rejected(self):
        cfg = SegmentConfig(window_seconds=0.02, onset_threshold=0.5,
                            offset_threshold=0.2)
        with self.assertRaises(ConfigError):
            validate_segment_config(cfg, dot_seconds=0.06)

    def test_bad_thresholds_rejected(self):
        cfg = SegmentConfig(window_seconds=0.01, onset_threshold=0.2,
                            offset_threshold=0.5)
        with self.assertRaises(ConfigError):
            validate_segment_config(cfg, dot_seconds=0.06)

    def test_normalize_full_scale(self):
        self.assertEqual(normalize_samples([0, 32767, -32768]),
                         [0.0, 32767 / 32768, -1.0])


if __name__ == "__main__":
    unittest.main()
