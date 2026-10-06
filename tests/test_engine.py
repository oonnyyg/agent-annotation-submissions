import json
import tempfile
import unittest
from pathlib import Path

from plume_exposure.config import (
    ConfigError,
    MetPeriod,
    load_met_periods,
    load_scenario,
)
from plume_exposure.engine import compute_contributions, summarize

SCENARIO_JSON = {
    "decay_rate": 0.0,
    "dispersion": {
        "lateral_coeff": 0.5,
        "lateral_exponent": 0.9,
        "vertical_coeff": 0.3,
        "vertical_exponent": 0.8,
    },
    "sources": [
        {"id": "s1", "east_m": 0.0, "north_m": 0.0, "height_m": 10.0,
         "emission_rate": 1.0},
        {"id": "s2", "east_m": 100.0, "north_m": 0.0, "height_m": 0.0,
         "emission_rate": 2.0},
    ],
    "receptors": [
        {"id": "r1", "east_m": 0.0, "north_m": -500.0, "height_m": 0.0},
    ],
}


def make_scenario(tmp: Path, data: dict | None = None):
    path = tmp / "scenario.json"
    path.write_text(json.dumps(data or SCENARIO_JSON), encoding="utf-8")
    return load_scenario(path)


def make_met(tmp: Path, rows: list[str]):
    path = tmp / "met.csv"
    path.write_text(
        "start,end,wind_from_deg,wind_speed\n" + "\n".join(rows) + "\n",
        encoding="utf-8",
    )
    return load_met_periods(path)


class UnequalDurationSummaryTests(unittest.TestCase):
    def test_weighted_average_and_peak(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            scenario = make_scenario(tmp)
            # Period 0: wind from north, 1 h, receptor downwind of s1.
            # Period 1: wind from south, 3 h, receptor upwind of s1.
            periods = make_met(
                tmp,
                [
                    "2026-01-01T00:00:00,2026-01-01T01:00:00,0,2.0",
                    "2026-01-01T02:00:00,2026-01-01T05:00:00,180,2.0",
                ],
            )
            contributions = compute_contributions(scenario, periods)
            summaries = summarize(scenario, periods, contributions)
            self.assertEqual(len(contributions), 2 * 1 * 2)

            c0 = sum(
                c.concentration
                for c in contributions
                if c.period_index == 0 and c.receptor_id == "r1"
            )
            c1 = sum(
                c.concentration
                for c in contributions
                if c.period_index == 1 and c.receptor_id == "r1"
            )
            self.assertGreater(c0, 0.0)
            summary = summaries[0]
            self.assertEqual(summary.total_duration_s, 4 * 3600.0)
            self.assertAlmostEqual(
                summary.time_integral, c0 * 3600.0 + c1 * 3 * 3600.0
            )
            self.assertAlmostEqual(
                summary.weighted_average,
                (c0 * 3600.0 + c1 * 3 * 3600.0) / (4 * 3600.0),
            )
            self.assertEqual(summary.peak_period_index, 0 if c0 >= c1 else 1)

    def test_peak_tie_prefers_earliest_period(self):
        scenario = make_scenario(Path(tempfile.mkdtemp()))
        periods = (
            MetPeriod("2026-01-01T00:00:00", "2026-01-01T01:00:00", 0.0, 2.0, 3600.0),
            MetPeriod("2026-01-01T01:00:00", "2026-01-01T02:00:00", 0.0, 2.0, 3600.0),
        )
        contributions = compute_contributions(scenario, periods)
        summary = summarize(scenario, periods, contributions)[0]
        self.assertEqual(summary.peak_period_index, 0)


class MetValidationTests(unittest.TestCase):
    def test_overlap_rejected(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            with self.assertRaises(ConfigError):
                make_met(
                    Path(tmpdir),
                    [
                        "2026-01-01T00:00:00,2026-01-01T02:00:00,0,2.0",
                        "2026-01-01T01:00:00,2026-01-01T03:00:00,0,2.0",
                    ],
                )

    def test_gaps_allowed(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            periods = make_met(
                Path(tmpdir),
                [
                    "2026-01-01T00:00:00,2026-01-01T01:00:00,0,2.0",
                    "2026-01-01T05:00:00,2026-01-01T06:00:00,0,2.0",
                ],
            )
            self.assertEqual(len(periods), 2)

    def test_calm_wind_rejected(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            with self.assertRaises(ConfigError):
                make_met(
                    Path(tmpdir),
                    ["2026-01-01T00:00:00,2026-01-01T01:00:00,0,0.0"],
                )

    def test_negative_emission_rejected(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            bad = json.loads(json.dumps(SCENARIO_JSON))
            bad["sources"][0]["emission_rate"] = -1.0
            with self.assertRaises(ConfigError):
                make_scenario(Path(tmpdir), bad)


if __name__ == "__main__":
    unittest.main()
