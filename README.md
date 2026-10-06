# UDP NAT Replay

`natreplay` 是一个 UDP 地址转换（NAT）回放器：读取 JSON 配置与 JSONL 事件记录，
在本地确定性模拟端口分配、映射超时与回包过滤，输出逐事件 JSON 结果及映射汇总。
不发送任何真实网络报文，仅处理 IPv4。

## 运行

```sh
go run ./cmd/natreplay -config config.json -events events.jsonl
```

`-config` 缺省为 `config.json`；`-events` 缺省从标准输入读取。

## 配置格式

```json
{
  "public_addresses": ["203.0.113.10", "203.0.113.11"],
  "reserved_ports": ["0-1023", "65535"],
  "idle_timeout_ms": 30000,
  "mode": "restricted"
}
```

- `public_addresses`：公网地址池，仅 IPv4。
- `reserved_ports`：不可分配端口，元素为 `"port"` 或 `"start-end"`。
- `idle_timeout_ms`：映射空闲期限（毫秒）。
- `mode`：`open`（开放回包）或 `restricted`（仅允许已联系过的 地址:端口 回包）。

## 事件格式（JSONL，每行一个事件）

时间 `time` 非递减（毫秒），同刻事件按输入顺序执行。

```json
{"time":1000,"type":"outbound","src_ip":"10.0.0.1","src_port":5000,"dst_ip":"8.8.8.8","dst_port":53}
{"time":2000,"type":"inbound","src_ip":"8.8.8.8","src_port":53,"dst_ip":"203.0.113.10","dst_port":5000}
{"time":3000,"type":"query"}
```

## 策略

- 每个事件先清理截止时间不晚于当前时刻的映射，再处理报文；`query` 同样推进清理。
- 同一内网端点（IP:端口）访问不同外部目标复用同一映射；每个公网 地址:端口
  只能归属一个内网端点。
- 端口分配优先保留源端口，再选其他空位；同优先级按公网地址数值、端口升序
  裁决；保留端口不可分配。
- 仅成功出站刷新空闲期限；入站不延长期限，也不能创建映射或为自己授权。
- `restricted` 模式允许列表绑定当前映射，只由成功出站添加；映射释放后旧允许
  列表一并丢弃。
- 容量耗尽时拒绝新出站但保留现有映射；失败报文不添加目标、不刷新期限。
- 入站丢弃区分 `no_mapping`（端口未映射）与 `filtered`（来源被过滤）。

## 输出

每个事件输出一行 JSON 结果；末尾输出一行 `"type":"summary"` 汇总，含成功转发
计数与各类丢弃原因计数。重复回放同一输入得到一致结果。

## 范围

仅模拟 UDP over IPv4，不扩展 TCP，也不操作真实防火墙。
