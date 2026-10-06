"""死时间回放模型测试。"""
import unittest

from coincidence_replay.core import (
    AcquisitionConfig,
    ChannelConfig,
    DeadTimeModel,
    Pulse,
    PulseStatus,
    ReplayError,
    replay_channel,
)

ACQ = AcquisitionConfig(start_ns=0, end_ns=1000, coincidence_window_ns=10)


def cfg(model: DeadTimeModel, dead: int = 100) -> ChannelConfig:
    return ChannelConfig(name="A", dead_time_ns=dead, model=model)


def pulses(*times: int) -> list[Pulse]:
    return [Pulse(f"p{i}", "A", t) for i, t in enumerate(times)]


class NonparalyzableTest(unittest.TestCase):
    def test_rejected_does_not_extend(self):
        res = replay_channel(
            pulses(0, 50, 100), cfg(DeadTimeModel.NONPARALYZABLE), ACQ
        )
        statuses = [r.status for r in res.records]
        self.assertEqual(
            statuses,
            [PulseStatus.ACCEPTED, PulseStatus.REJECTED, PulseStatus.ACCEPTED],
        )

    def test_exact_dead_time_end_is_accepted(self):
        res = replay_channel(
            pulses(0, 100), cfg(DeadTimeModel.NONPARALYZABLE), ACQ
        )
        accepted = [r.status for r in res.records]
        self.assertEqual(accepted, [PulseStatus.ACCEPTED, PulseStatus.ACCEPTED])

    def test_same_timestamp_sorted_by_id(self):
        ps = [Pulse("b", "A", 10), Pulse("a", "A", 10)]
        res = replay_channel(ps, cfg(DeadTimeModel.NONPARALYZABLE), ACQ)
        self.assertEqual([r.pulse.pulse_id for r in res.records], ["a", "b"])
        self.assertEqual(res.records[0].status, PulseStatus.ACCEPTED)
        self.assertEqual(res.records[1].status, PulseStatus.REJECTED)


class ExtendableTest(unittest.TestCase):
    def test_rejected_extends_dead_time(self):
        # 0 触发死区至 100；50 延至 150；120 在不可延长下可收、此处被拒并延至 220
        res = replay_channel(
            pulses(0, 50, 120), cfg(DeadTimeModel.EXTENDABLE), ACQ
        )
        statuses = [r.status for r in res.records]
        self.assertEqual(
            statuses,
            [PulseStatus.ACCEPTED, PulseStatus.REJECTED, PulseStatus.REJECTED],
        )
        self.assertEqual(res.busy_intervals, [(0, 220)])

    def test_exact_extended_end_is_accepted(self):
        res = replay_channel(
            pulses(0, 50, 150), cfg(DeadTimeModel.EXTENDABLE), ACQ
        )
        self.assertEqual(res.records[-1].status, PulseStatus.ACCEPTED)


class LiveTimeTest(unittest.TestCase):
    def test_busy_time_uses_union_not_sum(self):
        # 相邻死区 [0,100) 与 [100,200) 并集长度 200，不可简单相加
        res = replay_channel(
            pulses(0, 100), cfg(DeadTimeModel.NONPARALYZABLE), ACQ
        )
        self.assertEqual(res.busy_time_ns(ACQ), 200)
        self.assertEqual(res.live_time_ns(ACQ), 800)

    def test_busy_clipped_to_acquisition(self):
        res = replay_channel(
            pulses(950), cfg(DeadTimeModel.NONPARALYZABLE), ACQ
        )
        self.assertEqual(res.busy_time_ns(ACQ), 50)

    def test_negative_dead_time_rejected(self):
        with self.assertRaises(ReplayError):
            ChannelConfig(name="A", dead_time_ns=-1,
                          model=DeadTimeModel.NONPARALYZABLE)


if __name__ == "__main__":
    unittest.main()
