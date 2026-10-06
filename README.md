# ntp-exchange-auditor

离线 NTP 时间同步审计工具：从已记录的请求/响应报文还原时钟偏差，
区分坏样本与真正可用的校时依据。**只读分析**——不发送任何报文，
也不修改系统时钟。仅使用 Go 标准库。

## 用法

```sh
go run ./cmd/ntpaudit [-window N] [records.jsonl]   # 缺省从标准输入读取
```

`-window` 指定每个对端保留的最近有效样本数（默认 8）。

输入为 JSONL，每行一条交换记录：

```json
{"peer":"ntp1.example","request":"1b...hex...","response":"1c...hex...",
 "sent_at":"2026-01-01T12:00:00Z","received_at":"2026-01-01T12:00:00.08Z"}
```

- `peer`：对端标识。
- `request` / `response`：十六进制编码的 48 字节 NTP 报文。
- `sent_at` / `received_at`：本机发出请求、收到应答的本地时间（RFC3339）。

输出分两部分：逐条分析（`VALID` / `REJECT <原因>` / `DUPLICATE`）
和按对端的汇总（当前偏差、入选行号、窗口大小、窗口抖动）。
示例数据见 `testdata/sample.jsonl`。

## 时间表示

NTP 时间戳为 32 位秒（1900 年起算，约 136 年一个年代）加 32 位小数
（2^-32 秒）。报文中只有 32 位秒，年代信息丢失；工具以记录中的本地
发送时间 `sent_at` 作为近似 UTC 基准，把每个时间戳展开到**距离基准
最近的年代**。若某时间戳恰好距基准半个年代（2^31 秒），两个年代等距，
无法判定，按 `era-ambiguous` 拒绝。

全部时间运算使用整数定点（2^-32 秒为单位），不经过浮点，因此跨越
2036-02-07 年代回绕时顺序与亚秒小数差都保持正确。汇总阶段的抖动
计算只对毫秒级小量使用浮点，不涉及大时间戳。

## 报文校验与拒绝原因

仅接受**无扩展字段的 48 字节 NTPv4** 报文：请求须为 mode 3（client），
响应须为 mode 4（server）。响应校验包括：

- `bad-version` / `bad-mode`：版本或模式不符。
- `unsynchronized`：Leap Indicator 为 3（时钟未同步）。
- `kiss-of-death`：层级 0 控制报文，单独报告并附控制码（如 `RATE`、`DENY`）。
- `bad-stratum`：层级 ≥ 16（保留/无效）。
- `origin-mismatch`：响应起源时间戳与请求发送时间戳不一致（非本请求的应答）。

样本级拒绝：

- `invalid-sample`：服务器发送时间早于其接收时间，或算出的往返延迟为负。
- `era-ambiguous`：年代展开等距歧义。
- `bad-record` / `bad-request` / `bad-response` / `bad-json`：记录或报文无法解析。

## 滤波规则

- 按四时间戳公式计算服务器相对本机的偏差与往返延迟：
  `offset = ((t2-t1)+(t3-t4))/2`，`delay = (t4-t1)-(t3-t2)`。
- 同一请求（按请求发送时间戳判定）只收录**首个有效应答**；后续重复
  应答报告为 `DUPLICATE` 并忽略。
- 每个对端维护最近 `-window` 个有效样本；错误记录与重复应答**不会**
  挤出窗口中的旧有效样本。
- 当前偏差取窗口中**延迟最小**的样本；延迟相同取**最新**者。
- 窗口抖动为其他窗口样本相对入选偏差的均方根（RMS）；单样本窗口
  抖动为零。

## 开发

```sh
go test ./...
```

重新生成示例数据：`GEN_SAMPLE=1 go test ./internal/audit -run TestGenSample`
（Windows PowerShell: `$env:GEN_SAMPLE=1; go test ./internal/audit -run TestGenSample`）。
