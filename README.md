# piecepay — 加工班组计件结算工具

仅使用 Go 标准库。通过 JSONL（每行一个 JSON 操作）从标准输入接收指令，
逐行向标准输出返回 JSON 结果，并把全部状态保存在指定目录的
`state.json` 中，进程重启后结果一致。

## 运行

```sh
go run ./cmd/piecepay --state-dir ./state
```

也可用环境变量 `PIECEPAY_STATE_DIR` 指定状态目录；两者都缺省时
默认使用 `./state`。

## 操作

### register_scheme — 登记/更新计价方案

```json
{"op":"register_scheme","scheme_id":"S1","version":1,
 "tiers":[{"up_to":100,"unit_price":50},
          {"up_to":200,"unit_price":80},
          {"up_to":0,"unit_price":120}]}
```

- `unit_price` 为整数分/件；`up_to` 为该档累计件数上限（含），
  最后一档 `up_to: 0` 表示不限量。
- 同一 `scheme_id` 可登记多个 `version`；已登记批次永远保留
  当时的方案版本，更新方案不影响历史分配。

### register_batch — 登记生产批次

```json
{"op":"register_batch","batch_id":"B1","scheme_id":"S1",
 "month":"2026-01","quantity":120,
 "members":[{"emp_id":"E1","weight":1},{"emp_id":"E2","weight":1}]}
```

- 批次按合格整数件数独立套用**累进阶梯**单价：每一档只对该档
  区间内的件数计价，不会把最后一档价格乘全部件数。
- 批次编号唯一：相同内容重提返回原结果（幂等），内容不同则拒绝。
- 登记失败（校验不通过、月份已封账等）不留下任何部分成员收入，
  也不占用编号。

### void_batch — 整批撤销

```json
{"op":"void_batch","batch_id":"B1","month":"2026-02"}
```

- 在指定的**未封账**月份逐人冲销原分配金额（负数流水），
  严格按原金额冲销，不按当前权重或价格重算。
- 同一批次只能撤销一次。

### close_month — 封账

```json
{"op":"close_month","month":"2026-01"}
```

封账后该月不能新增批次，也不能把撤销记入该月；同内容重提批次
仍返回原结果。

### query_month — 月度结算查询

```json
{"op":"query_month","month":"2026-02"}
```

返回 `report`：`additions`（当期新增）、`reversals`（当期冲销）
与 `net`（逐人净额）。每条流水都带 `batch_id`、`scheme_id`、
`scheme_version`、`quantity`，可追查原批次和方案版本。

## 自定义业务规则

- **累进计价**：`Price` 逐档计算区间件数 × 该档单价之和，
  阶梯边界（如恰好 100 件）属于较低档。
- **分配规则**：总额按正整数权重比例拆分，先取各人份额的整数
  部分，剩余分按余数降序逐分分配，同余数按员工编号字典序；
  个人金额之和恒等于批次总额。
- **撤销规则**：整批撤销，逐人冲销原金额，不重算、不重复。
- **封账规则**：封账月份拒绝一切新增记录（批次与撤销）。

## 测试

```sh
go test ./...
```

覆盖阶梯边界、分币余数、方案版本隔离、幂等与失败回滚、
封账约束、跨月撤销及重开一致性。
