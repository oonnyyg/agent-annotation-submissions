"""Generator matrix construction and inversion over GF(256)."""

from . import gf256


def vandermonde(rows, cols):
    """Vandermonde matrix with evaluation points 0..rows-1 over GF(256)."""
    return [[gf256.pow_elem(r, c) for c in range(cols)] for r in range(rows)]


def mat_mul(a, b):
    out = []
    for row in a:
        new_row = []
        for j in range(len(b[0])):
            acc = 0
            for i, v in enumerate(row):
                if v and b[i][j]:
                    acc ^= gf256.mul(v, b[i][j])
            new_row.append(acc)
        out.append(new_row)
    return out


def mat_inverse(m):
    """Gauss-Jordan inversion over GF(256). Raises ValueError if singular."""
    n = len(m)
    aug = [row[:] + [1 if i == j else 0 for j in range(n)] for i, row in enumerate(m)]
    for col in range(n):
        pivot = None
        for r in range(col, n):
            if aug[r][col] != 0:
                pivot = r
                break
        if pivot is None:
            raise ValueError("matrix is singular")
        aug[col], aug[pivot] = aug[pivot], aug[col]
        inv_p = gf256.inv(aug[col][col])
        aug[col] = [gf256.mul(v, inv_p) for v in aug[col]]
        for r in range(n):
            if r != col and aug[r][col] != 0:
                factor = aug[r][col]
                aug[r] = [v ^ gf256.mul(factor, w) for v, w in zip(aug[r], aug[col])]
    return [row[n:] for row in aug]


def build_generator(k, m):
    """Systematic (n=k+m, k) generator: first k rows are the identity.

    Built from a Vandermonde matrix V by computing V * inverse(V[:k]),
    which yields [I | P]. Parity rows are genuine Reed-Solomon rows, not
    plain XOR, so any k distinct shards suffice to rebuild the stripe.
    """
    if not (2 <= k <= 8):
        raise ValueError("data shards per stripe must be between 2 and 8")
    if m < 1:
        raise ValueError("at least one parity shard is required")
    if k + m > 12:
        raise ValueError("total shards per stripe must not exceed 12")
    v = vandermonde(k + m, k)
    top_inv = mat_inverse([row[:] for row in v[:k]])
    return mat_mul(v, top_inv)
