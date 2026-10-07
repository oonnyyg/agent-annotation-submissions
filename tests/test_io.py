import json
import math
import tempfile
import unittest
from pathlib import Path

from circular_mixture.io import (
    InputError,
    load_config,
    load_observations,
    to_radians,
)


class IoTestCase(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.dir = Path(self.tmp.name)

    def tearDown(self):
        self.tmp.cleanup()

    def write(self, name, text):
        p = self.dir / name
        p.write_text(text, encoding="utf-8")
        return p


class TestObservations(IoTestCase):
    def test_ok(self):
        p = self.write("obs.csv", "angle,weight\n10,1.5\n370,2\n")
        angles, weights = load_observations(p)
        self.assertEqual(angles, [10.0, 370.0])
        self.assertEqual(weights, [1.5, 2.0])

    def test_reject_nonpositive_weight(self):
        p = self.write("obs.csv", "angle,weight\n10,0\n")
        with self.assertRaises(InputError):
            load_observations(p)

    def test_reject_nonfinite(self):
        for bad in ("nan", "inf", "-inf"):
            p = self.write("obs.csv", f"angle,weight\n{bad},1\n")
            with self.assertRaises(InputError):
                load_observations(p)
            p = self.write("obs.csv", f"angle,weight\n10,{bad}\n")
            with self.assertRaises(InputError):
                load_observations(p)

    def test_reject_missing_columns(self):
        p = self.write("obs.csv", "angle\n10\n")
        with self.assertRaises(InputError):
            load_observations(p)

    def test_reject_empty(self):
        p = self.write("obs.csv", "angle,weight\n")
        with self.assertRaises(InputError):
            load_observations(p)


class TestConfig(IoTestCase):
    def test_degrees_normalized(self):
        p = self.write("cfg.json", json.dumps({
            "unit": "degrees",
            "components": [{"weight": 1, "mu": 370, "kappa": 2}],
        }))
        cfg = load_config(p)
        self.assertAlmostEqual(cfg["components"][0].mu, math.radians(10.0))

    def test_component_count_bounds(self):
        for n in (0, 4):
            p = self.write("cfg.json", json.dumps({
                "components": [{"weight": 1, "mu": 0, "kappa": 1}] * n,
            }))
            with self.assertRaises(InputError):
                load_config(p)

    def test_reject_bad_unit(self):
        p = self.write("cfg.json", json.dumps({
            "unit": "gradians",
            "components": [{"weight": 1, "mu": 0, "kappa": 1}],
        }))
        with self.assertRaises(InputError):
            load_config(p)

    def test_reject_negative_kappa(self):
        p = self.write("cfg.json", json.dumps({
            "components": [{"weight": 1, "mu": 0, "kappa": -1}],
        }))
        with self.assertRaises(InputError):
            load_config(p)

    def test_reject_all_zero_weights(self):
        p = self.write("cfg.json", json.dumps({
            "components": [{"weight": 0, "mu": 0, "kappa": 1}],
        }))
        with self.assertRaises(InputError):
            load_config(p)


class TestToRadians(IoTestCase):
    def test_wrap(self):
        r = to_radians([350.0, 370.0, -10.0], "degrees")
        self.assertAlmostEqual(r[0], math.radians(350.0))
        self.assertAlmostEqual(r[1], math.radians(10.0))
        self.assertAlmostEqual(r[2], math.radians(350.0))


if __name__ == "__main__":
    unittest.main()
