"""GF(256) arithmetic with the AES polynomial 0x11B and generator 0x02."""

FIELD_POLY = 0x11B
GENERATOR = 0x03
FIELD_SIZE = 256

_EXP = [0] * 512
_LOG = [0] * 256


def _build_tables() -> None:
    x = 1
    for i in range(255):
        _EXP[i] = x
        _LOG[x] = i
        # Multiply by the generator 0x03: x*3 = xtime(x) ^ x, where
        # xtime is multiplication by 2 modulo FIELD_POLY. (0x02 has
        # order 51 mod 0x11B, so it cannot generate the field.)
        x2 = x << 1
        if x2 & 0x100:
            x2 ^= FIELD_POLY
        x = x2 ^ x
    for i in range(255, 512):
        _EXP[i] = _EXP[i - 255]


_build_tables()


def add(a: int, b: int) -> int:
    return a ^ b


def mul(a: int, b: int) -> int:
    if a == 0 or b == 0:
        return 0
    return _EXP[_LOG[a] + _LOG[b]]


def div(a: int, b: int) -> int:
    if b == 0:
        raise ZeroDivisionError("division by zero in GF(256)")
    if a == 0:
        return 0
    return _EXP[_LOG[a] - _LOG[b]]


def pow_elem(a: int, n: int) -> int:
    if n == 0:
        return 1
    if a == 0:
        return 0
    return _EXP[(_LOG[a] * n) % 255]


def inv(a: int) -> int:
    if a == 0:
        raise ZeroDivisionError("inverse of zero in GF(256)")
    return _EXP[255 - _LOG[a]]


def mul_slice(data: bytes, coef: int) -> bytes:
    """Multiply every byte of data by a constant in GF(256)."""
    if coef == 0:
        return bytes(len(data))
    if coef == 1:
        return bytes(data)
    log_c = _LOG[coef]
    exp = _EXP
    log = _LOG
    return bytes(exp[log[b] + log_c] if b else 0 for b in data)
