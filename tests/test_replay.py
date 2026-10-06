import csv
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from coincidence_replay.model import ReplayError, load_config, load_pulses
from coincidence_replay.report import run_replay


def make_config(tmp, **overrides):
    cfg = {
        "acquisition": {"start_ns": 0, "end_ns": 1000},
        "window_ns": 10,
        "channels": {
            "A": {"dead_time_ns": 100, "model": "nonparalyzable"},
            "B": {"dead_time_ns": 50, "model": "extendable", "time_offset_ns": 0},
        },
    }
    for key, value in overrides.items():
        cfg[key] = value
    path = Path(tmp) / "config.json"
    path.write_text(json.dumps(cfg), encoding="utf-8")
    return load_config(path)


def make_pulses(tmp, config, rows):
    path = Path(tmp) / "pulses.csv"
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["channel", "event_id", "time_ns"])
        writer.writerows(rows)
    return load_pulses(path, config)


class InputValidationTests(unittest.TestCase):
    def test_duplicate_event_id_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = make_config(tmp)
            with self.assertRaises(ReplayError):
                make_pulses(tmp, cfg, [["A", "e1", 10], ["B", "e1", 20]])

    def test_out_of_range_event_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = make_config(tmp)
            with self.assertRaises(ReplayError):
                make_pulses(tmp, cfg, [["A", "e1", 1000]])

    def test_left_closed_right_open_interval(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = make_config(tmp)
            pulses = make_pulses(tmp, cfg, [["A", "e1", 0], ["A", "e2", 999]])
            self.assertEqual(len(pulses["A"]), 2)

    def test_sorted_by_time_then_id(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = make_config(tmp)
            pulses = make_pulses(
                tmp, cfg, [["A", "b2", 5], ["A", "b1", 5], ["A", "a0", 1]]
            )
            ids = [p.event_id for p in pulses["A"]]
            self.assertEqual(ids, ["a0", "b1", "b2"])


class DeadTimeTests(unittest.TestCase):
    def test_nonparalyzable_rejection_keeps_dead_zone(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = make_config(tmp)
            pulses = make_pulses(
                tmp, cfg, [["A", "e1", 0], ["A", "e2", 50], ["A", "e3", 100]]
            )
            result = run_replay(pulses, cfg)
            states = {p.event_id: p.accepted for p in result.pulses["A"]}
            self.assertEqual(states, {"e1": True, "e2": False, "e3": True})

    def test_extendable_dead_zone_grows(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = make_config(tmp)
            pulses = make_pulses(
                tmp, cfg, [["B", "e1", 0], ["B", "e2", 40], ["B", "e3", 89]]
            )
            result = run_replay(pulses, cfg)
            states = {p.event_id: p.accepted for p in result.pulses["B"]}
            # e2 延长死区到 90，e3 在 89 被拒
            self.assertEqual(states, {"e1": True, "e2": False, "e3": False})

    def test_arrival_exactly_at_dead_end_accepted(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = make_config(tmp)
            pulses = make_pulses(tmp, cfg, [["A", "e1", 0], ["A", "e2", 100]])
            result = run_replay(pulses, cfg)
            self.assertTrue(all(p.accepted for p in result.pulses["A"]))

    def test_busy_time_uses_union_not_sum(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = make_config(tmp)
            pulses = make_pulses(tmp, cfg, [["A", "e1", 0], ["A", "e2", 150]])
            result = run_replay(pulses, cfg)
            # 两个死区 [0,100) 与 [150,250) 不重叠
            self.assertEqual(result.summary["channels"]["A"]["busy_time_ns"], 200)

    def test_busy_time_clipped_to_acquisition(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = make_config(tmp)
            pulses = make_pulses(tmp, cfg, [["A", "e1", 950]])
            result = run_replay(pulses, cfg)
            self.assertEqual(result.summary["channels"]["A"]["busy_time_ns"], 50)

    def test_zero_live_time_gives_null_rate(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = make_config(
                tmp,
                channels={
                    "A": {"dead_time_ns": 2000, "model": "nonparalyzable"},
                    "B": {"dead_time_ns": 0, "model": "extendable"},
                },
            )
            pulses = make_pulses(tmp, cfg, [["A", "e1", 0]])
            result = run_replay(pulses, cfg)
            ch_a = result.summary["channels"]["A"]
            self.assertEqual(ch_a["live_time_ns"], 0)
            self.assertIsNone(ch_a["accepted_rate_per_ns"])


class CoincidenceTests(unittest.TestCase):
    def test_same_timestamp_pulses_pair(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = make_config(tmp)
            pulses = make_pulses(tmp, cfg, [["A", "a1", 500], ["B", "b1", 500]])
            result = run_replay(pulses, cfg)
            self.assertEqual(result.summary["coincidence_count"], 1)
            self.assertEqual(result.pairs[0].diff_ns, 0)

    def test_offset_boundary_inclusive_window(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = make_config(
                tmp,
                channels={
                    "A": {"dead_time_ns": 0, "model": "nonparalyzable"},
                    "B": {"dead_time_ns": 0, "model": "nonparalyzable",
                          "time_offset_ns": 10},
                },
            )
            pulses = make_pulses(
                tmp, cfg,
                [["A", "a1", 500], ["B", "b1", 490], ["B", "b2", 501]],
            )
            result = run_replay(pulses, cfg)
            # b1 校时后 500，恰好差 0；b2 校时后 511 超出窗口
            self.assertEqual(result.summary["coincidence_count"], 1)
            self.assertEqual(result.pairs[0].event_b, "b1")

    def test_offset_pushes_pulse_out_of_range(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = make_config(
                tmp,
                channels={
                    "A": {"dead_time_ns": 0, "model": "nonparalyzable"},
                    "B": {"dead_time_ns": 0, "model": "nonparalyzable",
                          "time_offset_ns": 100},
                },
            )
            pulses = make_pulses(tmp, cfg, [["A", "a1", 950], ["B", "b1", 950]])
            result = run_replay(pulses, cfg)
            # b1 校时后 1050 超出区间，不参与符合
            self.assertEqual(result.summary["coincidence_count"], 0)
            b1 = result.pulses["B"][0]
            self.assertEqual(b1.note, "out_of_range_after_offset")

    def test_greedy_not_nearest_neighbor(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = make_config(
                tmp,
                channels={
                    "A": {"dead_time_ns": 0, "model": "nonparalyzable"},
                    "B": {"dead_time_ns": 0, "model": "nonparalyzable"},
                },
            )
            pulses = make_pulses(
                tmp, cfg,
                [["A", "a1", 100], ["A", "a2", 112],
                 ["B", "b1", 108], ["B", "b2", 113]],
            )
            result = run_replay(pulses, cfg)
            # 贪心：a1(100) 对 b1(108) 差 8 配对；a2(112) 对 b2(113) 配对
            self.assertEqual(result.summary["coincidence_count"], 2)
            self.assertEqual(
                [(p.event_a, p.event_b) for p in result.pairs],
                [("a1", "b1"), ("a2", "b2")],
            )

    def test_unmatched_reason_recorded(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = make_config(
                tmp,
                channels={
                    "A": {"dead_time_ns": 0, "model": "nonparalyzable"},
                    "B": {"dead_time_ns": 0, "model": "nonparalyzable"},
                },
            )
            pulses = make_pulses(tmp, cfg, [["A", "a1", 100], ["B", "b1", 500]])
            result = run_replay(pulses, cfg)
            self.assertEqual(result.pulses["A"][0].note, "unmatched_no_partner")


class CliTests(unittest.TestCase):
    def test_cli_replay_is_deterministic(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg_path = Path(tmp) / "config.json"
            cfg_path.write_text(json.dumps({
                "acquisition": {"start_ns": 0, "end_ns": 1000},
                "window_ns": 10,
                "channels": {
                    "A": {"dead_time_ns": 100, "model": "nonparalyzable"},
                    "B": {"dead_time_ns": 50, "model": "extendable",
                          "time_offset_ns": 5},
                },
            }), encoding="utf-8")
            csv_path = Path(tmp) / "pulses.csv"
            csv_path.write_text(
                "channel,event_id,time_ns\n"
                "A,a1,100\nA,a2,150\nA,a3,300\n"
                "B,b1,95\nB,b2,290\nB,b3,296\n",
                encoding="utf-8",
            )
            outputs = []
            for run in ("1", "2"):
                out_dir = Path(tmp) / f"out{run}"
                proc = subprocess.run(
                    [sys.executable, "-m", "coincidence_replay",
                     str(csv_path), str(cfg_path), "-o", str(out_dir)],
                    capture_output=True, text=True,
                    cwd=Path(__file__).resolve().parent.parent,
                )
                self.assertEqual(proc.returncode, 0, proc.stderr)
                outputs.append({
                    name: (out_dir / name).read_text(encoding="utf-8")
                    for name in ("pulses.csv", "pairs.csv", "summary.json")
                })
            self.assertEqual(outputs[0], outputs[1])
            summary = json.loads(outputs[0]["summary.json"])
            self.assertEqual(summary["coincidence_count"], 2)


if __name__ == "__main__":
    unittest.main()
