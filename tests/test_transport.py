"""Tests for the discrete mass transport tool."""
from __future__ import annotations

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from transport.io import load_problem
from transport.model import Problem, Site, TransportError
from transport.solver import InfeasibleError, solve

ROOT = Path(__file__).resolve().parent.parent


def site(sid, x, y, mass):
    return Site(sid, x, y, mass)


def flows(solution):
    return {(a.source_id, a.target_id): a.amount for a in solution.allocations}


class ValidationTests(unittest.TestCase):
    def test_unequal_totals_rejected(self):
        with self.assertRaises(TransportError):
            Problem((site("a", 0, 0, 5),), (site("b", 1, 0, 4),))

    def test_zero_total_rejected(self):
        with self.assertRaises(TransportError):
            Problem((site("a", 0, 0, 0),), (site("b", 1, 0, 0),))

    def test_negative_mass_rejected(self):
        with self.assertRaises(TransportError):
            Problem((site("a", 0, 0, -1),), (site("b", 1, 0, 1),))

    def test_duplicate_ids_rejected(self):
        with self.assertRaises(TransportError):
            Problem(
                (site("a", 0, 0, 1), site("a", 1, 0, 1)),
                (site("b", 0, 0, 2),),
            )

    def test_too_many_sites_rejected(self):
        sources = tuple(site(f"s{i}", i, 0, 1) for i in range(33))
        with self.assertRaises(TransportError):
            Problem(sources, (site("t", 0, 0, 33),))

    def test_unknown_disabled_pair_rejected(self):
        with self.assertRaises(TransportError):
            Problem(
                (site("a", 0, 0, 1),),
                (site("b", 1, 0, 1),),
                disabled=frozenset({("a", "zzz")}),
            )


class SolverTests(unittest.TestCase):
    def test_greedy_trap_requires_reassignment(self):
        # Nearest-target greedy sends s1(6) -> t1 (cost 9) and s2(4) -> t2
        # (cost 10), total 94. Optimal: s1 covers t2 first (cost 0) and only
        # the remainder goes to t1, s2 -> t1 (cost 1); total 22.
        problem = Problem(
            (site("s1", 0, 0, 6), site("s2", 10, 0, 4)),
            (site("t1", 9, 0, 6), site("t2", 0, 0, 4)),
        )
        solution = solve(problem)
        self.assertEqual(solution.total_cost, 22)
        self.assertEqual(
            flows(solution),
            {("s1", "t2"): 4, ("s1", "t1"): 2, ("s2", "t1"): 4},
        )

    def test_disabled_route_respected(self):
        problem = Problem(
            (site("s1", 0, 0, 5),),
            (site("t1", 0, 0, 5), site("t2", 3, 4, 0)),
            cost_metric="sqeuclidean",
            disabled=frozenset({("s1", "t1")}),
        )
        # Disabling the only route to positive-mass target t1 is infeasible.
        with self.assertRaises(InfeasibleError):
            solve(problem)

    def test_disabled_route_reroutes(self):
        problem = Problem(
            (site("s1", 0, 0, 4), site("s2", 0, 0, 6)),
            (site("t1", 0, 0, 10),),
            disabled=frozenset({("s1", "t1")}),
        )
        with self.assertRaises(InfeasibleError):
            solve(problem)

    def test_disabled_pair_forces_detour(self):
        problem = Problem(
            (site("s1", 0, 0, 5), site("s2", 2, 0, 5)),
            (site("t1", 0, 0, 5), site("t2", 2, 0, 5)),
            disabled=frozenset({("s1", "t1")}),
        )
        solution = solve(problem)
        self.assertEqual(flows(solution), {("s1", "t2"): 5, ("s2", "t1"): 5})
        self.assertEqual(solution.total_cost, 20)

    def test_mass_scaling_not_expanded(self):
        # Large masses must be handled as bulk amounts, not per-unit objects.
        big = 10**9
        problem = Problem(
            (site("s1", 0, 0, big), site("s2", 5, 5, big)),
            (site("t1", 0, 0, big), site("t2", 5, 5, big)),
        )
        solution = solve(problem)
        self.assertEqual(solution.total_cost, 0)
        self.assertEqual(solution.total_mass, 2 * big)

    def test_split_delivery(self):
        problem = Problem(
            (site("s1", 0, 0, 10),),
            (site("t1", 1, 0, 4), site("t2", 2, 0, 6)),
        )
        solution = solve(problem)
        self.assertEqual(flows(solution), {("s1", "t1"): 4, ("s1", "t2"): 6})
        self.assertEqual(solution.total_cost, 4 * 1 + 6 * 2)

    def test_deterministic_repeated_runs(self):
        problem = Problem(
            (site("s1", 0, 0, 3), site("s2", 0, 2, 3)),
            (site("t1", 0, 1, 3), site("t2", 1, 1, 3)),
        )
        first = solve(problem)
        for _ in range(5):
            again = solve(problem)
            self.assertEqual(flows(first), flows(again))
            self.assertEqual(first.total_cost, again.total_cost)

    def test_sqeuclidean_metric(self):
        problem = Problem(
            (site("s1", 0, 0, 2),),
            (site("t1", 3, 4, 2),),
            cost_metric="sqeuclidean",
        )
        solution = solve(problem)
        self.assertEqual(solution.total_cost, 2 * 25)

    def test_zero_mass_sites_kept_and_unused(self):
        problem = Problem(
            (site("s1", 0, 0, 5), site("sz", 9, 9, 0)),
            (site("t1", 1, 0, 5), site("tz", 9, 9, 0)),
        )
        solution = solve(problem)
        self.assertEqual(flows(solution), {("s1", "t1"): 5})


