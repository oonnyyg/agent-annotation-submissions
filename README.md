# survey-margin-raking

仅使用 Python 标准库实现的调查样本分类边际配权（raking / 迭代比例调整）工具。

## 输入

- 样本 CSV：必须包含唯一样本标识列与正的有限初始权重列，以及所有边际涉及的分类字段。
- 目标 JSON：

```json
{
  "margins": [
    {"name": "age", "fields": ["age"],
     "targets": {"young": 150, "old": 150}},
    {"name": "age_x_sex", "fields": ["age", "sex"],
     "targets": {"young||F": 75, "young||M": 75, "old||F": 75, "old||M": 75}}
  ]
}
```

- 边际可对应单字段或字段组合；联合边际的类别键按 `fields` 顺序用 `||` 连接。
- 目标必须非负且有限；所有边际的总量必须相同且为正。
- 重复类别键、重复边际名、样本中出现但目标缺失的类别都会被拒绝，不会静默删除任何冲突目标。

## 迭代顺序与停止规则

- 每轮按配置文件中 `margins` 的顺序逐个边际处理；在同一边际内，把每个类别下样本的权重乘以 `目标总量 / 当前总量`。
- 目标为 0 的类别，其样本权重被归零；目标为正但该类没有样本支持、或当前权重已全为零时，明确报告“不可达”并终止，不会制造新样本。
- 每轮结束后重新计算**所有**边际（而非只检查最后调整的边际），按
  `|实得 - 目标| / max(目标, 1)` 计算残差，全部边际的最大残差不超过容差才判定收敛。
- 达到 `--max-rounds` 上限仍未满足容差时标记为未收敛；每轮的最大残差与最差边际/类别都会记录在诊断中。
- 出现非有限（溢出）权重时立即中止并保留诊断；结构性矛盾下绝不把近似结果标成合格。

## 输出

成功（退出码 0）写入输出目录：

- `weights.csv`：逐样本 `initial_weight`、`final_weight` 及相对初始值的 `multiplier`。
- `margins.json`：每个边际每个类别的目标与实得值、迭代轮数、有效样本量
  `ESS = (Σw)² / Σw²`。

失败（退出码 1）只写 `diagnostics.json`（`status: failed`、失败原因、逐轮诊断、
各边际目标与实得值），不会覆盖已有的成功结果文件。输入非法时退出码 2。

## 用法

```bash
python -m raking --samples samples.csv --targets targets.json --out out \
    --id-field id --weight-field weight --tolerance 1e-6 --max-rounds 100
```

## 测试

```bash
python -m unittest discover -s tests
```

覆盖联合边际、零目标、不可达情形、输入校验与 CLI 端到端行为。
