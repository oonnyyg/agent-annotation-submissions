# typed-stack-rule-vm

一个仅依赖 Go 标准库的规则字节码工具，用于本地业务批处理。规则以 JSON
程序形式交付，运行前完成全部静态检查，执行失败时给出可定位的指令位置。
不调用宿主脚本，不做任何动态求值。

## 构建与运行

```sh
go build ./cmd/rulevm
./rulevm -program examples/abs.json -input examples/abs.input.json
./rulevm -program examples/sum.json -input examples/sum.input.json -trace 20
```

参数：

- `-program <file>`：程序 JSON（必填）。
- `-input <file>`：输入 JSON，为局部变量提供初始值（可选）。
- `-max-steps <n>`：执行步数上限，达到即受限终止（默认 100000，0 表示不限）。
- `-trace <n>`：记录最近前 n 条逐步追踪（pc、op、执行后栈），0 关闭（默认），
  -1 不限条数。截断追踪不影响程序结果。

退出码：正常结束为 0；静态拒绝、运行时异常或受限终止为 1；CLI 用法错误为 2。

## 程序格式

```json
{
  "locals": ["int", "bool"],
  "code": [
    {"op": "load", "var": 0},
    {"op": "push_int", "value": 10},
    {"op": "gt"},
    {"op": "jmp_if_not", "target": 6},
    {"op": "push_int", "value": 1},
    {"op": "jmp", "target": 7},
    {"op": "push_int", "value": 2},
    {"op": "halt"}
  ]
}
```

- `locals`：预先声明的局部变量类型表，取值为 `"int"`（有符号 64 位）或 `"bool"`。
- `code`：指令数组，跳转目标为指令下标。

输入文件为局部变量提供初值，`null` 表示该变量暂不赋值：

```json
{"locals": [{"type": "int", "int": 100}, null]}
```

布尔值写作 `{"type": "bool", "bool": true}`。

## 指令集

| 指令 | 操作数 | 语义 |
| --- | --- | --- |
| `push_int` / `push_bool` | `value` / `bool` | 常量入栈 |
| `pop` / `dup` / `swap` | — | 栈操作 |
| `load` / `store` | `var` | 读 / 写局部变量 |
| `add` `sub` `mul` `div` `mod` | — | 整数四则/取余，除法向零截断 |
| `eq` `ne` `lt` `le` `gt` `ge` | — | 整数比较，压入布尔 |
| `not` | — | 布尔取反 |
| `jmp` / `jmp_if` / `jmp_if_not` | `target` | 无条件 / 条件跳转（弹出布尔） |
| `halt` | — | 结束，栈上必须恰好剩一个结果值 |

## 静态检查（执行前完成，任何错误拒绝整个程序）

- 指令操作数齐全、局部变量下标与跳转目标在界内、操作码已知。
- 对可达控制流做抽象解释：每条指令的栈类型必须匹配；分支汇合处
  （含循环回边）栈高度与对应类型必须一致。
- 确定赋值分析：局部变量只有在所有到达路径上均已赋值（来自输入或
  `store`）后才允许 `load`；循环按不动点正确分析。
- 每个可达的 `halt` 处栈上必须恰好有一个值。

## 运行时语义

- 整数为有符号 64 位；`add`/`sub`/`mul`/`div`/`mod` 溢出明确报错，
  不绕回、不产生浮点数；除零报错。错误输出包含出错指令位置 `error_pc`。
- 条件跳转只接受布尔值（静态保证）。
- 越过程序末尾（pc 超出指令数组）视为运行时异常。
- 达到 `-max-steps` 时状态为 `limited`，保留停止位置，不算正常完成。
- 每次执行使用独立的局部变量与栈副本，多次运行互不污染。

## 输出

```json
{
  "status": "ok | static_error | runtime_error | limited",
  "value": {"type": "int", "int": 42},
  "error": "...", "error_pc": 3,
  "steps": 8,
  "trace": [{"pc": 0, "op": "push_int", "stack_after": [...]}],
  "trace_truncated": false
}
```

## 测试

```sh
go test ./...
```

覆盖分支合流栈不一致、跨路径确定赋值、循环赋值、算术溢出、除零、
越过末尾、步数受限、追踪截断与多次运行隔离，区分静态拒绝、异常停止
与正常返回三种结局。
