"""Safe expression parsing and evaluation with first-order derivatives.

Grammar (recursive descent, no eval/exec anywhere):

    expr    := term (('+' | '-') term)*
    term    := factor (('*' | '/') factor)*
    factor  := ('+' | '-') factor | power
    power   := primary ('^' integer)?
    primary := number | name | '(' expr ')' | func '(' expr ')'
    func    := 'sqrt' | 'exp' | 'ln'

Evaluation uses dual numbers: each value carries its gradient with respect
to all declared input quantities, giving exact first-order sensitivities.
"""

import math

from .errors import ExpressionError

_FUNCS = ("sqrt", "exp", "ln")


class Dual:
    """Value plus gradient vector (length = number of input quantities)."""

    __slots__ = ("value", "grad")

    def __init__(self, value, grad):
        self.value = value
        self.grad = grad


class _Node:
    def eval(self, env, n):
        raise NotImplementedError


class _Const(_Node):
    def __init__(self, value):
        self.value = value

    def eval(self, env, n):
        return Dual(self.value, [0.0] * n)


class _Var(_Node):
    def __init__(self, name):
        self.name = name

    def eval(self, env, n):
        value, index = env[self.name]
        grad = [0.0] * n
        grad[index] = 1.0
        return Dual(value, grad)


class _BinOp(_Node):
    def __init__(self, op, left, right, result, pos):
        self.op = op
        self.left = left
        self.right = right
        self.result = result
        self.pos = pos

    def eval(self, env, n):
        a = self.left.eval(env, n)
        b = self.right.eval(env, n)
        if self.op == "+":
            return Dual(a.value + b.value,
                        [x + y for x, y in zip(a.grad, b.grad)])
        if self.op == "-":
            return Dual(a.value - b.value,
                        [x - y for x, y in zip(a.grad, b.grad)])
        if self.op == "*":
            return Dual(a.value * b.value,
                        [b.value * x + a.value * y
                         for x, y in zip(a.grad, b.grad)])
        if b.value == 0.0:
            raise ExpressionError(
                "division by zero at the nominal point", self.result, self.pos)
        v = a.value / b.value
        return Dual(v, [(x - v * y) / b.value
                        for x, y in zip(a.grad, b.grad)])


class _Pow(_Node):
    def __init__(self, base, exponent, result, pos):
        self.base = base
        self.exponent = exponent
        self.result = result
        self.pos = pos

    def eval(self, env, n):
        a = self.base.eval(env, n)
        k = self.exponent
        if a.value == 0.0 and k <= 0:
            raise ExpressionError(
                f"0^{k} is undefined at the nominal point",
                self.result, self.pos)
        value = a.value ** k
        if a.value == 0.0:
            factor = 1.0 if k == 1 else 0.0
        else:
            factor = k * a.value ** (k - 1)
        return Dual(value, [factor * g for g in a.grad])


class _Func(_Node):
    def __init__(self, func, arg, result, pos):
        self.func = func
        self.arg = arg
        self.result = result
        self.pos = pos

    def eval(self, env, n):
        a = self.arg.eval(env, n)
        x = a.value
        if self.func == "sqrt":
            if x < 0.0:
                raise ExpressionError(
                    "sqrt of a negative number at the nominal point",
                    self.result, self.pos)
            if x == 0.0:
                raise ExpressionError(
                    "sqrt is not differentiable at 0 (derivative is infinite)",
                    self.result, self.pos)
            value = math.sqrt(x)
            factor = 0.5 / value
        elif self.func == "exp":
            value = math.exp(x)
            if not math.isfinite(value):
                raise ExpressionError(
                    "exp overflows at the nominal point", self.result, self.pos)
            factor = value
        else:  # ln
            if x <= 0.0:
                raise ExpressionError(
                    "ln requires a positive argument at the nominal point",
                    self.result, self.pos)
            value = math.log(x)
            factor = 1.0 / x
        return Dual(value, [factor * g for g in a.grad])


class _Token:
    __slots__ = ("kind", "text", "pos")

    def __init__(self, kind, text, pos):
        self.kind = kind
        self.text = text
        self.pos = pos


