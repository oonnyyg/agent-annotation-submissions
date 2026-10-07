# copycodec — Copybook 定长记录与 JSONL 双向转换工具

`copycodec` 依据精简版 COBOL Copybook，在定长二进制记录文件与 JSONL 之间双向转换。
仅使用 Go 标准库，不依赖任何 COBOL 编译器；字段布局与数值精度均可核对。

## 构建与用法

```sh
go build ./cmd/copycodec

copycodec layout <copybook> <layout.json>    # 输出记录布局 JSON（偏移/长度）
copycodec decode <copybook> <input.bin>  <output.jsonl>  # 二进制 -> JSONL
copycodec encode <copybook> <input.jsonl> <output.bin>   # JSONL -> 二进制
```

输出文件原子写入：任何记录转换失败都不会产生半成品输出，输入文件绝不修改。

## 支持的 Copybook 子集

- 自由格式、单个顶层 `01` 记录定义；层级 `01`–`49`，支持任意嵌套分组。
- 固定次数 `OCCURS n TIMES`，可作用于分组或基本项；`01` 级不允许 OCCURS。
- 每个字段必须命名（不支持 `FILLER`）。
- 基本项类型：
  - 文本 `PIC X(n)`：定长 ASCII，编码不足右补空格，解码原样保留空格；
    超长或非 ASCII 字符拒绝。
  - 无符号显示十进制 `PIC 9(i)[V9(s)]`：zoned display，纯 ASCII 数字。
  - 带符号压缩十进制 `PIC S9(i)[V9(s)] COMP-3`：packed decimal。
    只接受 `C`（正）/`D`（负）符号半字节；每个数字半字节必须为 0–9；
    偶数位数的前导填充半字节必须为零；编码零统一写正号 `C`。
- `V` 表示隐含小数位，不占存储字节。

明确拒绝的范围外语法（带定义位置报错）：`REDEFINES`、`OCCURS DEPENDING ON`
（变长数组）、`COMP/BINARY/COMP-1/2/4/5`、`VALUE`、`SIGN`、`JUSTIFIED`、
带符号显示十进制（`PIC S9` 无 COMP-3）、无符号 COMP-3、多个 `01` 记录等。
同级重名、层级不匹配、分组带 PIC、分组被当作标量等均在编译期定位到
`行:列` 报错。

## 数据语义

- 二进制文件由等长记录直接拼接，无换行分隔；末尾不足一条记录即失败。
- JSONL 每行一个记录，结构与分组/数组长度完全一致：分组是 JSON 对象，
  OCCURS 是定长 JSON 数组（长度不符即拒绝），未知字段或缺失字段即拒绝。
- 数值一律以 JSON 字符串形式的十进制数传递（如 `"-12.50"`），内部用
  `math/big` 运算，不经过浮点；超出整数位数或小数精度的值拒绝，绝不截断。
- 解码输出规范十进制字符串：恰好 `scale` 位小数（`scale=0` 时为整数），
  负值带 `-` 前缀，零不带符号。
- 数据错误报告记录序号与字段路径，例如：
  `record 3, field ORDER.LINES[2].PRICE: invalid COMP-3 sign nibble 0xF`。

## 往返语义

- `decode` 后再 `encode` 得到逐字节相同的文件（零的 COMP-3 符号位规范化为 `C`）。
- `encode` 后再 `decode`，数值以规范形式返回（如 `"42"` 而非 `"00042"`，
  `"10"` 在 `V99` 字段中返回 `"10.00"`），文本的空格填充原样保留。

## 布局 JSON

`copycodec layout` 输出总记录长度及每个嵌套字段、数组元素的偏移与长度，
便于与现有作业核对，例如：

```json
{
  "recordLength": 39,
  "root": { "name": "ORDER", "offset": 0, "length": 39, "kind": "group",
    "children": [
      { "name": "CUST-NAME", "offset": 0, "length": 10, "kind": "text" },
      { "name": "LINES", "offset": 15, "length": 20, "count": 2, "kind": "group" }
    ] }
}
```

## 测试

```sh
go test ./...
```

覆盖嵌套重复组、负小数 COMP-3、截断文件、非法符号/填充半字节、
精度与位数超限、非法定义定位等场景。
