# jsaudit — JSON 数据批量导入校验工具

为批量导入提供**可定位的拒绝原因**。实现一个**明确限定的 JSON Schema 子集**，
不宣称覆盖完整规范。仅使用 Go 标准库，不联网加载模式。

## 用法

```
jsaudit <schema.json> <instances.jsonl>
```

- 先编译并检查模式；模式有错误时**在校验任何数据前拒绝**，输出一条
  `{"line":0,"kind":"schema",...}` 结果并以退出码 2 结束。
- 模式合法后逐行校验 JSONL，每行输出一条 JSON 结果：
  - 通过：`{"line":3,"ok":true}`
  - 拒绝：`{"line":4,"ok":false,"errors":[{"instance":"#/age","schema":"#/properties/age/type","message":"..."}]}`
  - 坏 JSON 行：`{"line":5,"ok":false,"kind":"parse",...}`，**后续行继续**。
- 退出码：0 全部通过；1 存在实例失败或读取错误；2 模式错误。
- 错误顺序稳定（属性按名称排序、行按顺序处理）。

## 支持的关键词

| 类别 | 关键词 |
| --- | --- |
| 元数据 | `$schema` `$id` `title` `description`（仅注解，不参与校验） |
| 引用 | `$ref`（仅文件内 `#/...` JSON Pointer）、`$defs`、`definitions` |
| 通用 | `type`（单一字符串）、`enum`、`const`、布尔模式（`true`/`false`） |
| 数值 | `minimum` `maximum` `exclusiveMinimum` `exclusiveMaximum`（仅数值形式） |
| 字符串 | `minLength` `maxLength`（按 Unicode 码点计数） |
| 对象 | `properties` `required` `additionalProperties`（布尔或子模式） |
| 数组 | `items`（单一子模式） `minItems` `maxItems` |
| 组合 | `allOf` `anyOf` `oneOf` `not` |

**白名单以外的关键词一律报"unsupported keyword"模式错误**（如 `pattern`、
`format`、`uniqueItems`、`$dynamicRef` 等），不会静默忽略。

## 语义与兼容边界

- **整数按数值判断**：`3.0`、`3e0` 满足 `"type":"integer"`；布尔值不是数字。
- **枚举按 JSON 值结构比较**：数值相等不受写法影响（`1` 与 `1.0` 相同），
  数组/对象按结构递归比较。
- **`required` 只检查存在性**：显式 `null` 不等于缺失（但 `type` 等约束仍会拒绝它）。
- **数值约束只作用于数字**：`{"minimum":2}` 对字符串/布尔不产生错误（与规范一致）。
- **组合结果不依赖分支顺序**；`anyOf`/`oneOf` 成功时，失败分支的错误不会计入结果。
- **`$ref` 旁边的其他约束同时生效**（draft 2019-09+ 行为）。
- **引用限制**：仅文件内 JSON Pointer（含 `~0`/`~1` 转义）；缺失引用与
  循环引用在编译期拒绝；不加载远程/外部文件引用。
- **不支持**：`type` 数组、`items` 元组形式、旧式布尔 `exclusiveMinimum`、
  `patternProperties`、`dependentRequired`、`if/then/else`、`contains` 等。

## 开发

```
go build ./...
go test ./...
```

测试覆盖：引用与组合交叉（`ref_test.go`）、核心校验语义
（`validate_test.go`）、CLI 端到端行为（`cli_test.go`）。
