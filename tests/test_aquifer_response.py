import json
import math
import os
import subprocess
import sys
import tempfile
import unittest

from aquifer_response.fitting import (
    Candidate,
    FittingError,
    Observation,
    score_candidates,
)
from aquifer_response.model import (
    Aquifer,
    ModelError,
    ObservationPoint,
    RateStep,
    Well,
    simulate,
    validate_geometry,
    well_contribution_at,
)
from aquifer_response.wellfunction import W, WellFunctionError, well_function

T = 1e-3
S = 2e-4
AQUIFER = Aquifer(T, S)
POINT = ObservationPoint("OW", 30.0, 0.0)


def make_well(rate, start=0.0, name="W1", x=0.0, y=0.0, extra=()):
    return Well(name, x, y, (RateStep(start, rate),) + tuple(extra))


class TestWellFunction(unittest.TestCase):
    def test_known_values(self):
        cases = {
            0.5: 0.5597735947761608,
            1.0: 0.2193839343955203,
            2.0: 0.0489005107080611,
            10.0: 4.156968929685324e-06,
        }
        for u, expected in cases.items():
            self.assertAlmostEqual(W(u), expected, places=12)

    def test_very_small_u_matches_log_approximation(self):
        u = 1e-12
        expected = -0.5772156649015329 - math.log(u)
        self.assertAlmostEqual(W(u), expected, places=6)

    def test_very_large_u_matches_asymptotic(self):
        u = 1e6
        expected = math.exp(-u) / u
        self.assertAlmostEqual(W(u), expected, delta=expected * 1e-5)

    def test_invalid_u_rejected(self):
        for bad in (0.0, -1.0, float("nan"), float("inf")):
            with self.assertRaises(WellFunctionError):
                well_function(bad)

    def test_failure_reason_on_tiny_budget(self):
        with self.assertRaises(WellFunctionError) as ctx:
            well_function(0.9, tolerance=1e-16, max_terms=2)
        self.assertIn("did not converge", ctx.exception.reason)


class TestSingleWellBenchmark(unittest.TestCase):
    def test_matches_direct_theis(self):
        well = make_well(0.02)
        t = 3600.0
        u = 30.0**2 * S / (4 * T * t)
        expected = 0.02 / (4 * math.pi * T) * W(u)
        got = well_contribution_at(AQUIFER, well, POINT, t)
        self.assertAlmostEqual(got, expected, places=15)

    def test_zero_before_pumping_and_at_switch(self):
        well = make_well(0.02, start=100.0)
        self.assertEqual(well_contribution_at(AQUIFER, well, POINT, 50.0), 0.0)
        self.assertEqual(well_contribution_at(AQUIFER, well, POINT, 100.0), 0.0)


class TestSuperposition(unittest.TestCase):
    def test_two_wells_sum(self):
        w1 = make_well(0.02, name="A")
        w2 = make_well(0.01, name="B", x=120.0)
        times = [1000.0, 5000.0, 20000.0]
        both = simulate(AQUIFER, [w1, w2], [POINT], times)
        only_a = simulate(AQUIFER, [w1], [POINT], times)
        only_b = simulate(AQUIFER, [w2], [POINT], times)
        for i in range(len(times)):
            self.assertAlmostEqual(
                both["OW"]["total"][i],
                only_a["OW"]["total"][i] + only_b["OW"]["total"][i],
                places=15,
            )
            self.assertAlmostEqual(
                both["OW"]["total"][i],
                both["OW"]["A"][i] + both["OW"]["B"][i],
                places=15,
            )

    def test_sampling_density_does_not_change_results(self):
        well = make_well(0.02)
        coarse = simulate(AQUIFER, [well], [POINT], [1000.0, 5000.0])
        fine = simulate(AQUIFER, [well], [POINT], [1000.0, 2000.0, 5000.0])
        self.assertEqual(coarse["OW"]["total"][0], fine["OW"]["total"][0])
        self.assertEqual(coarse["OW"]["total"][1], fine["OW"]["total"][2])


class TestRecovery(unittest.TestCase):
    def test_shutdown_keeps_history_and_recovers(self):
        well = make_well(0.02, extra=(RateStep(10000.0, 0.0),))
        during = well_contribution_at(AQUIFER, well, POINT, 9000.0)
        just_after = well_contribution_at(AQUIFER, well, POINT, 10001.0)
        later = well_contribution_at(AQUIFER, well, POINT, 20000.0)
        much_later = well_contribution_at(AQUIFER, well, POINT, 1e12)
        self.assertGreater(just_after, 0.0)
        self.assertGreater(during, 0.0)
        self.assertGreater(just_after, later)
        self.assertGreater(later, much_later)
        self.assertLess(much_later, 1e-6)

    def test_injection_gives_negative_drawdown(self):
        well = make_well(-0.02)
        self.assertLess(well_contribution_at(AQUIFER, well, POINT, 5000.0), 0.0)


