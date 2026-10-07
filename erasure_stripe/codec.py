"""Stripe-level Reed-Solomon encode/decode over GF(256).

A stripe is `stripe_size` input bytes, split into k equal data shards of
`shard_len = ceil(stripe_size / k)` bytes; the final data shard of a short
stripe is zero-padded in memory only. Parity shards are produced by the
systematic generator matrix. Decoding solves a k x k linear system from any
k distinct available shards.
"""

from . import gf256
from .matrix import generator_matrix, solve

MIN_DATA_SHARDS = 2
MAX_DATA_SHARDS = 8
MAX_TOTAL_SHARDS = 12


def validate_layout(k: int, parity: int) -> None:
    if not (MIN_DATA_SHARDS <= k <= MAX_DATA_SHARDS):
        raise ValueError(f"data shards must be {MIN_DATA_SHARDS}..{MAX_DATA_SHARDS}, got {k}")
    if parity < 1:
        raise ValueError("need at least one parity shard")
    if k + parity > MAX_TOTAL_SHARDS:
        raise ValueError(f"total shards must be <= {MAX_TOTAL_SHARDS}, got {k + parity}")


def shard_length(stripe_size: int, k: int) -> int:
    if stripe_size <= 0:
        raise ValueError("stripe size must be positive")
    return (stripe_size + k - 1) // k


class StripeCoder:
    def __init__(self, k: int, parity: int, stripe_size: int):
        validate_layout(k, parity)
        self.k = k
        self.parity = parity
        self.n = k + parity
        self.stripe_size = stripe_size
        self.shard_len = shard_length(stripe_size, k)
        self.generator = generator_matrix(k, parity)

    def encode_stripe(self, stripe: bytes) -> list[bytes]:
        """Encode one stripe (<= stripe_size bytes) into n shards of shard_len."""
        if len(stripe) > self.stripe_size:
            raise ValueError("stripe larger than configured stripe size")
        padded = stripe.ljust(self.k * self.shard_len, b"\x00")
        data = [padded[i * self.shard_len:(i + 1) * self.shard_len] for i in range(self.k)]
        shards = list(data)
        for row in self.generator[self.k:]:
            buf = bytearray(self.shard_len)
            for coef, shard in zip(row, data):
                if coef == 0:
                    continue
                if coef == 1:
                    for j, b in enumerate(shard):
                        buf[j] ^= b
                else:
                    for j, b in enumerate(shard):
                        buf[j] ^= gf256.mul(coef, b)
            shards.append(bytes(buf))
        return shards

    def decode_stripe(self, available: dict[int, bytes], data_len: int) -> bytes:
        """Rebuild the stripe's data bytes from any k distinct shards.

        `available` maps shard index -> shard bytes (each shard_len long).
        Returns exactly `data_len` bytes (padding stripped).
        """
        if data_len < 0 or data_len > self.stripe_size:
            raise ValueError("invalid data length for stripe")
        indices = sorted(available)
        if len(indices) < self.k:
            raise InsufficientShardsError(self.k - len(indices))
        chosen = indices[: self.k]
        decode = solve(self.generator, chosen, self.k)
        cols = [available[i] for i in chosen]
        out = bytearray()
        for row in decode:
            buf = bytearray(self.shard_len)
            for coef, shard in zip(row, cols):
                if coef == 0:
                    continue
                if coef == 1:
                    for j, b in enumerate(shard):
                        buf[j] ^= b
                else:
                    for j, b in enumerate(shard):
                        buf[j] ^= gf256.mul(coef, b)
            out += buf
        return bytes(out[:data_len])


class InsufficientShardsError(Exception):
    def __init__(self, missing: int):
        self.missing = missing
        super().__init__(f"insufficient valid shards: {missing} more required")
