# Offline SPF Evaluator

面向邮件运维的离线 SPF 核验工具。它从一份固定的 DNS 快照出发，按
RFC 7208 解释某个发送地址为何被允许（pass）、拒绝（fail）或判为
配置错误（permerror），并输出逐步证据。整个过程不访问真实 DNS。

## 运行

```sh
go run ./cmd/spf -snapshot examples/snapshot.json -queries examples/queries.json
```

省略 `-queries` 时从标准输入读取查询。输出为 JSON，批量请求互不影响：
单条查询的 temperror/permerror 不会中断其余查询。

## 快照格式

```json
{
  "records": {
    "example.com": {
      "TXT": [["v=spf1", " ip4:192.0.2.0/24 -all"]],
      "A": [["192.0.2.10"]],
      "AAAA": [["2001:db8::10"]]
    }
  },
  "errors": {
    "flaky.test": "SERVFAIL"
  }
}
```

- `records`：域名 → 记录类型（`TXT`/`A`/`AAAA`）→ 应答列表。
  域名匹配大小写不敏感。
- 每条 TXT 应答是**分段数组**，求值前先把分段拼接成完整字符串，
  再从中选择以 `v=spf1` 开头的记录。
- `errors`：域名 → 错误码，模拟临时解析故障（对应 temperror）。
- 域名不存在或没有对应类型时返回空应答，计入空查询（void）预算。

## 查询格式

```json
{"queries": [{"ip": "192.0.2.55", "domain": "example.com"}]}
```

## 结果语义

每条结果包含 `result`、`reason`、`matched_rule`、`budget_used`、
`void_lookups` 和逐步 `trace`。`result` 取值：

- `pass` / `fail` / `softfail` / `neutral`：命中某条机制，按限定符
  `+ - ~ ?`（缺省 `+`）给出结论；`matched_rule` 为命中的原始规则。
- `none`：域名没有发布 SPF 记录。
- `neutral`：所有机制均未命中且没有适用的 `redirect`。
- `temperror`：快照中该域名被标记为临时解析故障。
- `permerror`：永久性配置错误，例如多条 SPF 记录、不支持的机制或
  修饰符、宏、include/redirect 循环、超出查找预算等。

## 支持范围

- 机制：`ip4`、`ip6`（含网段长度）、`a`（按发送地址族选择 A/AAAA，
  支持 `/cidr` 与 `//cidr6` 双网段记法）、`include`、`all`。
- 修饰符：`redirect`。
- 四种限定符均适用于上述机制。
- **不支持**宏（`%{...}`）与 `mx`/`ptr`/`exists`/`exp` 等其它机制或
  修饰符；遇到时返回 permerror 并说明原因，绝不当作不匹配跳过。

## 求值规则要点

- `include` 仅在被引用域返回 `pass` 时匹配；子域的
  `fail`/`softfail`/`neutral` 视为不匹配，父域继续评估后续机制。
  子域的 `none` 是 permerror，`temperror`/`permerror` 沿调用链传播。
- `redirect` 只在所有机制均未命中**且记录中不存在 `all`** 时生效；
  递归判断链完整保留在 trace 中，redirect 目标无 SPF 记录时为
  permerror。
- include/redirect 循环会被检测并终止为 permerror。

## 预算

每次顶层判断共享 10 次 DNS 查找项预算（TXT 记录查询、`a` 的地址
查询、include/redirect 引起的子查询各计 1 次），空查询（void）最多
2 次。缓存命中**仍然消耗预算**。超限返回 permerror。预算按顶层查询
独立计算，批量请求之间互不影响。

## 测试

```sh
go test ./...
```

覆盖递归分支（include/redirect 链、循环终止、错误传播）与预算边界
（第 10/11 次查找、缓存命中计费、空查询上限、跨查询预算隔离）。
