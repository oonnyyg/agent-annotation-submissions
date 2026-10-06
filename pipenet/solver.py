"""Newton solver for steady-state quadratic pipe networks.

Unknowns: heads at demand nodes and flow in every pipe (signed along the
pipe's positive direction). Equations:

  pipe:   h_start - h_end - r * q * |q| = 0
  node:   (net pipe outflow at node) - demand = 0

Both residual families must fall below their own tolerance for convergence.
"""

from __future__ import annotations

from dataclasses import dataclass, field

from .model import Network, Scenario


@dataclass(frozen=True)
class SolverOptions:
    max_iterations: int = 50
    node_tolerance: float = 1e-9   # max |continuity residual| (flow units)
    pipe_tolerance: float = 1e-9   # max |head-loss residual| (head units)


@dataclass
class SolveResult:
    converged: bool
    iterations: int
    heads: dict[str, float] = field(default_factory=dict)   # all nodes
    flows: dict[str, float] = field(default_factory=dict)   # pipe name -> signed q
    node_residual: float = float("inf")
    pipe_residual: float = float("inf")
    message: str = ""


def _solve_linear(matrix: list[list[float]], rhs: list[float]) -> list[float] | None:
    """Dense Gaussian elimination with partial pivoting. None if singular."""
    n = len(rhs)
    for col in range(n):
        pivot = max(range(col, n), key=lambda row: abs(matrix[row][col]))
        if abs(matrix[pivot][col]) < 1e-14:
            return None
        if pivot != col:
            matrix[col], matrix[pivot] = matrix[pivot], matrix[col]
            rhs[col], rhs[pivot] = rhs[pivot], rhs[col]
        for row in range(col + 1, n):
            factor = matrix[row][col] / matrix[col][col]
            if factor == 0.0:
                continue
            for k in range(col, n):
                matrix[row][k] -= factor * matrix[col][k]
            rhs[row] -= factor * rhs[col]
    x = [0.0] * n
    for row in range(n - 1, -1, -1):
        total = rhs[row] - sum(matrix[row][k] * x[k] for k in range(row + 1, n))
        x[row] = total / matrix[row][row]
    return x


class _Assembler:
    """Builds residuals and the analytic Jacobian for one scenario."""

    def __init__(self, network: Network, scenario: Scenario) -> None:
        self.network = network
        self.demand_nodes = network.demand_nodes
        self.node_index = {node: i for i, node in enumerate(self.demand_nodes)}
        self.n_heads = len(self.demand_nodes)
        self.n_pipes = len(network.pipes)
        self.size = self.n_heads + self.n_pipes
        self.demands = [scenario.demands.get(node, 0.0) for node in self.demand_nodes]
        # Incidence: for each demand node, (pipe_index, +1 if node is start else -1).
        self.incidence: list[list[tuple[int, float]]] = [[] for _ in self.demand_nodes]
        for p, pipe in enumerate(network.pipes):
            if pipe.start in self.node_index:
                self.incidence[self.node_index[pipe.start]].append((p, 1.0))
            if pipe.end in self.node_index:
                self.incidence[self.node_index[pipe.end]].append((p, -1.0))

    def head_of(self, node: str, x: list[float]) -> float:
        index = self.node_index.get(node)
        if index is not None:
            return x[index]
        return self.network.fixed_heads[node]

    def residuals(self, x: list[float]) -> tuple[list[float], float, float]:
        res = [0.0] * self.size
        pipe_res = 0.0
        for p, pipe in enumerate(self.network.pipes):
            q = x[self.n_heads + p]
            value = (
                self.head_of(pipe.start, x)
                - self.head_of(pipe.end, x)
                - pipe.resistance * q * abs(q)
            )
            res[self.n_heads + p] = value
            pipe_res = max(pipe_res, abs(value))
        node_res = 0.0
        for i in range(self.n_heads):
            # Conservation: demand (net withdrawal) + net pipe outflow = 0.
            balance = self.demands[i]
            for p, sign in self.incidence[i]:
                balance += sign * x[self.n_heads + p]
            res[i] = balance
            node_res = max(node_res, abs(balance))
        return res, node_res, pipe_res

    def jacobian(self, x: list[float]) -> list[list[float]]:
        jac = [[0.0] * self.size for _ in range(self.size)]
        for p, pipe in enumerate(self.network.pipes):
            row = self.n_heads + p
            # Regularize the zero derivative of q*|q| at q = 0 so the
            # Jacobian stays nonsingular; residuals are unaffected.
            jac[row][row] = -2.0 * pipe.resistance * max(abs(x[row]), 1e-10)
            start = self.node_index.get(pipe.start)
            if start is not None:
                jac[row][start] = 1.0
            end = self.node_index.get(pipe.end)
            if end is not None:
                jac[row][end] = -1.0
        for i in range(self.n_heads):
            for p, sign in self.incidence[i]:
                jac[i][self.n_heads + p] = sign
        return jac

    def initial_guess(self) -> list[float]:
        x = [0.0] * self.size
        mean_head = sum(self.network.fixed_heads.values()) / len(self.network.fixed_heads)
        for i in range(self.n_heads):
            x[i] = mean_head
        # Seed each pipe with the free-flow estimate from the initial heads
        # so Newton starts on the physical branch.
        for p, pipe in enumerate(self.network.pipes):
            drop = self.head_of(pipe.start, x) - self.head_of(pipe.end, x)
            if drop != 0.0:
                sign = 1.0 if drop > 0.0 else -1.0
                x[self.n_heads + p] = sign * (abs(drop) / pipe.resistance) ** 0.5
        return x


def solve(
    network: Network,
    scenario: Scenario,
    options: SolverOptions | None = None,
) -> SolveResult:
    """Solve one scenario. Never raises for numerical trouble; reports instead."""
    options = options or SolverOptions()
    asm = _Assembler(network, scenario)
    x = asm.initial_guess()

    def converged_result(iteration: int, node_res: float, pipe_res: float) -> SolveResult:
        heads = dict(network.fixed_heads)
        for i, node in enumerate(asm.demand_nodes):
            heads[node] = x[i]
        flows = {
            pipe.name: x[asm.n_heads + p] for p, pipe in enumerate(network.pipes)
        }
        return SolveResult(
            converged=True,
            iterations=iteration,
            heads=heads,
            flows=flows,
            node_residual=node_res,
            pipe_residual=pipe_res,
            message="converged",
        )

    def failure(iteration: int, node_res: float, pipe_res: float, message: str) -> SolveResult:
        return SolveResult(
            converged=False,
            iterations=iteration,
            node_residual=node_res,
            pipe_residual=pipe_res,
            message=message,
        )

    for iteration in range(1, options.max_iterations + 1):
        res, node_res, pipe_res = asm.residuals(x)
        if node_res <= options.node_tolerance and pipe_res <= options.pipe_tolerance:
            return converged_result(iteration - 1, node_res, pipe_res)
        step = _solve_linear(asm.jacobian(x), [-value for value in res])
        if step is None:
            return failure(iteration, node_res, pipe_res, "singular Jacobian matrix")
        if any(abs(value) == float("inf") or value != value for value in step):
            return failure(iteration, node_res, pipe_res, "non-finite Newton step")
        x = [a + b for a, b in zip(x, step)]

    res, node_res, pipe_res = asm.residuals(x)
    if node_res <= options.node_tolerance and pipe_res <= options.pipe_tolerance:
        return converged_result(options.max_iterations, node_res, pipe_res)
    return failure(
        options.max_iterations,
        node_res,
        pipe_res,
        f"iteration limit {options.max_iterations} reached without meeting tolerances",
    )
