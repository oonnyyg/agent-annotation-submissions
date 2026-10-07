# zone-access-replay

离线门禁回放工具：根据凭据与读卡事件还原人员所在区域，解释每一次拒绝通行的原因。纯标准库实现，不接入任何真实门禁设备。

## 用法

```sh
go run . -config examples/config.json -events examples/events.json -summary
go run . -config examples/config.json -events examples/events.json -at 2026-03-01T09:30:00Z
go test ./...
```

- 默认输出逐事件判定（ALLOW/DENY + 理由 + 状态变化后的各区域人数）。
- `-summary` 追加最终占用汇总。
- `-at <RFC3339>` 输出该时刻全部事件处理完之后的各区域名单与计数（只重放到该时刻，含等于该时刻的事件）。

## 输入

`config.json`：
- `zones`：区域，`outdoor: true` 表示室外，不限人数；室内区域必须有非负 `capacity`。
- `people`：人员及 `initial_zone` 初始位置。
- `credentials`：凭据，绑定一个 `person`，含 `enabled`、`valid_from`/`valid_to`、可进入的室内区域列表 `zones`。
- `readers`：读卡器，连接明确的 `from`（起点）与 `to`（终点）区域。

`events.json`：事件数组。
- `swipe`：`credential` + `reader`。
- `correct`：人工位置纠正，`person` + `to` + 必填 `reason`。

## 边界语义

- **时间**：全部为 UTC（RFC3339）。事件按时间稳定排序，同一时刻保持输入顺序，先输入者先占用容量。
- **有效期半开区间**：`[valid_from, valid_to)`，`valid_to` 时刻起凭据即失效。
- **单人单位置**：多张凭据可绑定同一人，但人只有一个已确认位置；任何一张卡的事件都基于该位置判断。换卡或停用卡不会自动移走人员。
- **通行检查顺序**：凭据存在 → 读卡器存在 → 凭据启用 → 有效期 → 人位于读卡器起点 → 目标权限（仅室内目标需要，退出到室外不检查）→ 目标容量。任一失败即拒绝并给出理由。
- **原子性**：拒绝不改变任何位置或人数；成功通行同时更新起点与终点两侧计数。
- **人工纠正**：必须带理由，仍需满足目标容量，记录前后位置；纠正后的位置成为后续事件的判断基准。
- **配置校验**：回放开始前拒绝缺失引用（未知区域/人员/读卡器）、重复 ID、负容量、空有效期、初始位置超容量等非法配置。
- **名单一致性**：`-at` 名单与 `-summary` 汇总由同一回放引擎产生，计数与逐事件回放过程一致。