def _tokenize(source, result):
    tokens = []
    i = 0
    while i < len(source):
        ch = source[i]
        if ch.isspace():
            i += 1
        elif ch.isdigit() or ch == ".":
            j = i
            while j < len(source) and (source[j].isdigit() or source[j] == "."):
                j += 1
            if j < len(source) and source[j] in "eE":
                k = j + 1
                if k < len(source) and source[k] in "+-":
                    k += 1
                if k < len(source) and source[k].isdigit():
                    j = k
                    while j < len(source) and source[j].isdigit():
                        j += 1
            text = source[i:j]
            try:
                value = float(text)
            except ValueError:
                raise ExpressionError(
                    f"invalid number literal '{text}'", result, i + 1)
            if not math.isfinite(value):
                raise ExpressionError(
                    f"number literal '{text}' is not finite", result, i + 1)
            tokens.append(_Token("num", text, i + 1))
            i = j
        elif ch.isalpha() or ch == "_":
            j = i
            while j < len(source) and (source[j].isalnum() or source[j] == "_"):
                j += 1
            tokens.append(_Token("name", source[i:j], i + 1))
            i = j
        elif ch in "+-*/^()":
            tokens.append(_Token(ch, ch, i + 1))
            i += 1
        else:
            raise ExpressionError(
                f"unexpected character '{ch}'", result, i + 1)
    tokens.append(_Token("end", "", len(source) + 1))
    return tokens


class _Parser:
    def __init__(self, source, names, result):
        self.tokens = _tokenize(source, result)
        self.i = 0
        self.names = names
        self.result = result

    def peek(self):
        return self.tokens[self.i]

    def advance(self):
        tok = self.tokens[self.i]
        self.i += 1
        return tok

    def error(self, message, pos=None):
        if pos is None:
            pos = self.peek().pos
        raise ExpressionError(message, self.result, pos)

    def parse(self):
        node = self.parse_expr()
        if self.peek().kind != "end":
            self.error(f"unexpected '{self.peek().text}'")
        return node

    def parse_expr(self):
        node = self.parse_term()
        while self.peek().kind in ("+", "-"):
            op = self.advance()
            right = self.parse_term()
            node = _BinOp(op.kind, node, right, self.result, op.pos)
        return node

    def parse_term(self):
        node = self.parse_factor()
        while self.peek().kind in ("*", "/"):
            op = self.advance()
            right = self.parse_factor()
            node = _BinOp(op.kind, node, right, self.result, op.pos)
        return node

    def parse_factor(self):
        tok = self.peek()
        if tok.kind in ("+", "-"):
            self.advance()
            operand = self.parse_factor()
            if tok.kind == "-":
                zero = _Const(0.0)
                return _BinOp("-", zero, operand, self.result, tok.pos)
            return operand
        return self.parse_power()

    def parse_power(self):
        node = self.parse_primary()
        if self.peek().kind == "^":
            op = self.advance()
            exp_tok = self.peek()
            if exp_tok.kind == "-":
                self.advance()
                sign = -1
                exp_tok = self.peek()
            else:
                sign = 1
            if exp_tok.kind != "num" or not float(exp_tok.text).is_integer() \
                    or any(c in exp_tok.text for c in ".eE"):
                self.error("exponent after '^' must be an integer literal",
                           exp_tok.pos)
            exponent = sign * int(float(exp_tok.text))
            self.advance()
            node = _Pow(node, exponent, self.result, op.pos)
        return node

    def parse_primary(self):
        tok = self.advance()
        if tok.kind == "num":
            return _Const(float(tok.text))
        if tok.kind == "(":
            node = self.parse_expr()
            if self.peek().kind != ")":
                self.error("missing closing parenthesis")
            self.advance()
            return node
        if tok.kind == "name":
            if tok.text in _FUNCS:
                if self.peek().kind != "(":
                    self.error(f"'{tok.text}' must be followed by '('",
                               tok.pos)
                self.advance()
                arg = self.parse_expr()
                if self.peek().kind != ")":
                    self.error("missing closing parenthesis")
                self.advance()
                return _Func(tok.text, arg, self.result, tok.pos)
            if tok.text not in self.names:
                self.error(
                    f"unknown name '{tok.text}' "
                    "(only declared input quantities and constants allowed)",
                    tok.pos)
            return _Var(tok.text)
        self.error(f"unexpected '{tok.text or 'end of expression'}'", tok.pos)


def compile_expression(source, names, result):
    """Parse source into an AST. names: set of declared quantity names."""
    if not isinstance(source, str) or not source.strip():
        raise ExpressionError("expression must be a non-empty string", result)
    return _Parser(source, names, result).parse()
