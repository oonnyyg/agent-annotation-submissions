# fontmetrics

一个只依赖 Go 标准库的字体度量工具：读取单个 TrueType sfnt 文件与一份
JSON 文本请求，输出逐字符的字形编号、落笔位置、相邻调整量与整行总前进
宽度。工具只解析度量，不绘制任何轮廓。

## 用法

```sh
go run ./cmd/fontmetrics <font.ttf> <request.json>
```

请求 JSON：

```json
{ "text": "AV😀?", "size": 16, "kern": true }
```

- `text`：要排版的文本（UTF-8）。
- `size`：正字号（像素/点），按 `size / unitsPerEm` 缩放字体单位。
- `kern`：是否启用旧式 `kern` 字偶距调整。

输出 JSON 包含 `unitsPerEm`、逐字形条目（`rune`、`codepoint`、`glyph`、
`missing`、`x`、`adjustment`、`advance`）以及 `totalWidth`。

## 排版模型

每个字形的落笔位置 `x` 等于之前所有字形前进宽度与字偶距调整之和；
`adjustment` 是前一个字形与当前字形之间的 kern 调整，发生在当前字形
定位之前；`totalWidth` 是全部前进宽度与调整量的总和。

## 字体支持与校验

- 仅支持非集合、非可变的 TrueType 字体（sfnt 版本 `0x00010000` 或
  `true`）。拒绝 TTC 集合、CFF（OTTO）以及含 `fvar`/`gvar` 的可变字体。
- 校验表目录：拒绝重复表标签与超出文件范围的表；要求 `head`、`hhea`、
  `hmtx`、`maxp`、`cmap` 存在，并校验 `numberOfHMetrics` 与 `numGlyphs`
  的数量关系及 `hmtx` 长度。
- 字符映射优先选择 Windows Unicode 完整字符集（platform 3, encoding 10）
  的 cmap format 12，否则选择 Windows Unicode BMP（3, 1）的 format 4。
  同一优先项出现重复子表时拒绝加载，绝不拼接两张映射。未映射字符使用
  零号字形（.notdef）并在输出中标记 `missing: true`。
- format 4 正确处理 `idRangeOffset` 的相对偏移与 `idDelta` 增量；
  format 12 支持非 BMP 字符（如 U+1F600）。
- 横向前进宽度来自 `hmtx`；长宽度项（`numberOfHMetrics`）用尽后的字形
  沿用最后一个前进宽度（压缩宽度表）。
- `kern` 仅支持版本零且仅含一个普通水平 format 0 子表；没有 `kern` 表
  时视为零调整，其他形式（版本非零、多个子表、format 非 0、非水平）
  明确报不支持。

## 限制

本工具不做连字、复杂文字塑形（shaping）或自动换行；文本按 rune 顺序
逐一映射到字形并线性累加宽度。

## 测试

```sh
go test ./...
```

测试通过合成最小字体覆盖：缺字、非 BMP 字符、压缩宽度表、format 4/12
选择优先级、重复子表/重复表/越界表拒绝、集合与可变字体拒绝，以及
kern 调整与各种不支持形式。