"""二维方晶格 Ising 模型：能量、磁化与单点翻转能量差。"""

from __future__ import annotations


class Lattice:
    """周期边界的 LxL 方晶格，自旋取 ±1。

    能量约定：每条最近邻键只计一次，E = -J * sum_<ij> s_i s_j - h * sum_i s_i。
    """

    def __init__(self, size: int, coupling: float, field: float, spins: tuple[int, ...]):
        if len(spins) != size * size:
            raise ValueError("初态长度与晶格不符")
        if any(s not in (1, -1) for s in spins):
            raise ValueError("自旋只能取 ±1")
        self.size = size
        self.coupling = coupling
        self.field = field
        self.spins = list(spins)

    @property
    def n_spins(self) -> int:
        return self.size * self.size

    def neighbors(self, index: int) -> tuple[int, int, int, int]:
        """返回周期边界下 index 的四个最近邻。"""
        size = self.size
        row, col = divmod(index, size)
        up = ((row - 1) % size) * size + col
        down = ((row + 1) % size) * size + col
        left = row * size + (col - 1) % size
        right = row * size + (col + 1) % size
        return up, down, left, right

    def total_magnetization(self) -> int:
        return sum(self.spins)

    def total_energy(self) -> float:
        """每条键只计一次（向右、向下两个方向）。"""
        size = self.size
        spins = self.spins
        bond_sum = 0
        for index, spin in enumerate(spins):
            row, col = divmod(index, size)
            right = row * size + (col + 1) % size
            down = ((row + 1) % size) * size + col
            bond_sum += spin * (spins[right] + spins[down])
        return -self.coupling * bond_sum - self.field * sum(spins)

    def flip_delta_energy(self, index: int) -> float:
        """翻转 index 处自旋的能量差 dE = E_new - E_old。"""
        spin = self.spins[index]
        neighbor_sum = sum(self.spins[n] for n in self.neighbors(index))
        return 2.0 * spin * (self.coupling * neighbor_sum + self.field)

    def flip(self, index: int) -> None:
        self.spins[index] = -self.spins[index]
