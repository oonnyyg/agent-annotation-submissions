import io
import json
import tempfile
import unittest
from contextlib import redirect_stdout
from pathlib import Path

from mass_transport.cli import main
from mass_transport.model import InputError, load_problem
from mass_transport.solver import solve


def write(tmp: Path, name: str, text: str) -> Path:
    path = tmp / name
    path.write_text(text, encoding="utf-8")
    return path


def make_problem(tmp, sources, targets, config):
    s = write(tmp, "s.csv", "id,x,y,mass\n" + sources)
    t = write(tmp, "t.csv", "id,x,y,mass\n" + targets)
    c = write(tmp, "c.json", json.dumps(config))
    return load_problem(s, t, c)


class ValidationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp())

    def test_mass_mismatch_rejected(self):
        with self.assertRaises(InputError):
            make_problem(self.tmp, "a,0,0,5\n", "b,1,0,4\n", {})

    def test_zero_total_rejected(self):
        with self.assertRaises(InputError):
            make_problem(self.tmp, "a,0,0,0\n", "b,1,0,0\n", {})

    def test_negative_mass_rejected(self):
        with self.assertRaises(InputError):
            make_problem(self.tmp, "a,0,0,-1\n", "b,1,0,0\n", {})

    def test_duplicate_id_rejected(self):
        with self.assertRaises(InputError):
            make_problem(self.tmp, "a,0,0,1\na,1,1,1\n", "b,1,0,2\n", {})

    def test_too_many_locations_rejected(self):
        rows = "".join(f"s{i},{i},0,1\n" for i in range(33))
        with self.assertRaises(InputError):
            make_problem(self.tmp, rows, "b,0,0,33\n", {})

    def test_unknown_disabled_id_rejected(self):
        with self.assertRaises(InputError):
            make_problem(
                self.tmp, "a,0,0,1\n", "b,1,0,1\n",
                {"disabled": [["a", "zzz"]]},
            )

    def test_same_coordinates_different_ids_ok(self):
        problem = make_problem(
            self.tmp, "a,0,0,2\n", "b,0,0,1\nc,0,0,1\n", {}
        )
        result = solve(problem)
        self.assertTrue(result.feasible)
        self.assertEqual(result.total_cost, 0)


