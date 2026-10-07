# 集选电梯离线调度回放器

仅用 Go 标准库实现的单台电梯集选控制（collective control）离线回放器。
读取 JSON 楼层参数与乘客到达记录，输出逐事件日志、每位乘客的行程与运行汇总。
纯仿真，不控制任何真实设备。

## 运行

```sh
go run . examples/sample.json     # 从文件读取
go run . < examples/sample.json   # 或从标准输入读取
go test ./...                     # 运行测试
```

## 输入格式

```json
{
  "minFloor": 1, "maxFloor": 6, "startFloor": 1,
  "capacity": 2,
  "travelTimePerFloor": 2, "dwellTime": 3,
  "passengers": [
    { "id": "p1", "arrive": 0, "from": 1, "to": 5 }
  ]
}
```

- 楼层为闭区间整数范围；`capacity`、`travelTimePerFloor`（每层运行耗时）、
  `dwellTime`（单次停靠耗时）必须为正。
- 乘客 `id` 唯一，`arrive >= 0`，`from != to` 且都在楼层范围内。
- 任一记录非法则整体拒绝（非零退出码，错误打印到 stderr），不产生任何回放。

## 调度规则

- 空闲时选择最早等待者（到达时刻优先，同刻按输入次序），空载直达其起点，
  并采用该乘客的出行方向。
- 运行方向上，若前方（严格越过当前层）还有车内目的地或任意等待起点，
  或当前层有同方向等待者，则继续同向；否则反向。绝不在楼层之间改向，
  反向只发生在楼层处。
- 到站先下客，再按等待队列次序接纳同方向乘客；容量已满则不为其停靠，
  未上车的乘客保留在等待队列中。仅有下客或可接纳乘客时才停靠；
  反向后可在当前层接客。车内目的地一旦登记永不丢失。
- 每次只在开门瞬间上客；停靠期间新到的乘客等待下一次机会。
- 无停靠时只累计楼层移动耗时；全部乘客送达后停止推进。

## 事件次序

同一时刻的事件按如下次序处理：

1. 登记所有 `arrive <= t` 的新到乘客（同刻到达按输入次序入队）。
2. 处理电梯到站：决定停靠、保持/反向/转空闲。
3. 开门瞬间：先下客（记录 `alight=t`），再按队列次序上客（记录 `board=t`）。
4. 停靠耗时 `dwellTime` 后关门出发。

## 输出

- `== event log ==`：`ARRIVE / DISPATCH / REPOSITION / STOP / BOARD / ALIGHT /
  REVERSE / DEPART` 逐事件日志，均带时刻与载客数。
- `== passenger trips ==`：每位乘客的 `board`、`alight`、`wait=board-arrive`
  与 `ride=alight-board`。
- `== summary ==`：乘客数、平均/最大等待、结束时刻、峰值载客与容量。
  任意时刻载客不超过 `capacity`，每位乘客只完成一次行程（程序内置校验，
  违反时非零退出）。

## 复现

```sh
go run . examples/sample.json
```

示例覆盖：同刻到达按输入次序上车、满载跳过 `p3`（t=5 二楼不停）、
顶层反向接 `p4`、空闲空载调度。对应单元测试见 `sim_test.go`
（端点反向、满载滞留、同刻到达、停靠期间到达等待、非法输入拒绝）。
