"""符合配对与端到端测试。"""
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from coincidence_replay.core import (
    AcquisitionConfig,
    Pulse,
    find_coincidences,
)

ACQ = AcquisitionConfig(start_ns=0, end_ns=1000, coincidence_window_ns=10)


def ps(channel: str, *items: tuple[str, int]) -> list[Pulse]:
    return [Pulse(pid, channel, t) for pid, t in items]


class CoincidenceTest(unittest.TestCase):
    def test_greedy_not_nearest_neighbor(self):
        # a1=0 与 b1=5 配对后，a2=6 不能再抢 b1，只能与 b2=100 失配被丢弃
        a = ps("A", ("a1", 0), ("a2", 6))
        b = ps("B", ("b1", 5), ("b2", 100))
        pairs, unpaired = find_coincidences(a, b, 0, 0, 10, ACQ)
        self.assertEqual(len(pairs), 1)
        self.assertEqual(pairs[0].delta_ns, 5)
        self.assertEqual([u.pulse.pulse_id for u in unpaired], ["a2", "b2"])
        self.assertEqual(unpaired[0].reason, "no_partner_in_window")
        self.assertEqual(unpaired[1].reason, "channel_exhausted")

    def test_window_boundary_inclusive(self):
        a = ps("A", ("a1", 100))
        b = ps("B", ("b1", 110))
        pairs, _ = find_coincidences(a, b, 0, 0, 10, ACQ)
        self.assertEqual(len(pairs), 1)

    def test_offset_shifts_and_excludes_out_of_range(self):
        # b1 校时后 -5 落在区间外不参与；b2 校时后 105 与 a1=100 配对
        a = ps("A", ("a1", 100))
        b = ps("B", ("b1", 0), ("b2", 110))
        pairs, unpaired = find_coincidences(a, b, 0, -5, 10, ACQ)
        self.assertEqual(len(pairs), 1)
        self.assertEqual(pairs[0].pulse_b.pulse_id, "b2")
        self.assertEqual(pairs[0].delta_ns, 5)
        self.assertEqual(unpaired, [])

    def test_offset_boundary_inclusive(self):
        # 校时后恰好等于 start/end 边界：start 闭、end 开
        a = ps("A", ("a1", 10), ("a2", 20))
        b = ps("B", ("b1", 5), ("b2", 990))
        pairs, unpaired = find_coincidences(a, b, -10, 10, 10, ACQ)
        # a1 校时后 0（保留），a2 校时后 10；b1 校时后 15，b2 校时后 1000（排除）
        self.assertEqual(len(pairs), 1)
        self.assertEqual(pairs[0].pulse_a.pulse_id, "a2")
        self.assertEqual([u.pulse.pulse_id for u in unpaired], ["a1"])


class EndToEndTest(unittest.TestCase):
    def _write_inputs(self, tmp: Path) -> tuple[Path, Path]:
        config = {
            "acquisition": {"start_ns": 0, "end_ns": 1000},
            "coincidence_window_ns": 10,
            "channels": {
                "A": {"dead_time_ns": 100, "model": "nonparalyzable",
                      "time_offset_ns": 0},
                "B": {"dead_time_ns": 50, "model": "extendable",
                      "time_offset_ns": 5},
            },
        }
        cfg_path = tmp / "config.json"
        cfg_path.write_text(json.dumps(config), encoding="utf-8")
        csv_path = tmp / "pulses.csv"
        csv_path.write_text(
            "pulse_id,channel,time_ns\n"
            "a1,A,0\n"
            "a2,A,50\n"
            "a3,A,300\n"
            "b1,B,4\n"
            "b2,B,300\n",
            encoding="utf-8",
        )
        return cfg_path, csv_path

    def test_cli_outputs_and_determinism(self):
        with tempfile.TemporaryDirectory() as td:
            tmp = Path(td)
            cfg_path, csv_path = self._write_inputs(tmp)
            outputs = []
            for run in ("1", "2"):
                out_dir = tmp / f"out{run}"
                proc = subprocess.run(
                    [sys.executable, "-m", "coincidence_replay",
                     "--pulses", str(csv_path), "--config", str(cfg_path),
                     "--out-dir", str(out_dir)],
                    capture_output=True, text=True,
                )
                self.assertEqual(proc.returncode, 0, proc.stderr)
                outputs.append({
                    p.name: p.read_text(encoding="utf-8")
                    for p in sorted(out_dir.iterdir())
                })
            self.assertEqual(outputs[0], outputs[1])
            summary = json.loads(outputs[0]["summary.json"])
            self.assertEqual(summary["channels"]["A"]["accepted_count"], 2)
            self.assertEqual(summary["channels"]["B"]["live_time_ns"], 900)
            # a1(0) 与 b1(4+5=9) 差 9 配对；a3(300) 与 b2(305) 差 5 配对
            self.assertEqual(summary["coincidences"]["pair_count"], 2)
            pairs_csv = outputs[0]["coincidence_pairs.csv"]
            self.assertIn("a1,b1,0,9,9", pairs_csv.replace("\r", ""))

    def test_duplicate_id_rejected(self):
        with tempfile.TemporaryDirectory() as td:
            tmp = Path(td)
            cfg_path, _ = self._write_inputs(tmp)
            bad = tmp / "bad.csv"
            bad.write_text(
                "pulse_id,channel,time_ns\nx,A,1\nx,B,2\n",
                encoding="utf-8",
            )
            proc = subprocess.run(
                [sys.executable, "-m", "coincidence_replay",
                 "--pulses", str(bad), "--config", str(cfg_path),
                 "--out-dir", str(tmp / "o")],
                capture_output=True, text=True,
            )
            self.assertEqual(proc.returncode, 2)
            self.assertIn("重复编号", proc.stderr)

    def test_out_of_range_rejected(self):
        with tempfile.TemporaryDirectory() as td:
            tmp = Path(td)
            cfg_path, _ = self._write_inputs(tmp)
            bad = tmp / "bad.csv"
            bad.write_text(
                "pulse_id,channel,time_ns\nx,A,1000\n",
                encoding="utf-8",
            )
            proc = subprocess.run(
                [sys.executable, "-m", "coincidence_replay",
                 "--pulses", str(bad), "--config", str(cfg_path),
                 "--out-dir", str(tmp / "o")],
                capture_output=True, text=True,
            )
            self.assertEqual(proc.returncode, 2)
            self.assertIn("采集区间", proc.stderr)


if __name__ == "__main__":
    unittest.main()