class SolverTests(unittest.TestCase):
    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp())

    def test_greedy_trap(self):
        # Per-source nearest-target greedy sends s0 -> t1 (its nearest),
        # which forces s2's mass onto the far target t0: total 30.
        # The optimum revises that choice and costs 28.
        problem = make_problem(
            self.tmp,
            "s0,3,4,3\ns1,6,0,1\ns2,3,1,2\n",
            "t0,5,6,5\nt1,3,3,1\n",
            {"metric": "manhattan"},
        )
        result = solve(problem)
        self.assertTrue(result.feasible)
        self.assertEqual(result.total_cost, 28)
        plan = {(f.source, f.target): f.amount for f in result.flows}
        self.assertEqual(
            plan,
            {("s0", "t0"): 3, ("s1", "t0"): 1, ("s2", "t0"): 1, ("s2", "t1"): 1},
        )

    def test_disabled_paths(self):
        problem = make_problem(
            self.tmp,
            "s1,0,0,5\ns2,10,0,5\n",
            "t1,0,0,5\nt2,10,0,5\n",
            {"metric": "manhattan", "disabled": [["s1", "t1"], ["s2", "t2"]]},
        )
        result = solve(problem)
        self.assertTrue(result.feasible)
        plan = {(f.source, f.target): f.amount for f in result.flows}
        self.assertEqual(plan, {("s1", "t2"): 5, ("s2", "t1"): 5})
        self.assertEqual(result.total_cost, 100)

    def test_infeasible_when_all_paths_disabled(self):
        problem = make_problem(
            self.tmp,
            "s1,0,0,5\n",
            "t1,3,0,5\n",
            {"disabled": [["s1", "t1"]]},
        )
        result = solve(problem)
        self.assertFalse(result.feasible)
        self.assertEqual(result.flows, ())

    def test_mass_scaling_not_unit_expansion(self):
        small = make_problem(
            self.tmp, "a,0,0,3\nb,5,0,3\n", "c,1,0,4\nd,9,0,2\n",
            {"metric": "sqeuclidean"},
        )
        big = make_problem(
            self.tmp,
            "a,0,0,3000000\nb,5,0,3000000\n",
            "c,1,0,4000000\nd,9,0,2000000\n",
            {"metric": "sqeuclidean"},
        )
        r_small = solve(small)
        r_big = solve(big)
        self.assertTrue(r_big.feasible)
        self.assertEqual(r_big.total_cost, r_small.total_cost * 1_000_000)
        self.assertLessEqual(len(r_big.flows), 3)
        self.assertEqual(r_small.average_cost, r_big.average_cost)

    def test_deterministic_tie_break(self):
        problem = make_problem(
            self.tmp, "a,0,0,1\nb,0,0,1\n", "c,0,0,1\nd,0,0,1\n", {}
        )
        first = solve(problem)
        second = solve(problem)
        self.assertEqual(first, second)
        plan = [(f.source, f.target) for f in first.flows]
        self.assertEqual(plan, sorted(plan))

    def test_conservation(self):
        problem = make_problem(
            self.tmp,
            "a,0,0,7\nb,3,4,5\nc,8,1,3\n",
            "d,2,2,6\ne,6,6,9\n",
            {"metric": "sqeuclidean"},
        )
        result = solve(problem)
        self.assertTrue(result.feasible)
        out = {}
        inc = {}
        for f in result.flows:
            out[f.source] = out.get(f.source, 0) + f.amount
            inc[f.target] = inc.get(f.target, 0) + f.amount
        self.assertEqual(out, {"a": 7, "b": 5, "c": 3})
        self.assertEqual(inc, {"d": 6, "e": 9})
        self.assertEqual(
            result.total_cost, sum(f.amount * f.unit_cost for f in result.flows)
        )


class CliTests(unittest.TestCase):
    def setUp(self):
        self.tmp = Path(tempfile.mkdtemp())

    def test_cli_success_and_report(self):
        s = write(self.tmp, "s.csv", "id,x,y,mass\na,0,0,4\nb,6,0,4\n")
        t = write(self.tmp, "t.csv", "id,x,y,mass\nc,1,0,4\nd,7,0,4\n")
        c = write(self.tmp, "c.json", json.dumps({"metric": "manhattan"}))
        buf = io.StringIO()
        with redirect_stdout(buf):
            code = main([str(s), str(t), str(c)])
        self.assertEqual(code, 0)
        out = buf.getvalue()
        self.assertIn("total cost: 8", out)
        self.assertIn("average cost per unit mass: 1.000000", out)
        self.assertIn("conservation check: OK", out)
        self.assertIn("a -> c: 4 @ 1 = 4", out)

    def test_cli_infeasible_exit_code(self):
        s = write(self.tmp, "s.csv", "id,x,y,mass\na,0,0,1\n")
        t = write(self.tmp, "t.csv", "id,x,y,mass\nb,1,0,1\n")
        c = write(self.tmp, "c.json", json.dumps({"disabled": [["a", "b"]]}))
        buf = io.StringIO()
        with redirect_stdout(buf):
            code = main([str(s), str(t), str(c)])
        self.assertEqual(code, 1)
        self.assertIn("INFEASIBLE", buf.getvalue())

    def test_cli_bad_input_exit_code(self):
        s = write(self.tmp, "s.csv", "id,x,y,mass\na,0,0,2\n")
        t = write(self.tmp, "t.csv", "id,x,y,mass\nb,1,0,1\n")
        c = write(self.tmp, "c.json", "{}")
        code = main([str(s), str(t), str(c)])
        self.assertEqual(code, 2)


if __name__ == "__main__":
    unittest.main()
