# Offline SPF Evaluator

面向邮件运维的离线 SPF 核验工具：从一份固定的 DNS 快照出发，解释某个
发送地址为何被允许（pass）、拒绝（fail）或判为配置错误（permerror）。
全程不访问真实 DNS，仅使用 Go 标准库。

## 用法

```sh
# 批量查询
go run ./cmd/spf -snapshot examples/snapshot.json -queries examples/queries.json

# 单次查询
go run ./cmd/spf -snapshot examples/snapshot.json -ip 192.0.2.55 -domain example.com
```

输出为 JSON 数组，每个查询一项，互不影响（某一条的 temperror/permerror
不会中断批次）。

## 快照格式

```json
{
  "zones": {
    "example.com": {
      "TXT": [["v=spf1", " ip4:192.0.2.0/24 -all"]],
      "A": ["203.0.113.10"],
      "AAAA": ["2001:db8::10"]
    }
  },
  "errors": {
    "TXT:flaky.example": "temperror"
  }
}
```

- `zones`：域名 → 记录。域名大小写等价（比较前统一归一化）。
- `TXT`：每条记录是一个**分段数组**；求值前先把分段拼接成完整字符串，
  再从中选择 SPF 记录（恰好一条 `v=spf1`，零条为 none，多条为 permerror）。
- `A` / `AAAA`：地址记录；`a` 机制按发送地址族（IPv4/IPv6）选择对应类型。
- `errors`（可选）：键为 `TYPE:name`，值 `temperror` 表示模拟临时解析故障。

查询文件是 JSON 数组：`[{"ip": "192.0.2.55", "domain": "example.com"}]`。

## 结果语义

| result      | 含义                                             |
|-------------|--------------------------------------------------|
| `pass`      | 命中 `+`（默认）限定符机制，允许发送             |
| `fail`      | 命中 `-` 机制，拒绝                              |
| `softfail`  | 命中 `~` 机制，软拒绝                            |
| `neutral`   | 命中 `?` 机制，或所有规则未命中且无 redirect     |
| `none`      | 域名不存在 SPF 记录（区别于多条记录的 permerror）|
| `permerror` | 永久配置错误：多条 SPF、宏/未支持机制、循环引用、超预算、redirect 目标无记录等 |
| `temperror` | 临时解析故障（快照 `errors` 模拟），可重试       |

每项结果还包含：

- `matched_rule`：命中的机制（如 `-all`、`include:_spf.example.net`）。
- `reason`：错误类结果的传播原因（含子域来源）。
- `lookups_used`：本次判断实际消耗的 DNS 查找项数。
- `evidence`：逐步证据链（TXT 查询、记录选择、每个机制命中/未命中、
  include/redirect 跳转），可完整还原判断过程。

## 支持范围（RFC 7208 子集）

- 机制：`ip4`、`ip6`、`a`、`include`、`all`；修饰符 `redirect=`。
- 限定符：`+`（pass）、`-`（fail）、`~`（softfail）、`?`（neutral）。
- 网段长度：`ip4:1.2.3.0/24`、`ip6::/32`，以及 `a/24//64` 双栈写法。
- **不支持**：宏（`%{...}`）与 `mx`/`ptr`/`exists` 等其他机制。遇到时
  返回 permerror 并注明 out of scope，**不会**当作不匹配跳过。

## 求值规则要点

- `include` 仅在被引用域返回 `pass` 时匹配；子域的 fail/softfail/neutral
  只算未命中，继续父域后续规则，不会直接使父域失败。子域的
  temperror/permerror/none 会向上传播。
- `redirect` 只在所有规则均未命中**且记录中没有 `all`** 时生效；
  redirect 目标无 SPF 记录时返回 permerror。
- 每次顶层判断共享 10 次 DNS 查找预算（空查询最多 2 次），缓存命中
  同样消耗预算；超限返回 permerror。
- include/redirect 循环会被检测并终止为 permerror。

## 测试

```sh
go test ./...
```

覆盖递归 include/redirect 分支、循环终止、预算与空查询边界、
临时/永久错误隔离、TXT 分段拼接与域名大小写等等。
