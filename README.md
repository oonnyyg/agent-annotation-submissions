# jsonaudit — JSON Schema 子集实例校验工具

面向批量导入场景的 JSON 数据校验 CLI：对 JSONL 实例逐条校验，给出可定位到
实例位置与模式位置的拒绝原因。**本工具只实现明确限定的 JSON Schema 子集，
不宣称兼容完整 JSON Schema 规范。** 仅使用 Go 标准库。

## 用法

```
jsonaudit -schema schema.json instances.jsonl
```

- 退出码：`0` 全部通过；`1` 存在被拒绝或无法解析的实例行；`2` 模式或文件错误。
- 每行输出 `line N: OK`、`line N: REJECT: ...`（含实例指针与模式指针）或
  `line N: INVALID JSON: ...`。坏 JSON 行单独失败，后续行继续处理。
- 空行跳过。错误顺序稳定（对象属性按名称排序遍历）。

## 支持的关键词

| 类别 | 关键词 |
| --- | --- |
| 类型 | `type`（单一字符串：`null`/`boolean`/`object`/`array`/`number`/`integer`/`string`） |
| 枚举 | `enum`（按 JSON 值结构比较，数值按值相等：`1` 等于 `1.0`） |
| 布尔模式 | 模式本身可以是 `true` / `false` |
| 数值 | `minimum`、`maximum`（闭区间） |
| 字符串 | `minLength`、`maxLength`（按 Unicode 码点计数） |
| 对象 | `properties`、`required`、`additionalProperties`（仅布尔值） |
| 数组 | `items`（统一元素模式）、`minItems`、`maxItems` |
| 组合 | `allOf`、`anyOf`、`oneOf`、`not` |
| 引用 | `$ref`（仅文件内 `#...` JSON Pointer）、`$defs`、`definitions` |

## 语义要点

- `integer` 按数值判断（`3.0` 是整数）；布尔值不是数字。
- 缺失属性与显式 `null` 不混淆：`required` 只看键是否存在。
- 组合结果不依赖分支顺序；成功组合中失败分支的错误不会计入结果。
- `$ref` 旁边的其他约束（兄弟关键词）与引用目标同时生效。
- 引用支持 `~0`/`~1` 指针转义；只接受无环的文件内引用。缺失引用、
  循环引用、远程 URL 在校验任何数据之前拒绝。

## 兼容边界（明确不支持）

- 任何上表之外的关键词（如 `pattern`、`format`、`exclusiveMinimum`、
  `multipleOf`、`contains`、`dependentRequired`、`if/then/else`、
  `patternProperties`、`propertyNames`、`prefixItems`、`const` 等）都会在
  模式编译阶段报 `unsupported keyword`，而不是被静默忽略。
- `type` 不支持数组形式（如 `["string","null"]`）。
- 不联网加载模式，不支持跨文件引用与 `$id`/锚点。
- 数值用有理数精确比较；无法解析为有理数的数字退化为字符串比较。

## 错误分类

- **模式错误**：结构不合法或含不支持的关键词，编译阶段报出并退出码 `2`，
  不校验任何实例。
- **实例错误**：实例不符合规则，逐条报出 `REJECT`，附实例 JSON Pointer
  与模式位置。

## 开发

```
go build ./...
go test ./...
```
