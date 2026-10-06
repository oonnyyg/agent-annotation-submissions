# group_logit — 分组逻辑回归训练工具

仅依赖 Python 标准库（>= 3.12）。在同组样本不跨训练/验证集的约束下，
通过分组交叉验证选择 L2 正则强度，并交付可独立复用的模型 JSON。

## 用法

训练（分组交叉验证 + 全量重拟合）：

```
python -m group_logit train --config config.json
```

用已保存模型预测新 CSV：

```
python -m group_logit predict --model model.json --csv new.csv --out preds.csv
```

## 配置（JSON）

```json
{
  "train_csv": "train.csv",
  "label_column": "y",
  "group_column": "grp",
  "folds": {"groupA": 0, "groupB": 1, "groupC": 2},
  "lambdas": [0.001, 0.1, 10.0],
  "model_out": "model.json",
  "predictions_out": "oof.csv",
  "max_iter": 200,
  "tol": 1e-8
}
```

- `folds` 显式指定每个组所属的验证折；每组只属于一折。CSV 中出现未在
  `folds` 中声明的组会被拒绝。
- `lambdas` 候选正则强度，必须全部为正数。
- `max_iter` / `tol` 可选，默认 200 / 1e-8。

## 数据校验

- 标签只能为 `0` 或 `1`；缺失、非数值或非有限（NaN/Inf）特征一律拒绝。
- 每一折的训练部分（其余折的并集）必须同时包含两类，否则报错。
- 预测 CSV 按列名匹配特征，列顺序改变不影响预测结果；多余列被忽略，
  缺少特征列会报错。

## 模型与优化

- 带截距的逻辑回归，目标函数为
  `平均二元交叉熵 + lambda * ||w||^2 / 2`，截距不惩罚。
- 优化方法：牛顿法（IRLS），Hessian 用部分主元高斯消元求解，
  配合基于方向导数的 Armijo 回溯线搜索，保证目标单调下降。
- 收敛阈值：梯度 L2 范数 `<= tol`，或相对步长 `<= tol`；在 `max_iter`
  内未收敛则明确报错失败，不会输出未收敛的模型。
- 数值稳定：交叉熵用 `log1p(exp(-|z|))` 形式计算，sigmoid 分正负支，
  大分数不会溢出或静默截断。

## 预处理

- 每折仅用该折的训练部分计算特征均值和总体标准差（除以 n），
  常量列缩放因子取 1；验证数据不参与预处理拟合。
- 选定 lambda 后在全量数据上重新拟合预处理和参数，一并存入模型 JSON。

## 强度选择与输出

- 按全部样本的折外（out-of-fold）平均交叉熵选择 lambda，平局取较大值。
  折外损失是真正的留出验证指标，绝不使用训练集成绩代替。
- 训练过程向 stdout 报告每个候选 lambda 的折外损失、迭代次数与收敛状态。
- `predictions_out`：折外预测 CSV，每个训练行恰好出现一次
  （列：row, group, fold, label, probability）。
- `model_out`：模型 JSON，包含特征名、均值、缩放因子、权重、截距、
  选定 lambda 及全部候选的评估记录，可独立用于 `predict`。

## 测试

```
python -m unittest discover -s tests
```

覆盖：组隔离与折外每行一次、常量列缩放、保存/重载与列乱序一致、
非法标签/未知组/非有限特征/非正 lambda 拒绝、大分数数值稳定、
未收敛报错、平局取较大 lambda。
