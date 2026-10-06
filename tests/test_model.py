
import math
import unittest

from aquifer_response.model import AquiferModel, ModelError, ObservationPoint, Well
from aquifer_response.well_function import theis_w

T = 1.0e-3   # m^2/s
S = 1.0e-4


def make_model(wells):
    return AquiferModel(transmissivity=T, storativity=S, wells=wells)


class SingleWellBenchmarkTest(unittest.TestCase):
    def test_matches_closed_form_theis(self):
        well = Well("W1", 0.0, 0.0, ((0.0, 0.01),))
        model = make_model([well])
        r, t = 30.0, 3600.0
        u = r * r * S / (4.0 * T * t)
        expected = 0.01 / (4.0 * math.pi * T) * theis_w(u)
        got = model.well_drawdown(well, r, 0.0, t)
        self.assertAlmostEqual(got, expected, delta=abs(expected) * 1e-12)

    def test_zero_before_first_schedule_time(self):
        well = Well("W1", 0.0, 0.0, ((100.0, 0.01),))
        model = make_model([well])
        self.assertEqual(model.well_drawdown(well, 10.0, 0.0, 99.9), 0.0)

    def test_increment_is_zero_at_switch_instant(self):
        well = Well("W1", 0.0, 0.0, ((0.0, 0.01), (500.0, 0.02)))
        model = make_model([well])
        at_switch = model.well_drawdown(well, 10.0, 0.0, 500.0)
        just_before = model.well_drawdown(well, 10.0, 0.0, 499.999)
        self.assertAlmostEqual(at_switch, just_before, delta=abs(just_before) * 1e-6)


class SuperpositionTest(unittest.TestCase):
    def test_two_wells_add_up(self):
        w1 = Well("W1", 0.0, 0.0, ((0.0, 0.01),))
        w2 = Well("W2", 50.0, 0.0, ((0.0, 0.005),))
        model = make_model([w1, w2])
        t = 7200.0
        total = model.well_drawdown(w1, 20.0, 0.0, t) + model.well_drawdown(
            w2, 20.0, 0.0, t
        )
        points = [ObservationPoint("obs", 20.0, 0.0)]
        series = model.total_drawdown(points, [t])
        self.assertAlmostEqual(series["obs"][0], total, delta=abs(total) * 1e-15)

    def test_recovery_after_shutdown_keeps_history(self):
        # Pump then stop: shutdown is a negative increment; the drawdown
        # must decay towards zero but the past response is not erased.
        well = Well("W1", 0.0, 0.0, ((0.0, 0.01), (3600.0, 0.0)))
        model = make_model([well])
        r = 25.0
        at_stop = model.well_drawdown(well, r, 0.0, 3600.0)
        during_recovery = model.well_drawdown(well, r, 0.0, 7200.0)
        late_recovery = model.well_drawdown(well, r, 0.0, 36000.0)
        self.assertGreater(at_stop, 0.0)
        self.assertGreater(during_recovery, 0.0)
        self.assertLess(during_recovery, at_stop)
        self.assertLess(late_recovery, during_recovery)

    def test_injection_gives_negative_drawdown(self):
        well = Well("W1", 0.0, 0.0, ((0.0, -0.01),))
        model = make_model([well])
        self.assertLess(model.well_drawdown(well, 10.0, 0.0, 1000.0), 0.0)

    def test_extra_sample_times_do_not_change_existing_values(self):
        well = Well("W1", 0.0, 0.0, ((0.0, 0.01), (3600.0, 0.0)))
        model = make_model([well])
        points = [ObservationPoint("obs", 15.0, 0.0)]
        coarse = model.total_drawdown(points, [1000.0, 5000.0])
        fine = model.total_drawdown(points, [1000.0, 2000.0, 3000.0, 5000.0])
        self.assertEqual(coarse["obs"][0], fine["obs"][0])
        self.assertEqual(coarse["obs"][1], fine["obs"][3])


class ValidationTest(unittest.TestCase):
    def test_non_positive_aquifer_parameters_rejected(self):
        with self.assertRaises(ModelError):
            AquiferModel(transmissivity=0.0, storativity=S)
        with self.assertRaises(ModelError):
            AquiferModel(transmissivity=T, storativity=-1.0)

    def test_observation_point_must_not_coincide_with_well(self):
        well = Well("W1", 3.0, 4.0, ((0.0, 0.01),))
        model = make_model([well])
        with self.assertRaises(ModelError):
            model.validate_points([ObservationPoint("obs", 3.0, 4.0)])

    def test_unsorted_schedule_rejected(self):
        well = Well("W1", 0.0, 0.0, ((100.0, 0.01), (50.0, 0.0)))
        with self.assertRaises(ModelError):
            make_model([well])


if __name__ == "__main__":
    unittest.main()