class TestValidation(unittest.TestCase):
    def test_nonpositive_aquifer_params(self):
        with self.assertRaises(ModelError):
            Aquifer(0.0, S)
        with self.assertRaises(ModelError):
            Aquifer(T, -1.0)

    def test_coincident_point_rejected(self):
        well = make_well(0.02)
        with self.assertRaises(ModelError):
            validate_geometry([well], [ObservationPoint("bad", 0.0, 0.0)])


class TestCandidateScoring(unittest.TestCase):
    def setUp(self):
        self.well = make_well(0.02)
        self.times = [1000.0, 5000.0, 20000.0]
        series = simulate(AQUIFER, [self.well], [POINT], self.times)
        self.observations = [
            Observation("OW", t, s, 1.0)
            for t, s in zip(self.times, series["OW"]["total"])
        ]

    def test_true_parameters_rank_first(self):
        candidates = [
            Candidate("lo", 5e-4, S),
            Candidate("true", T, S),
            Candidate("hi", 2e-3, S),
        ]
        ranked = score_candidates(candidates, self.observations, [self.well], [POINT])
        self.assertEqual(ranked[0].name, "true")
        self.assertAlmostEqual(ranked[0].weighted_sse, 0.0, places=20)
        self.assertEqual([s.rank for s in ranked], [1, 2, 3])
        self.assertLess(ranked[0].weighted_sse, ranked[1].weighted_sse)
        self.assertLess(ranked[1].weighted_sse, ranked[2].weighted_sse)

    def test_ties_keep_input_order(self):
        candidates = [
            Candidate("first", 5e-4, S),
            Candidate("second", 5e-4, S),
            Candidate("third", 5e-4, S),
        ]
        ranked = score_candidates(candidates, self.observations, [self.well], [POINT])
        self.assertEqual([s.name for s in ranked], ["first", "second", "third"])

    def test_weights_change_ranking(self):
        heavy = [
            Observation("OW", self.times[0], 0.0, 100.0),
            Observation("OW", self.times[1], 0.0, 1.0),
        ]
        candidates = [Candidate("true", T, S), Candidate("zero-ish", 1e9, S)]
        ranked = score_candidates(candidates, heavy, [self.well], [POINT])
        self.assertEqual(ranked[0].name, "zero-ish")

    def test_invalid_weight_rejected(self):
        bad = [Observation("OW", 1.0, 0.0, 0.0)]
        with self.assertRaises(FittingError):
            score_candidates([Candidate("a", T, S)], bad, [self.well], [POINT])


class TestCli(unittest.TestCase):
    def run_cli(self, job, outdir):
        job_path = os.path.join(outdir, "job.json")
        with open(job_path, "w", encoding="utf-8") as fh:
            json.dump(job, fh)
        return subprocess.run(
            [sys.executable, "-m", "aquifer_response", job_path, "-o", outdir],
            capture_output=True,
            text=True,
        )

    def base_job(self):
        return {
            "aquifer": {"transmissivity": T, "storativity": S},
            "wells": [
                {"name": "W1", "x": 0, "y": 0,
                 "schedule": [{"time": 0, "rate": 0.02},
                              {"time": 10000, "rate": 0.0}]}
            ],
            "observation_points": [{"name": "OW", "x": 30, "y": 0}],
            "times": [1000, 5000, 20000],
        }

    def test_end_to_end_and_failure_preserves_report(self):
        with tempfile.TemporaryDirectory() as outdir:
            ok = self.run_cli(self.base_job(), outdir)
            self.assertEqual(ok.returncode, 0, ok.stderr)
            report_path = os.path.join(outdir, "report.json")
            with open(report_path, encoding="utf-8") as fh:
                good_report = fh.read()
            self.assertIn("theis_confined_infinite", good_report)

            bad = self.base_job()
            bad["aquifer"]["transmissivity"] = -1.0
            failed = self.run_cli(bad, outdir)
            self.assertEqual(failed.returncode, 2)
            with open(report_path, encoding="utf-8") as fh:
                self.assertEqual(fh.read(), good_report)


if __name__ == "__main__":
    unittest.main()
