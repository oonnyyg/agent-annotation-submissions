# blockdelta — 离线滚动差量传输工具

接收端利用旧文件（基线）在本地重建新版本，只需传输一个 JSON 差量文件。
三个阶段各自独立执行，全部使用 Go 标准库，不兼容 rsync 线协议。

## 用法

```sh
# 1. 在持有旧文件的一端生成签名
go run ./cmd/blockdelta signature -old old.bin -block 4096 -out sig.json

# 2. 在持有新文件的一端，仅凭签名和新文件生成差量（不读取旧文件）
go run ./cmd/blockdelta delta -sig sig.json -new new.bin -out delta.json
#    输出: reused N bytes, literal M bytes

# 3. 在接收端用旧文件应用差量，原子替换目标
go run ./cmd/blockdelta apply -old old.bin -delta delta.json -out new.bin
```

`-block` 必须是正整数。目标路径必须与旧文件路径不同。

## 自定义格式

### 签名（sig.json）

```json
{
  "blockSize": 4096,
  "fileSize": 10000,
  "sha256": "<整份旧文件的 SHA-256, hex>",
  "blocks": [
    { "offset": 0, "length": 4096, "weak": 12345678, "strong": "<块 SHA-256, hex>" }
  ]
}
```

旧文件按 `blockSize` 顺序划分，末块可较短；空文件没有块。签名通过
`fileSize` 与整体 `sha256` 绑定旧文件身份。

### 差量（delta.json）

```json
{
  "blockSize": 4096,
  "oldSize": 10000, "oldSHA256": "<基线摘要>",
  "newSize": 10003, "newSHA256": "<预期新文件摘要>",
  "ops": [
    { "op": "copy", "offset": 0, "length": 4096 },
    { "op": "literal", "data": "<base64>" }
  ]
}
```

`copy` 从旧文件 `offset` 处复制 `length` 字节；`literal` 内嵌原始字节
（JSON 中 base64）。差量同时绑定基线身份（`oldSize`/`oldSHA256`）和预期
新文件身份（`newSize`/`newSHA256`）。

## 弱校验公式

滚动弱校验仿 rsync，对长度 `n` 的窗口 `buf`：

```
a = sum(buf[i])
b = sum((n - i) * buf[i])        i 从 0 到 n-1
weak = (b << 16) | (a & 0xffff)
```

窗口滑动一字节（移出 `xOut`，移入 `xIn`）时 O(1) 滚动更新：

```
a' = a - xOut + xIn
b' = b - n*xOut + a'
```

## 匹配与安全规则

- 在新文件的**任意字节偏移**（不只块对齐位置）用滚动弱校验筛选候选块，
  命中后再核对块级 SHA-256 强摘要；相同内容取最小旧偏移。
- 从左到右扫描：当前位置取最长匹配（整块优先于短末块），平局按旧偏移；
  无匹配前进一字节并累积到相邻字面量中合并；短末块同样可复用。
- 应用前核对旧文件长度与摘要，拒绝越界 `copy`、非法指令、空字面量以及
  操作总长度与 `newSize` 不符的差量。
- 全部重建并通过新文件长度 + SHA-256 校验后，才把目标目录中的临时文件
  重命名替换到目标；任何失败都保留已有目标且不改旧文件。

## 测试

```sh
go test ./...
```

覆盖头部插入、重复块（最小偏移平局）、损坏差量（越界复制 / 长度篡改 /
非法指令 / 摘要不符，且失败不触碰已有目标）、错误基线拒绝、目标与旧文件
同路径拒绝、空文件与短末块复用，以及位移一字节时不得退化为整份字面量。
