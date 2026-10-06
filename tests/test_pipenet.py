import json
import os
import shutil
import tempfile
import unittest

from pipenet.cli import main
from pipenet.model import ModelError, Network, Node, Pipe, load_network
from pipenet.solver import solve


def make_net(nodes, pipes):
    ns = {n[0]: Node(n[0], n[1], n[2] if len(n) > 2 else 0.0) for n in nodes}
    ps = [Pipe(p[0], p[1], p[2], p[3]) for p in pipes]
    net = Network(ns, ps)
    net.fixed_ids = [n.node_id for n in ns.values() if n.head is not None]
    net.free_ids = [n.node_id for n in ns.values() if n.head is None]
    return net


class TreeNetworkTest(unittest.TestCase):
    def test_tree_exact(self):
        # tank(100) --r=1--> a(demand 3) --r=2--> b(demand 1)
        net = make_net(
            [("tank", 100.0), ("a", None, 3.0), ("b", None, 1.0)],
            [("p1", "tank", "a", 1.0), ("p2", "a", "b", 2.0)],
        )
        res = solve(net, {"tank": 0.0, "a": 3.0, "b": 1.0})
        self.assertTrue(res.converged, res.reason)
        self.assertAlmostEqual(res.flows["p2"], 1.0, places=8)
        self.assertAlmostEqual(res.flows["p1"], 4.0, places=8)
        self.assertAlmostEqual(res.heads["a"], 100.0 - 16.0, places=6)
        self.assertAlmostEqual(res.heads["b"], 84.0 - 2.0, places=6)


class LoopNetworkTest(unittest.TestCase):
    def test_loop_split(self):
        # tank(100) feeds node n via two parallel paths of different r.
        net = make_net(
            [("tank", 100.0), ("n", None, 2.0)],
            [("p1", "tank", "n", 1.0), ("p2", "tank", "n", 4.0)],
        )
        res = solve(net, {"tank": 0.0, "n": 2.0})
        self.assertTrue(res.converged, res.reason)
        q1, q2 = res.flows["p1"], res.flows["p2"]
        self.assertAlmostEqual(q1 + q2, 2.0, places=8)
        self.assertAlmostEqual(1.0 * q1 * abs(q1), 4.0 * q2 * abs(q2), places=8)
        self.assertGreater(q1, q2)  # easier path carries more

    def test_closed_loop_circulation(self):
        # symmetric triangle: the closing link carries no flow.
        net = make_net(
            [("tank", 50.0), ("a", None, 1.0), ("b", None, 1.0)],
            [
                ("p1", "tank", "a", 1.0),
                ("p2", "tank", "b", 1.0),
                ("p3", "a", "b", 1.0),
            ],
        )
        res = solve(net, {"tank": 0.0, "a": 1.0, "b": 1.0})
        self.assertTrue(res.converged, res.reason)
        self.assertAlmostEqual(res.flows["p1"], res.flows["p2"], places=8)
        self.assertAlmostEqual(res.flows["p3"], 0.0, places=8)


class ReverseFlowTest(unittest.TestCase):
    def test_reverse_and_injection(self):
        # injection at b pushes flow back toward the tank (against p1 direction).
        net = make_net(
            [("tank", 10.0), ("a", None, 5.0), ("b", None, -20.0)],
            [("p1", "tank", "a", 0.1), ("p2", "b", "a", 0.1)],
        )
        res = solve(net, {"tank": 0.0, "a": 5.0, "b": -20.0})
        self.assertTrue(res.converged, res.reason)
        self.assertLess(res.flows["p1"], 0.0)
        self.assertAlmostEqual(res.flows["p2"], 20.0, places=8)
        self.assertAlmostEqual(res.flows["p1"], -15.0, places=8)
        self.assertGreater(res.heads["b"], res.heads["a"])
        self.assertGreater(res.heads["a"], 10.0)


