"""Matrix helpers over GF(256): Vandermonde construction, inversion, and the
systematic generator matrix used by the codec.

Convention: shards are indexed 0..n-1. Row i of the Vandermonde matrix V is
[1, a_i, a_i^2, ..., a_i^(k-1)] with evaluation point a_i = i + 1 (nonzero
field elements, distinct because n <= 12 < 256). The systematic generator is
Concretely G = V * inv(V_top) where V_top is the first k rows of V, so
G[:k] == I and data shards pass through unchanged.
"""

from . import gf256


def vandermonde(n: int, k: int) -> list[list[int]]:
    """n x k Vandermonde matrix with evaluation points 1..n."""
    rows = []
    for i in range(n):
        a = i + 1
        row = [1]
        for _ in range(1, k):
            row.append(gf256.mul(row[-1], a))
        rows.append(row)
    return rows


def mat_mul(a: list[list[int]], b: list[list[int]]) -> list[list[int]]:
    rows, inner, cols = len(a), len(b), len(b[0])
    out = [[0] * cols for _ in range(rows)]
    for i in range(rows):
        for j in range(cols):
            acc = 0
            for t in range(inner):
                acc ^= gf256.mul(a[i][t], b[t][j])
            out[i][j] = acc
    return out


def invert(m: list[list[int]]) -> list[list[int]]:
    """Gauss-Jordan inversion over GF(256). Raises ValueError if singular."""
    size = len(m)
    aug = [list(m[i]) + [1 if j == i else 0 for j in range(size)] for i in range(size)]
    for col in range(size):
        pivot = next((r for r in range(col, size) if aug[r][col] != 0), None)
        if pivot is None:
            raise ValueError("matrix is singular over GF(256)")
        aug[col], aug[pivot] = aug[pivot], aug[col]
        inv_p = gf256.inv(aug[col][col])
        aug[col] = [gf256.mul(v, inv_p) for v in aug[col]]
        for r in range(size):
            if r != col and aug[r][col] != 0:
                factor = aug[r][col]
                aug[r] = [x ^ gf256.mul(factor, y) for x, y in zip(aug[r], aug[col])]
    return [row[size:] for row in aug]


def generator_matrix(k: int, parity: int) -> list[list[int]]:
    """(k+parity) x k systematic generator: top k rows are the identity."""
    v = vandermonde(k + parity, k)
    top_inv = invert([row[:] for row in v[:k]])
    return mat_mul(v, top_inv)


def solve(generator: list[list[int]], row_indices: list[int], k: int) -> list[list[int]]:
    """Given k distinct row indices of the generator, return the decode matrix
    D such that D * (those rows' shard values) reconstructs the k data shards.
    """
    if len(row_indices) != k or len(set(row_indices)) != k:
        raise ValueError("need exactly k distinct shard rows to decode")
    sub = [generator[i] for i in row_indices]
    return invert(sub)
