"""Stripe-level Reed-Solomon encode/decode over GF(256)."""

from . import gf256
from .matrix import build_generator, mat_inverse


def encode_stripe(data_shards, generator):
    """Encode k equal-length data shards into n=k+m shards (systematic)."""
    k = len(data_shards)
    size = len(data_shards[0])
    if any(len(s) != size for s in data_shards):
        raise ValueError("data shards in a stripe must have equal length")
    out = [bytes(s) for s in data_shards]
    for row in generator[k:]:
        parity = bytearray(size)
        for coef, shard in zip(row, data_shards):
            if coef:
                contrib = gf256.mul_slice(shard, coef)
                for i in range(size):
                    parity[i] ^= contrib[i]
        out.append(bytes(parity))
    return out


def decode_stripe(available, k, m):
    """Rebuild all k data shards from any k distinct valid shards."""
    if len(available) < k:
        missing_hint = sorted(set(range(k + m)) - set(available))
        raise ValueError(
            f"need {k} valid shards, have {len(available)}; "
            f"missing shard indexes include {missing_hint}"
        )
    generator = build_generator(k, m)
    size = len(next(iter(available.values())))
    if any(len(s) != size for s in available.values()):
        raise ValueError("shard lengths within a stripe disagree")
    chosen = sorted(available)[:k]
    sub = [generator[i] for i in chosen]
    inv = mat_inverse(sub)
    shards = [available[i] for i in chosen]
    data = []
    for row in inv:
        rebuilt = bytearray(size)
        for coef, shard in zip(row, shards):
            if coef:
                contrib = gf256.mul_slice(shard, coef)
                for i in range(size):
                    rebuilt[i] ^= contrib[i]
        data.append(bytes(rebuilt))
    return data
