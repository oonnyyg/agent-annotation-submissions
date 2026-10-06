import unittest

from catenary_clearance.analysis import run


def valid_config(**overrides):
    cfg = {
        "supports": {"left": {"x": 0, "y": 0}, "right": {"x": 100, "y": 10}},
        "linear_density": 1.0,
        "gravity": 9.81,
        "terrain": [[0, -5], [100, -5]],
        "cases": [{"name": "a", "length": 120}],
    }
    cfg.update(overrides)
    return cfg


class ValidateTest(unittest.TestCase):
    def assert_invalid(self, cfg, fragment):
        with self.assertRaises(ValueError) as ctx:
            run(cfg)
        self.assertIn(fragment, str(ctx.exception))

    def test_reversed_supports(self):
        cfg = valid_config(supports={"left": {"x": 100, "y": 0},
                                     "right": {"x": 0, "y": 0}})
        self.assert_invalid(cfg, "strictly greater")

    def test_nonpositive_density(self):
        self.assert_invalid(valid_config(linear_density=0), "linear_density")

    def test_nonpositive_gravity(self):
        self.assert_invalid(valid_config(gravity=-9.81), "gravity")

    def test_unsorted_terrain(self):
        self.assert_invalid(valid_config(terrain=[[50, 0], [10, 0], [100, 0]]),
                            "increasing")

    def test_terrain_must_cover_span(self):
        self.assert_invalid(valid_config(terrain=[[10, 0], [100, 0]]), "cover")

    def test_empty_cases(self):
        self.assert_invalid(valid_config(cases=[]), "non-empty")

    def test_unequal_supports_ok(self):
        report, _, _ = run(valid_config())
        case = report["cases"][0]
        self.assertTrue(case["solved"])
        self.assertGreater(case["tension_right"], case["tension_left"])


if __name__ == "__main__":
    unittest.main()
