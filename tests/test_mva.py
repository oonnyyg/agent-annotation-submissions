import json
import math
import unittest
from pathlib import Path
from tempfile import TemporaryDirectory

from closed_mva.analysis import analyze
from closed_mva.cli import main
from closed_mva.model import InputError, Network, Scenario, Station, load_network
from closed_mva.visits import visit_ratios


def make_net(scenarios, kind_b="single"):
    return Network(
        stations=[
            Station("a", "single", 0.5),
            Station("b", kind_b, 0.8),
        ],
        routing={"a": {"b": 1.0}, "b": {"a": 1.0}},
        reference_station="a",
        scenarios=scenarios,
    )


class VisitRatioTests(unittest.TestCase):
    def test_ratios_normalized_to_reference(self):
        net = Network(
            stations=[Station("a", "single", 1.0), Station("b", "single", 1.0),
                      Station("c", "single", 1.0)],
            routing={"a": {"b": 0.5, "c": 0.5}, "b": {"a": 1.0}, "c": {"a": 1.0}},
            reference_station="a",
            scenarios=[Scenario("s", 1)],
        )
        ratios = visit_ratios(net)
        self.assertEqual(ratios, {"a": 1.0, "b": 0.5, "c": 0.5})

    def test_periodic_routing_solves(self):
        # Deterministic 2-cycle: power iteration would oscillate forever.
        net = make_net([Scenario("s", 1)])
        ratios = visit_ratios(net)
        self.assertEqual(ratios, {"a": 1.0, "b": 1.0})

    def test_periodic_three_cycle(self):
        net = Network(
            stations=[Station(x, "single", 1.0) for x in "abc"],
            routing={"a": {"b": 1.0}, "b": {"c": 1.0}, "c": {"a": 1.0}},
            reference_station="b",
            scenarios=[Scenario("s", 1)],
        )
        self.assertEqual(visit_ratios(net), {"a": 1.0, "b": 1.0, "c": 1.0})


class MvaTests(unittest.TestCase):
    def test_population_zero_is_empty(self):
        report = analyze(make_net([Scenario("s", 3)]))
        zero = report.scenarios[0].curve[0]
        self.assertEqual(zero.population, 0)
        self.assertEqual(zero.throughput, 0.0)
        for st in zero.stations:
            self.assertEqual(st.queue_length, 0.0)
            self.assertEqual(st.residence_time, st.mean_service_time)

    def test_queue_lengths_sum_to_population(self):
        report = analyze(make_net([Scenario("s", 25)]))
        for point in report.scenarios[0].curve:
            total = sum(st.queue_length for st in point.stations)
            self.assertAlmostEqual(total, point.population, places=6)

    def test_infinite_server_residence_is_constant(self):
        report = analyze(make_net([Scenario("s", 10)], kind_b="infinite"))
        for point in report.scenarios[0].curve:
            b = next(st for st in point.stations if st.name == "b")
            self.assertEqual(b.residence_time, 0.8)
            self.assertEqual(b.load_kind, "busy_servers")

    def test_busy_servers_may_exceed_one(self):
        net = Network(
            stations=[Station("a", "single", 0.01), Station("b", "infinite", 50.0)],
            routing={"a": {"b": 1.0}, "b": {"a": 1.0}},
            reference_station="a",
            scenarios=[Scenario("s", 10)],
        )
        report = analyze(net)
        b = next(st for st in report.scenarios[0].final.stations if st.name == "b")
        self.assertGreater(b.load, 1.0)  # no utilization cap for delay servers

    def test_utilization_never_exceeds_one(self):
        report = analyze(make_net([Scenario("s", 50)]))
        for point in report.scenarios[0].curve:
            for st in point.stations:
                if st.kind == "single":
                    self.assertLessEqual(st.load, 1.0 + 1e-9)

    def test_known_two_station_values(self):
        # a: S=0.5, b: S=0.8, v=1 each, N=2 (both single-server).
        report = analyze(make_net([Scenario("s", 2)]))
        curve = report.scenarios[0].curve
        # N=1: R=(0.5,0.8), X=1/1.3, Q=(0.5/1.3, 0.8/1.3)
        p1 = curve[1]
        self.assertAlmostEqual(p1.throughput, 1 / 1.3)
        # N=2: Ra=0.5*(1+0.5/1.3), Rb=0.8*(1+0.8/1.3)
        ra = 0.5 * (1 + 0.5 / 1.3)
        rb = 0.8 * (1 + 0.8 / 1.3)
        p2 = curve[2]
        self.assertAlmostEqual(p2.throughput, 2 / (ra + rb))
        self.assertAlmostEqual(p2.cycle_time, ra + rb)

    def test_scenarios_are_independent(self):
        net = make_net([Scenario("base", 5), Scenario("fast", 5, {"b": 0.4})])
        report = analyze(net)
        base, fast = report.scenarios
        self.assertIsNone(base.throughput_change_vs_baseline)
        self.assertGreater(fast.final.throughput, base.final.throughput)
        self.assertAlmostEqual(
            fast.throughput_change_vs_baseline,
            (fast.final.throughput - base.final.throughput) / base.final.throughput)
        # Independent recursion: base curve matches a standalone base run.
        solo = analyze(make_net([Scenario("base", 5)])).scenarios[0]
        self.assertEqual([p.throughput for p in solo.curve],
                         [p.throughput for p in base.curve])


