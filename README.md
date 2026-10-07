# typed-stack-rule-vm

一个用 Go 标准库实现的规则字节码工具：规则在执行前完成全部静态检查，
执行失败时给出可定位的原因（`pc` + 消息）。纯字节码解释，不调用宿主脚本、
不做动态求值。

## 构建与运行

```sh
go build ./cmd/rulevm
./rulevm -program examples/sum.json -input examples/sum-input.json
```

输出为执行摘要 JSON，例如：

```json
{
  "outcome": "ok",
  "value": { "type": "int", "value": 5050 },
  "stop_pc": 19,
  "steps": 1311
}
```

### 命令行参数

| 参数 | 说明 |
| --- | --- |
| `-program` | 程序 JSON 文件路径（必填） |
| `-input` | 输入 JSON 文件路径（缺省为空栈） |
| `-max-steps` | 指令步数上限，达到后以 `limited` 停止（默认 100000） |
| `-trace` | 记录每条指令的位置与执行后的栈 |
| `-trace-limit` | 追踪保留条数上限（0 为不限）；截断不影响程序结果 |

退出码：`0` 正常返回；`1` 用法/IO/输入不匹配；`2` 静态拒绝；
`3` 运行时异常；`4` 步数受限终止。

## 程序格式

```json
{
  "params": ["int"],
  "locals": [{ "name": "acc", "type": "int" }],
  "code": [{ "op": "store", "arg": 0 }]
}
```

- `params`：声明入口栈的类型（自栈底向上），运行前校验输入个数与类型。
- `locals`：预先声明的局部变量，类型为 `int` 或 `bool`。
- `code`：指令数组，`arg` 为整型操作数（立即数、局部变量下标或跳转目标）。

值只有两类：有符号 64 位整数（`int`）与布尔（`bool`）。输入 JSON 形如：

```json
{ "stack": [{ "type": "int", "value": 100 }] }
```

## 指令集

| 指令 | 操作数 | 语义 |
| --- | --- | --- |
| `push_int` / `push_bool` | 立即数（bool 用 0/1） | 入栈 |
| `dup` / `pop` / `swap` | — | 栈操作 |
| `load` / `store` | 局部变量下标 | 读/写局部变量 |
| `add` / `sub` / `mul` / `div` | — | 整数四则运算，除法向零截断 |
| `eq` / `ne` / `lt` / `le` / `gt` / `ge` | — | 整数比较，结果入栈 bool |
| `jmp` / `jmp_if` / `jmp_if_not` | 目标 pc | 无条件/条件跳转（条件须为 bool） |
| `halt` | — | 结束；栈上必须恰好剩一个结果值 |

## 静态校验（执行前完成，拒绝整个程序）

- 指令操作码与操作数合法，跳转目标在代码范围内。
- 对可达控制流做栈类型分析：分支汇合处栈高度与对应类型必须一致。
- 确定赋值分析：局部变量只有在所有到达路径均已赋值时才能读取；
  循环（回边）同样按不动点分析处理。
- `halt` 时栈上必须恰好一个值；`jmp_if*` 条件必须为 bool。

## 运行时语义

- 算术溢出（add/sub/mul/div）与除零立即报错，不绕回、不产生浮点数。
- 越过程序末尾视为运行时异常。
- 达到 `-max-steps` 时以 `limited` 停止并保留停止位置 `stop_pc`，不算正常完成。
- 每次运行使用独立的栈与局部变量副本，多次执行互不污染。

## 结果摘要字段

- `outcome`：`ok` / `fault` / `limited`。
- `value`：正常结束时的结果值。
- `fault`：异常时的 `pc` 与消息。
- `stop_pc` / `steps`：停止位置与已执行步数。
- `trace` / `trace_dropped`：追踪条目与被截断丢弃的条数。

## 示例

`examples/sum.json` 计算 `1..n` 的和（含循环与分支汇合）：

```sh
./rulevm -program examples/sum.json -input examples/sum-input.json -trace -trace-limit 5
```

## 测试

```sh
go test ./...
```

覆盖分支合流栈不一致、单分支赋值后读取、循环内赋值后读取、
溢出/除零/步数受限/越过末尾等静态拒绝与运行时失败场景。
