# composition-balance-transform

仅用 Python 标准库实现的可逆平衡坐标（ILR）转换工具，用于成分比例数据
（compositional data），避免把受总和约束的数据直接当作普通独立变量比较。

## 用法

    python -m composition_balance transform \
        --samples samples.csv --tree tree.json \
        --delta 1e-6 \
        --preprocessed-out preprocessed.csv \
        --coordinates-out balances.csv \
        --reconstructed-out reconstructed.csv \
        --report-out quality_report.json

    python -m composition_balance inverse \
        --coordinates balances.csv --tree tree.json --out inverse.csv

示例数据见 `examples/`。

## 输入格式

- 样本 CSV：首行为表头。若第一列表头为 `sample`/`id`/`name` 等，则该列作为
  样本编号，其余列为成分；否则所有列均为成分，自动生成样本编号。
- 树 JSON：叶子为成分名字符串（或 `{"name": ...}`），内部节点必须是且只能是
  `{"left": ..., "right": ...}`，两个子树均非空。叶子必须与成分名一一对应。

## 处理流程

1. **校验与闭合**：成分值必须有限且非负；每行除以行总和；全零行被拒绝。
2. **零值替换**：零值替换为配置的正数 `--delta`，正值按原比例缩减
   （乘以 `(1 - k*delta)`，`k` 为该行零值个数），使总和仍为一。要求
   `k * delta < 1`，否则报错。无零值的行不做任何修改。
3. **正变换**：每个内部节点产生一个平衡坐标，按**先根顺序**输出：
   `balance = sqrt(l*r/(l+r)) * ln(g_left/g_right)`，其中 `l`、`r` 为左右
   子树的叶子数，`g` 为该侧成分的几何均值。
4. **反变换**：由同一棵树构造正交基 `V`（左叶 `+sqrt(r/(l*(l+r)))`，右叶
   `-sqrt(l/(r*(l+r)))`），对 `V^T b` 做 softmax（减去最大值避免指数溢出），
   得到正且总和为一的组成。重建对象是替换零值后的组成：不恢复原始总量，
   也不把替换值重新改成零。

## 成分顺序与坐标对应关系

- 成分顺序由样本 CSV 表头决定；树叶子按名字与之匹配。
- 坐标顺序为内部节点的先根遍历顺序。输出坐标 CSV 的列名形如
  `balance_0:a|b+c+d`，标明该坐标对应的左右分组；质量报告的
  `balance_order` 字段给出同样的顺序。

## 质量报告

`quality_report.json` 包含：每行零值替换次数、最大重建误差、样本两两平衡
坐标欧氏距离矩阵，以及用中心化对数比（clr）坐标独立计算的距离与平衡坐标
距离的最大差值。交换树的左右分支只会改变坐标符号，重建值与距离不变。

## 误差容限

- 正反变换往返误差（`max_reconstruction_error`）：约 `1e-15` 量级。
- 平衡坐标距离与 clr 距离差值：约 `1e-14` 量级。
- 测试中统一使用 `places=12`（绝对误差约 `5e-13`）作为断言容限。

## 测试

    python -m unittest discover -s tests

覆盖不平衡树（坐标解析式验证）、零值替换（闭合性、无零行不变、
`k*delta >= 1` 拒绝）、正反变换往返、指数溢出防护、交换左右分支的
符号/重建/距离不变性，以及平衡距离与 clr 距离一致性。