class InputValidationTests(unittest.TestCase):
    def write_tmp(self, data):
        self.tmp = TemporaryDirectory()
        path = Path(self.tmp.name) / "net.json"
        path.write_text(json.dumps(data), encoding="utf-8")
        return path

    def tearDown(self):
        if hasattr(self, "tmp"):
            self.tmp.cleanup()

    def base_input(self):
        return {
            "stations": [{"name": "a", "type": "single", "mean_service_time": 1.0},
                         {"name": "b", "type": "single", "mean_service_time": 1.0}],
            "routing": {"a": {"b": 1.0}, "b": {"a": 1.0}},
            "reference_station": "a",
            "scenarios": [{"name": "s", "population": 3}],
        }

    def test_row_sum_error_is_located(self):
        data = self.base_input()
        data["routing"]["a"] = {"b": 0.5}
        with self.assertRaisesRegex(InputError, r"routing\['a'\].*sum"):
            load_network(self.write_tmp(data))

    def test_disconnected_error_is_located(self):
        data = self.base_input()
        data["stations"].append({"name": "c", "type": "single", "mean_service_time": 1.0})
        data["routing"]["c"] = {"c": 1.0}
        with self.assertRaisesRegex(InputError, "strongly connected"):
            load_network(self.write_tmp(data))

    def test_bad_service_time(self):
        data = self.base_input()
        data["stations"][0]["mean_service_time"] = 0
        with self.assertRaisesRegex(InputError, r"stations\[0\]"):
            load_network(self.write_tmp(data))

    def test_unknown_override_station(self):
        data = self.base_input()
        data["scenarios"][0]["service_time_overrides"] = {"nope": 1.0}
        with self.assertRaisesRegex(InputError, "unknown station"):
            load_network(self.write_tmp(data))

    def test_bad_reference(self):
        data = self.base_input()
        data["reference_station"] = "zzz"
        with self.assertRaisesRegex(InputError, "reference_station"):
            load_network(self.write_tmp(data))


class CliTests(unittest.TestCase):
    def test_end_to_end_outputs(self):
        with TemporaryDirectory() as tmp:
            out = Path(tmp)
            code = main(["examples/network.json",
                         "--csv", str(out / "d.csv"),
                         "--json", str(out / "s.json")])
            self.assertEqual(code, 0)
            summary = json.loads((out / "s.json").read_text(encoding="utf-8"))
            self.assertEqual(summary["reference_station"], "cpu")
            self.assertEqual(len(summary["scenarios"]), 3)
            for sc in summary["scenarios"]:
                self.assertAlmostEqual(sc["queue_length_total"], sc["population"], places=6)
            rows = (out / "d.csv").read_text(encoding="utf-8").strip().splitlines()
            self.assertTrue(rows[0].startswith("scenario,population"))
            self.assertGreater(len(rows), 1)

    def test_cli_reports_input_error(self):
        with TemporaryDirectory() as tmp:
            bad = Path(tmp) / "bad.json"
            bad.write_text("{not json", encoding="utf-8")
            self.assertEqual(main([str(bad)]), 2)


if __name__ == "__main__":
    unittest.main()