class CliTests(unittest.TestCase):
    def _write_inputs(self, tmp, sources_csv, targets_csv, config):
        src = Path(tmp) / "sources.csv"
        dst = Path(tmp) / "targets.csv"
        cfg = Path(tmp) / "config.json"
        src.write_text(sources_csv, encoding="utf-8")
        dst.write_text(targets_csv, encoding="utf-8")
        cfg.write_text(json.dumps(config), encoding="utf-8")
        return src, dst, cfg

    def test_cli_end_to_end(self):
        with tempfile.TemporaryDirectory() as tmp:
            src, dst, cfg = self._write_inputs(
                tmp,
                "id,x,y,mass\ns1,0,0,6\ns2,10,0,4\n",
                "id,x,y,mass\nt1,9,0,6\nt2,0,0,4\n",
                {"cost_metric": "manhattan"},
            )
            proc = subprocess.run(
                [sys.executable, "-m", "transport.cli", str(src), str(dst), str(cfg)],
                capture_output=True, text=True, cwd=ROOT,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertIn("total cost: 22", proc.stdout)
            self.assertIn("average cost per unit mass: 2.200000", proc.stdout)
            self.assertIn("status: OK", proc.stdout)
            self.assertIn("s1 -> t2: 4 @ 0 = 0", proc.stdout)

    def test_cli_rejects_bad_input(self):
        with tempfile.TemporaryDirectory() as tmp:
            src, dst, cfg = self._write_inputs(
                tmp,
                "id,x,y,mass\ns1,0,0,5\n",
                "id,x,y,mass\nt1,1,0,4\n",
                {},
            )
            proc = subprocess.run(
                [sys.executable, "-m", "transport.cli", str(src), str(dst), str(cfg)],
                capture_output=True, text=True, cwd=ROOT,
            )
            self.assertEqual(proc.returncode, 2)
            self.assertIn("error:", proc.stderr)

    def test_cli_reports_infeasible(self):
        with tempfile.TemporaryDirectory() as tmp:
            src, dst, cfg = self._write_inputs(
                tmp,
                "id,x,y,mass\ns1,0,0,5\n",
                "id,x,y,mass\nt1,1,0,5\n",
                {"disabled": [["s1", "t1"]]},
            )
            proc = subprocess.run(
                [sys.executable, "-m", "transport.cli", str(src), str(dst), str(cfg)],
                capture_output=True, text=True, cwd=ROOT,
            )
            self.assertEqual(proc.returncode, 1)
            self.assertIn("infeasible:", proc.stderr)

    def test_load_problem_roundtrip(self):
        with tempfile.TemporaryDirectory() as tmp:
            src, dst, cfg = self._write_inputs(
                tmp,
                "id,x,y,mass\ns1,0,0,3\n",
                "id,x,y,mass\nt1,2,0,3\n",
                {"cost_metric": "sqeuclidean", "disabled": []},
            )
            problem = load_problem(str(src), str(dst), str(cfg))
            self.assertEqual(problem.cost_metric, "sqeuclidean")
            self.assertEqual(solve(problem).total_cost, 12)


if __name__ == "__main__":
    unittest.main()
