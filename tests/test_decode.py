"""End-to-end and unit tests for the Morse decoder."""

from __future__ import annotations

import json
import os
import tempfile
import unittest

from morse_decoder.cli import main
from morse_decoder.morse import MORSE_TABLE
from morse_decoder.segment import segment_audio
from morse_decoder.wavfile import PcmAudio

from tests import synth

DOT = 0.06
WINDOW = 0.01
CFG = {
    "dot_duration": DOT,
    "tolerance": 0.4,
    "window": WINDOW,
    "threshold_on": 0.3,
    "threshold_off": 0.15,
}


class DecodeTestBase(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.dir = self.tmp.name

    def tearDown(self):
        self.tmp.cleanup()

    def run_cli(self, samples, cfg=None):
        wav_path = os.path.join(self.dir, "in.wav")
        cfg_path = os.path.join(self.dir, "cfg.json")
        out_path = os.path.join(self.dir, "out.json")
        synth.write_wav(wav_path, samples)
        with open(cfg_path, "w", encoding="utf-8") as fh:
            json.dump(cfg or CFG, fh)
        code = main([wav_path, cfg_path, "-o", out_path])
        report = None
        if os.path.exists(out_path):
            with open(out_path, encoding="utf-8") as fh:
                report = json.load(fh)
        return code, report


class TestRoundTrip(DecodeTestBase):
    def test_text_with_word_gap(self):
        samples = synth.silence(0.2)
        samples += synth.encode_text("SOS 123", DOT, MORSE_TABLE)
        samples += synth.silence(0.2)
        code, report = self.run_cli(samples)
        self.assertEqual(code, 0)
        self.assertEqual(report["text"], "SOS 123")
        chars = [c for c in report["characters"] if c["char"] != " "]
        self.assertEqual(chars[0]["char"], "S")
        self.assertEqual(chars[0]["code"], "...")
        self.assertAlmostEqual(chars[0]["start"], 0.2, delta=WINDOW)

    def test_all_silence_returns_empty(self):
        code, report = self.run_cli(synth.silence(0.5))
        self.assertEqual(code, 0)
        self.assertEqual(report["text"], "")
        self.assertEqual(report["characters"], [])

    def test_final_char_flushed_at_eof(self):
        samples = synth.encode_text("E", DOT, MORSE_TABLE)
        code, report = self.run_cli(samples)
        self.assertEqual(code, 0)
        self.assertEqual(report["text"], "E")


class TestAmplitudeAndHysteresis(DecodeTestBase):
    def test_quieter_amplitude_still_decodes(self):
        samples = synth.encode_text("HI", DOT, MORSE_TABLE, amplitude=0.45)
        code, report = self.run_cli(samples)
        self.assertEqual(code, 0)
        self.assertEqual(report["text"], "HI")

    def test_hysteresis_keeps_state_between_thresholds(self):
        # A dash whose middle dips to RMS between the thresholds must
        # remain voiced instead of splitting into two tones.
        samples = synth.tone(DOT, 0.8)
        samples += synth.tone(DOT, 0.28)
        samples += synth.tone(DOT, 0.8)
        samples += synth.silence(3 * DOT)
        samples += synth.tone(DOT, 0.8)
        code, report = self.run_cli(samples)
        self.assertEqual(code, 0)
        self.assertEqual(report["text"], "TE")

    def test_segment_states_merge(self):
        audio = PcmAudio(8000, synth.tone(0.03) + synth.silence(0.02))
        segs = segment_audio(audio, 0.01, 0.3, 0.15)
        self.assertEqual(len(segs), 2)
        self.assertTrue(segs[0].voiced)
        self.assertFalse(segs[1].voiced)


class TestCharBoundaries(DecodeTestBase):
    def test_consecutive_chars_split_by_three_unit_gap(self):
        samples = synth.encode_text("EE", DOT, MORSE_TABLE)
        code, report = self.run_cli(samples)
        self.assertEqual(code, 0)
        self.assertEqual(report["text"], "EE")
        self.assertEqual(len(report["characters"]), 2)

    def test_word_gap_produces_space(self):
        samples = synth.encode_text("A B", DOT, MORSE_TABLE)
        code, report = self.run_cli(samples)
        self.assertEqual(code, 0)
        self.assertEqual(report["text"], "A B")


class TestErrors(DecodeTestBase):
    def test_unknown_code_reports_time_range(self):
        # ".-.-.-" is not a letter or digit.
        samples = synth.tone(DOT) + synth.silence(DOT) + synth.tone(3 * DOT)
        samples += synth.silence(DOT) + synth.tone(DOT) + synth.silence(DOT)
        samples += synth.tone(3 * DOT) + synth.silence(DOT) + synth.tone(DOT)
        samples += synth.silence(DOT) + synth.tone(3 * DOT)
        code, report = self.run_cli(samples)
        self.assertEqual(code, 1)
        self.assertIsNone(report["text"])
        err = report["errors"][0]
        self.assertIn("未知码组", err["reason"])
        self.assertGreater(err["end"], err["start"])

    def test_ambiguous_tone_duration_rejected(self):
        # 2 units fits neither dot (1) nor dash (3) with tolerance 0.4.
        samples = synth.tone(2 * DOT) + synth.silence(3 * DOT)
        samples += synth.tone(DOT)
        code, report = self.run_cli(samples)
        self.assertEqual(code, 1)
        self.assertIn("无法", report["errors"][0]["reason"])

    def test_no_overwrite_existing_output(self):
        out_path = os.path.join(self.dir, "out.json")
        open(out_path, "w").close()
        wav_path = os.path.join(self.dir, "in.wav")
        cfg_path = os.path.join(self.dir, "cfg.json")
        synth.write_wav(wav_path, synth.silence(0.1))
        with open(cfg_path, "w", encoding="utf-8") as fh:
            json.dump(CFG, fh)
        self.assertEqual(main([wav_path, cfg_path, "-o", out_path]), 2)

    def test_window_larger_than_dot_fifth_rejected(self):
        bad = dict(CFG, window=DOT / 5 + 0.001)
        code, report = self.run_cli(synth.silence(0.1), cfg=bad)
        self.assertEqual(code, 2)
        self.assertIsNone(report)

    def test_inverted_thresholds_rejected(self):
        bad = dict(CFG, threshold_on=0.1, threshold_off=0.2)
        code, report = self.run_cli(synth.silence(0.1), cfg=bad)
        self.assertEqual(code, 2)

    def test_non_wav_rejected(self):
        wav_path = os.path.join(self.dir, "in.wav")
        with open(wav_path, "wb") as fh:
            fh.write(b"not a wav")
        cfg_path = os.path.join(self.dir, "cfg.json")
        with open(cfg_path, "w", encoding="utf-8") as fh:
            json.dump(CFG, fh)
        out_path = os.path.join(self.dir, "out.json")
        self.assertEqual(main([wav_path, cfg_path, "-o", out_path]), 2)


if __name__ == "__main__":
    unittest.main()
