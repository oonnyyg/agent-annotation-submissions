"""GF(256) arithmetic with the AES polynomial x^8 + x^4 + x^3 + x + 1 (0x11B)
and generator 0x03 (0x02 is not primitive in this field). This is the field
convention recorded in every manifest.
"""

FIELD_POLY = 0x11B
GENERATOR = 0x03
FIELD_SIZE = 256

_EXP = [0] * 512
_LOG = [0] * 256

_x = 1
for _i in range(255):
    _EXP[_i] = _x
    _LOG[_x] = _i
    _x2 = (_x << 1) ^ (FIELD_POLY if _x & 0x80 else 0)
    _x = _x2 ^ _x  # multiply by 0x03: x*3 = x*2 ^ x
for _i in range(255, 512):
    _EXP[_i] = _EXP[_i - 255]


def add(a: int, b: int) -> int:
    return a ^ b


def sub(a: int, b: int) -> int:
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
    return _EXP[(_LOG[a] - _LOG[b]) % 255]


def inv(a: int) -> int:
    if a == 0:
        raise ZeroDivisionError("inverse of zero in GF(256)")
    return _EXP[255 - _LOG[a]]


def pow_(a: int, n: int) -> int:
    if n == 0:
        return 1
    if a == 0:
        return 0
    return _EXP[(_LOG[a] * n) % 255]
