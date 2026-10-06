# blockdelta — 离线差量传输工具

接收端已有旧文件时，只需传输 JSON 差量即可在本地重建新版本。
三个子命令可独立执行，均只依赖 Go 标准库：

```sh
go run ./cmd/blockdelta sig   -block 4096 -in old.bin  -out old.sig.json
go run ./cmd/blockdelta delta -sig old.sig.json -in new.bin -out delta.json
go run ./cmd/blockdelta apply -sig old.sig.json -delta delta.json -old old.bin -out new.bin
```

- `sig`：扫描旧文件，输出签名（每块的偏移、长度、滚动弱校验、SHA-256，
  并绑定整份旧文件的长度与 SHA-256）。
- `delta`：只读签名与新文件（**不读取旧文件**），在新文件任意字节偏移
  查找匹配，输出 JSON 差量（复制块 / 字面量指令序列）。
- `apply`：核对旧文件身份后应用差量到临时文件，全部重建并校验通过
  才替换目标；任何失败都保留已有目标、不修改旧文件。

## 自定义格式

本工具不兼容 rsync 线协议，使用自定义 JSON 格式。

### 签名文件

```json
{
  "format": "blockdelta-sig-v1",
  "blockSize": 4096,
  "fileSize": 10000,
  "sha256": "<旧文件整体 SHA-256, hex>",
  "blocks": [
    {"offset": 0, "length": 4096, "weak": 123456789, "strong": "<块 SHA-256, hex>"},
    {"offset": 4096, "length": 4096, "weak": 0, "strong": "..."},
    {"offset": 8192, "length": 1808, "weak": 0, "strong": "..."}
  ]
}
```

末块可较短；空文件没有块，但仍记录 `fileSize: 0` 与空内容的 SHA-256。

### 差量文件

```json
{
  "format": "blockdelta-delta-v1",
  "sourceSize": 10000,
  "sourceSHA256": "<旧文件整体 SHA-256, hex>",
  "targetSize": 12000,
  "targetSHA256": "<新文件整体 SHA-256, hex>",
  "ops": [
    {"op": "copy", "offset": 0, "length": 4096},
    {"op": "literal", "data": "<base64>"}
  ]
}
```

`copy` 从旧文件 `offset` 处复制 `length` 字节；`literal` 写入内嵌字节。
差量绑定旧文件身份（长度 + SHA-256）与预期新文件长度和摘要。

## 弱校验公式

采用 rsync 风格的 32 位滚动弱校验。对长度 `n` 的窗口 `b[0..n-1]`：

```
a = ( Σ b[i] )            mod 2^16
c = ( Σ (n - i) * b[i] )  mod 2^16
weak = a | (c << 16)
```

窗口右移一字节（移出 `x`，移入 `y`）时可 O(1) 滚动更新：

```
a' = a - x + y
c' = c - n * x + a'
```

弱校验仅用于筛选候选块；候选必须再用 SHA-256 强摘要核对。
相同内容匹配多个旧块时选择最小偏移；当前位置优先最长匹配，
长度相同按旧偏移打破平局；无匹配则前进一字节并把该字节并入字面量。