class FailureTest(unittest.TestCase):
    def test_iteration_limit_not_disguised(self):
        net = make_net(
            [("tank", 100.0), ("a", None, 3.0)],
            [("p1", "tank", "a", 1.0)],
        )
        res = solve(net, {"tank": 0.0, "a": 3.0}, max_iter=0)
        self.assertFalse(res.converged)
        self.assertEqual(res.flows, {})
        self.assertGreater(res.node_residual + res.pipe_residual, 0.0)


class ValidationTest(unittest.TestCase):
    def _write(self, obj):
        fd, path = tempfile.mkstemp(suffix=".json")
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            json.dump(obj, fh)
        self.addCleanup(os.remove, path)
        return path

    def test_self_loop_rejected(self):
        path = self._write({
            "nodes": [{"id": "t", "head": 1.0}, {"id": "a", "demand": 1.0}],
            "pipes": [{"id": "p", "from": "a", "to": "a", "resistance": 1.0}],
        })
        with self.assertRaises(ModelError):
            load_network(path)

    def test_component_without_fixed_head_rejected(self):
        path = self._write({
            "nodes": [{"id": "t", "head": 1.0}, {"id": "a", "demand": 1.0},
                      {"id": "b", "demand": 1.0}, {"id": "c", "demand": 1.0}],
            "pipes": [
                {"id": "p1", "from": "t", "to": "a", "resistance": 1.0},
                {"id": "p2", "from": "b", "to": "c", "resistance": 1.0},
            ],
        })
        with self.assertRaises(ModelError):
            load_network(path)

    def test_negative_resistance_rejected(self):
        path = self._write({
            "nodes": [{"id": "t", "head": 1.0}, {"id": "a", "demand": 1.0}],
            "pipes": [{"id": "p", "from": "t", "to": "a", "resistance": -1.0}],
        })
        with self.assertRaises(ModelError):
            load_network(path)


class CliTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, self.tmp, True)
        self.net_path = os.path.join(self.tmp, "net.json")
        self.sc_path = os.path.join(self.tmp, "sc.json")
        with open(self.net_path, "w", encoding="utf-8") as fh:
            json.dump({
                "nodes": [{"id": "tank", "head": 100.0},
                          {"id": "a", "demand": 0.0}, {"id": "b", "demand": 0.0}],
                "pipes": [
                    {"id": "p1", "from": "tank", "to": "a", "resistance": 1.0},
                    {"id": "p2", "from": "a", "to": "b", "resistance": 2.0},
                ],
            }, fh)

    def _run(self, scens, tag):
        with open(self.sc_path, "w", encoding="utf-8") as fh:
            json.dump({"scenarios": scens}, fh)
        po = os.path.join(self.tmp, f"p{tag}.csv")
        no = os.path.join(self.tmp, f"n{tag}.json")
        rc = main([self.net_path, self.sc_path, "--pipes-out", po, "--nodes-out", no])
        self.assertEqual(rc, 0)
        with open(no, encoding="utf-8") as fh:
            return json.load(fh)

    def test_batch_and_order_independence(self):
        scens = [
            {"name": "low", "demands": {"a": 1.0, "b": 1.0}},
            {"name": "high", "demands": {"a": 3.0, "b": 2.0}},
        ]
        fwd = self._run(scens, "1")
        rev = self._run(list(reversed(scens)), "2")
        f = {s["name"]: s for s in fwd["scenarios"]}
        r = {s["name"]: s for s in rev["scenarios"]}
        for name in ("low", "high"):
            self.assertEqual(f[name]["heads"], r[name]["heads"])
            self.assertAlmostEqual(f[name]["balance_error"], 0.0, places=6)
            self.assertAlmostEqual(
                f[name]["total_supply"], f[name]["total_demand"], places=6)
            self.assertEqual(
                f[name]["boundary_supply"], {"tank": f[name]["total_demand"]})


if __name__ == "__main__":
    unittest.main()
