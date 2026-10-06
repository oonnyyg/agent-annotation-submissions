# Layered Soil Storage

静水平衡假设下的分层土壤储水核算工具（仅 Python 标准库）。将各层
van Genuchten 持水曲线转换为剖面储水量，并可按目标总储水量反求地下
水位。本工具**不模拟入渗过程**，只做静水核算。

## 物理假设

- 地表深度为 0，向下为正，单位统一为**厘米 (cm)**。
- 地下水位深度 `d >= 0`。水位以上某深度 `z` 处的吸力水头
  `h = d - z`（cm）；水位及以下（`z >= d`）土壤饱和。
- 每层使用各自的 van Genuchten 持水曲线：

  ```
  theta(h) = theta_r + (theta_s - theta_r) / (1 + (alpha * h)^n)^m,
  m = 1 - 1/n
  ```

## 参数与单位

| 参数        | 含义             | 约束                          | 单位  |
|-------------|------------------|-------------------------------|-------|
| `thickness` | 层厚             | > 0                           | cm    |
| `theta_r`   | 残余体积含水率   | 0 <= theta_r < theta_s <= 1   | -     |
| `theta_s`   | 饱和体积含水率   | 0 <= theta_r < theta_s <= 1   | -     |
| `alpha`     | 尺度参数         | > 0                           | 1/cm  |
| `n`         | 形状指数         | > 1（`m = 1 - 1/n`）          | -     |

所有参数与请求值必须有限。JSON 中土层按**自上而下**顺序给出。

## 储水量积分

每层储水量为含水率沿深度的积分（等效水层厚度，cm），在**土层边界和
地下水位处分段**：水位以下饱和段解析求和（`theta_s * 厚度`），水位以
上非饱和段用自适应 Simpson 积分按容差求积，绝不以层中心值乘厚度代替
积分。输出报告所用容差；积分不收敛时抛出 `IntegrationError`（CLI 以
退出码 3 报告）。

## 水位反求

总储水量随水位加深单调递减。在 `[0, max_depth]` 内二分搜索满足目标
总储水量的**最浅**水位；若目标超出该区间可达水量
`[S(max_depth), S(0)]`，明确报告无解（`solved: false`），不会以边界
值冒充结果。反求仍使用同一分层积分模型，并报告回代残差
`residual_cm`。

## 用法

```bash
# 单次计算（JSON 报告）
python -m soil_storage config.json --water-table 45 --samples 10,30,100

# 反求水位
python -m soil_storage config.json --invert 40 --max-depth 200

# CSV 批量（输入列：kind,depth_cm；kind 为 water_table 或 sample）
python -m soil_storage config.json --csv-in requests.csv --csv-out results.csv
```

配置示例（`config.json`）：

```json
{
  "layers": [
    {"thickness": 30, "theta_r": 0.05, "theta_s": 0.45, "alpha": 0.02, "n": 1.5},
    {"thickness": 50, "theta_r": 0.08, "theta_s": 0.50, "alpha": 0.05, "n": 2.0},
    {"thickness": 20, "theta_r": 0.10, "theta_s": 0.40, "alpha": 0.10, "n": 3.0}
  ],
  "water_table": 45,
  "samples": [10, 30, 100],
  "invert": {"target": 40, "max_depth": 200},
  "tolerance": 1e-9
}
```

CSV 批量规则：

- 每个水位独立计算，输入排序不影响结果（内部排序去重）。
- 剖面采样点恰好落在层界时取**下层**参数；剖面最底点取末层。
- 越界采样点（超出 `[0, 总厚度]`）直接拒绝并报错。

退出码：0 成功；1 输入非法；2 反求无解；3 积分收敛失败。

## 测试

```bash
python -m unittest discover -s tests
```

覆盖层界跨越（水位恰在层界时的分段积分与细网格参考值对比）、全饱和
（水位为 0 时等于 `sum(theta_s * 厚度)`）、反求回代（往返恢复水位且
残差为零）、越界拒绝与 CSV 顺序无关性。
