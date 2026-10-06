"""Safe expression parser: builds an AST without executing any input code.

Grammar:
    expr    := term (('+' | '-') term)*
    term    := factor (('*' | '/') factor)*
    factor  := ('+' | '-') factor | power
    power   := atom ('^' exponent)?        (exponent must be an integer literal)
    atom    := NUMBER | NAME | NAME '(' ')' | '(' expr ')'
    exponent:= ('+' | '-')? INTEGER

Supported functions: sqrt, exp, ln.
"""

from __future__ import annotations

import re
from dataclasses import dataclass

from .errors import ExpressionError

_TOKEN_RE = re.compile(
    r"""
    (?P<number>\d+(?:\.\d*)?(?:[eE][+-]?\d+)?|\.\d+(?:[eE][+-]?\d+)?)
    |(?P<name>[A-Za-z_][A-Za-z0-9_]*)
    |(?P<op>[+\-*/^()])
    |(?P<space>\s+)
    |(?P<bad>.)
    """,
    re.VERBOSE,
)

FUNCTIONS = ("sqrt", "exp", "ln")


@dataclass(frozen=True)
class Token:
    kind: str
    text: str
    pos: int


def tokenize(source: str) -> list[Token]:
    tokens: list[Token] = []
    for match in _TOKEN_RE.finditer(source):
        kind = match.lastgroup
        text = match.group()
        pos = match.start()
        if kind == "space":
            continue
        if kind == "bad":
            raise ExpressionError(f"unexpected character {text!r}", pos)
        tokens.append(Token(kind, text, pos))
    tokens.append(Token("eof", "", len(source)))
    return tokens


# AST nodes: (kind, ...) tuples would do, but small classes read better.
@dataclass(frozen=True)
class Num:
    value: float


@dataclass(frozen=True)
class Var:
    name: str
    pos: int


@dataclass(frozen=True)
class Unary:
    op: str
    operand: object


@dataclass(frozen=True)
class Binary:
    op: str
    left: object
    right: object
    pos: int


@dataclass(frozen=True)
class Pow:
    base: object
    exponent: int
    pos: int


@dataclass(frozen=True)
class Func:
    name: str
    arg: object
    pos: int


class _Parser:
    def __init__(self, tokens: list[Token]):
        self._tokens = tokens
        self._i = 0

    def _peek(self) -> Token:
        return self._tokens[self._i]

    def _next(self) -> Token:
        tok = self._tokens[self._i]
        self._i += 1
        return tok

    def _expect_op(self, op: str) -> Token:
        tok = self._peek()
        if tok.kind != "op" or tok.text != op:
            raise ExpressionError(f"expected {op!r}", tok.pos)
        return self._next()

    def parse(self):
        node = self._expr()
        tok = self._peek()
        if tok.kind != "eof":
            raise ExpressionError(f"unexpected trailing {tok.text!r}", tok.pos)
        return node

    def _expr(self):
        node = self._term()
        while True:
            tok = self._peek()
            if tok.kind == "op" and tok.text in "+-":
                self._next()
                node = Binary(tok.text, node, self._term(), tok.pos)
            else:
                return node

    def _term(self):
        node = self._factor()
        while True:
            tok = self._peek()
            if tok.kind == "op" and tok.text in "*/":
                self._next()
                node = Binary(tok.text, node, self._factor(), tok.pos)
            else:
                return node

    def _factor(self):
        tok = self._peek()
        if tok.kind == "op" and tok.text in "+-":
            self._next()
            operand = self._factor()
            if tok.text == "+":
                return operand
            return Unary("-", operand)
        return self._power()

    def _power(self):
        base = self._atom()
        tok = self._peek()
        if tok.kind == "op" and tok.text == "^":
            self._next()
            exponent = self._integer_literal()
            return Pow(base, exponent, tok.pos)
        return base

    def _integer_literal(self) -> int:
        sign = 1
        tok = self._peek()
        if tok.kind == "op" and tok.text in "+-":
            self._next()
            if tok.text == "-":
                sign = -1
            tok = self._peek()
        if tok.kind != "number" or not re.fullmatch(r"\d+", tok.text):
            raise ExpressionError("exponent after '^' must be an integer literal", tok.pos)
        self._next()
        return sign * int(tok.text)

    def _atom(self):
        tok = self._next()
        if tok.kind == "number":
            return Num(float(tok.text))
        if tok.kind == "name":
            nxt = self._peek()
            if nxt.kind == "op" and nxt.text == "(":
                if tok.text not in FUNCTIONS:
                    raise ExpressionError(f"unknown function {tok.text!r}", tok.pos)
                self._next()
                arg = self._expr()
                self._expect_op(")")
                return Func(tok.text, arg, tok.pos)
            return Var(tok.text, tok.pos)
        if tok.kind == "op" and tok.text == "(":
            node = self._expr()
            self._expect_op(")")
            return node
        if tok.kind == "eof":
            raise ExpressionError("unexpected end of expression", tok.pos)
        raise ExpressionError(f"unexpected {tok.text!r}", tok.pos)


def parse(source: str):
    """Parse an expression string into an AST. Raises ExpressionError."""
    if not source.strip():
        raise ExpressionError("empty expression", 0)
    return _Parser(tokenize(source)).parse()
