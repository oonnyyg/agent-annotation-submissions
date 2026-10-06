# UDP NAT 回放器（natreplay）

用本地事件记录解释 UDP 地址转换：端口分配、映射超时与回包过滤。
纯确定性模拟，不发送任何真实网络报文，仅处理 IPv4，不扩展 TCP 或真实防火墙。

## 运行

```
go run ./cmd/natreplay -config examples/config.json -events examples/events.jsonl
go test ./...
```

## 配置（JSON）

```json
{
  "pool": [{"ip": "203.0.113.10", "port_start": 10000, "port_end": 10100}],
  "reserved_ports": [10053],
  "idle_timeout_ms": 60000,
  "inbound_mode": "restricted"
}
```

- `pool`：公网地址与端口区间，可多条。
- `reserved_ports`：保留端口，任何公网地址上的这些端口都不可分配。
- `idle_timeout_ms`：映射空闲期限，仅由成功出站刷新。
- `inbound_mode`：`open`（开放回包）或 `restricted`（仅允许已联系地址端口回包）。

## 事件（JSONL，每行一条）

```json
{"type":"outbound","time":0,"src_ip":"10.0.0.1","src_port":5000,"dst_ip":"8.8.8.8","dst_port":53}
{"type":"inbound","time":100,"src_ip":"8.8.8.8","src_port":53,"dst_ip":"203.0.113.10","dst_port":10000}
{"type":"query","time":500}
```

- `time` 非递减；同刻事件按输入顺序执行。
- `outbound`：内网→外部；`inbound`：外部→公网地址端口；`query`：仅推进清理并记录一条结果。

## 策略

- 每个事件先清理 `expires_at <= time` 的映射，再处理报文；查询同样推进清理。
- 同一内网端点（IP:端口）到不同外部目标复用同一映射；每个公网地址端口只归属一个内网端点。
- 分配端口：优先在池中保留源端口（任一公网地址上空闲即可），否则按
  （公网地址数值、端口）升序选第一个空位；保留端口不可分配。
- 仅成功出站刷新空闲期限并（restricted 模式）把目标地址端口加入允许列表；
  入站不续期、不能创建映射、不能为自己授权；映射释放后旧允许列表一并丢弃。
- 容量耗尽时拒绝新出站（`no_capacity`），现有映射保留；失败报文不添加目标、不刷新期限。
- 入站丢弃区分 `unmapped`（公网端口未映射）与 `filtered`（来源不在允许列表）。

## 输出（JSONL）

每个事件一行结果（`action` 为 `forward` / `drop` / `query`，丢弃附 `reason`），
最后一行为汇总：

```json
{"forwarded_out":1,"forwarded_in":1,"drops":{"filtered":1,"unmapped":1},"active_mappings":[...]}
```

同一输入重复回放得到完全一致的结果。
