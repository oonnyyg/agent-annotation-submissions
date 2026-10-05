# call-auction-matcher

本地交易演练用的集合竞价工具（仅 Go 标准库）。它累积限价买卖单，
统一确定成交价并分配数量。**仅用于本题模拟**：不连接任何交易所、
行情或资金账户，所有规则都是教学性质的简化实现。

## 运行

```sh
go build -o matcher .
./matcher          # 从 stdin 读 JSONL，向 stdout 写 JSONL
go test ./...      # 运行测试
```

## 协议（JSONL，每行一个请求/响应）

请求：

- `{"type":"order","id":"b1","side":"buy","price":105,"qty":10}`
  下限价单。`id` 全局唯一（撤单后也不可复用），`side` 为 `buy`/`sell`，
  `price` 为正整数分，`qty` 为正整数。成功返回递增受理序号：
  `{"type":"ack","seq":1,"id":"b1"}`。非法请求返回 `error` 且不改变订单簿。
- `{"type":"cancel","id":"b1"}` 撤销未完全成交的订单。
- `{"type":"preview","reference":100}` 预览竞价结果，不消耗任何数量。
- `{"type":"execute","batch":"A1","reference":100}` 执行竞价并扣减成交数量。
  `batch` 为唯一批次号：相同批次号且参数相同的重试返回原结果、不重复成交；
  同批次号改参数会被拒绝。

竞价响应：

```json
{"type":"auction","result":{"priced":true,"price":105,"volume":4,
 "trades":[{"buy_id":"b1","sell_id":"s1","price":105,"qty":4}],
 "remaining":[{"id":"b1","side":"buy","price":105,"qty":10,"seq":1,"remaining":6}]}}
```

`priced:false` 表示最大可成交量为零，无成交价，不强行匹配。

## 定价规则

1. 候选价格只取当前有效订单的报价。
2. 每个价格累计愿意成交的买量（买价 ≥ 候选价）与卖量（卖价 ≤ 候选价），
   两者较小值为可成交量，差的绝对值为失衡量。
3. 依次比较：可成交量最大 → 失衡量最小 → 距给定正参考价最近 → 价格较低者。

## 分配规则

在选定价格上，买单按报价降序、卖单按报价升序排列，同价按受理序号升序；
逐对撮合，全部以同一成交价成交，不超过任一订单的剩余数量。未成交余量
保留在订单簿中，可参与后续批次。

## 示例

```sh
printf '%s\n' \
  '{"type":"order","id":"b1","side":"buy","price":105,"qty":10}' \
  '{"type":"order","id":"s1","side":"sell","price":100,"qty":4}' \
  '{"type":"execute","batch":"A1","reference":105}' | ./matcher
```

## 测试

`auction/` 下的测试覆盖：非法请求不改变订单簿、受理序号与撤单后标识
不可复用、定价平局（参考价距离 / 等距取低价 / 失衡量优先）、零量不定价、
部分成交与价格/序号优先顺序、买卖双方成交总量与逐单数量守恒、
重复执行幂等及同批次改参数拒绝。
