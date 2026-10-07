# dns-wire-workbench

离线 DNS 报文工作台（仅 Go 标准库，无网络请求）。用于检查抓包中的
域名压缩与记录边界：把原始二进制报文解码为 JSON，或把受支持的 JSON
文档编码回 wire 格式。

## 用法

```
dnswire decode <input.bin> [output.json]
dnswire encode <input.json> [output.bin]
```

- 输入 `-` 表示 stdin；输出省略或为 `-` 表示 stdout。
- 输出通过临时文件原子写入：解码/编码失败时不会覆盖已有输出文件。
- 失败时 stderr 输出结构化错误并以非零码退出：

```json
{"error":{"offset":27,"reason":"compression pointer loop at offset 12"}}
```

`offset` 为相对报文起始的字节偏移（非错误类问题为 `-1`）。

## 支持范围

- RFC 1035 头部与四个分区（question / answer / authority / additional），
  头部计数必须与实际字节完全吻合，多余尾随字节视为错误。
- 仅 IN 类的 A、CNAME、MX、TXT 做结构化解码；其余类型/类别以 `raw`
  十六进制展示原始 RDATA，重新编码时明确拒绝。
- 保留记录顺序与域名标签大小写。
- 不实现 TCP 长度包装与 DNSSEC。

## 域名与压缩

- 域名表示为 ASCII 标签序列，根名称为空序列。
- 解码识别普通标签与压缩指针（含多级指针），并区分：
  - `occupiedBytes`：名称在当前位置占用的字节数（指针为 2）；
  - `expandedLen`：完全展开后的长度。
  读取下一字段以前者为准，不会因指针跳转错位。
- 校验：标签 ≤ 63 字节、展开名 ≤ 255 字节、非 ASCII 拒绝、截断 /
  循环指针 / 越界指针 / 保留标签类型（0x40、0x80）均报错。
- RDATA 必须恰好落在声明的 `rdlength` 边界内；TXT 的多个字符串
  （含空字符串）逐项保留，不拼接。
- 编码时按实际数据生成计数与长度；相同后缀复用此前出现的偏移生成
  压缩指针。指针只指向此前已编码的标签边界，偏移超出 14 位可表示
  范围时回退为普通标签，不产生非法引用。

## JSON 表达

编码输入（`Document`）：

```json
{
  "header": {"id": 1, "qr": true, "opcode": 0, "aa": false, "tc": false,
             "rd": true, "ra": false, "z": 0, "rcode": 0},
  "questions": [{"name": ["www", "Example", "com"], "qtype": 1, "qclass": 1}],
  "answers": [
    {"name": ["www", "Example", "com"], "type": 1, "class": 1, "ttl": 300,
     "data": "192.0.2.1"},
    {"name": ["Example", "com"], "type": 5, "class": 1, "ttl": 60,
     "data": ["www", "Example", "com"]},
    {"name": ["Example", "com"], "type": 15, "class": 1, "ttl": 60,
     "data": {"preference": 10, "exchange": ["mail", "Example", "com"]}},
    {"name": [], "type": 16, "class": 1, "ttl": 0, "data": ["v=spf1 -all", ""]}
  ],
  "authority": [],
  "additional": []
}
```

- 名称既接受标签数组，也接受解码输出中的
  `{"labels": [...], "occupiedBytes": N, "expandedLen": M}` 对象，
  因此 decode 的 JSON 可直接喂回 encode。
- `data` 形态按类型：A 为点分 IPv4 字符串；CNAME 为名称；MX 为
  `{"preference", "exchange"}`；TXT 为字符串数组。
- 未知类型记录带 `"raw": "<hex>"`，编码时被拒绝。

## 测试

```
go test ./...
```

覆盖：压缩与非压缩往返、多级指针、TXT 多字符串与空字符串、根名称、
大小写保留，以及截断、循环/越界指针、保留标签类型、RDATA 越界、
TXT 越界、展开名超长、尾随字节等恶意边界输入。
