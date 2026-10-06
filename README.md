# laminate-response

小变形经典层合板理论（CLT）分析工具，仅用 Python 标准库。
比较铺层顺序对面内载荷、弯矩与均匀温变响应的影响，
并通过回积分逐层应力核对整体载荷平衡。

## 运行

```sh
python -m laminate_response input.json -o results.json --csv-dir out
```

- 输入：包含 `materials`、`layup`、`cases` 的 JSON（见 `examples/laminate.json`）。
- 输出：整体结果 JSON（A/B/D、热合力/热弯矩、中面应变与曲率、平衡残差），
  以及每个工况一份逐层上下表面应力 CSV。
- 每个工况独立求解，不沿用上一工况状态；ABD 系统奇异时明确报错退出。

## 单位约定

工具本身不绑定单位制，输入自洽即可。推荐 SI：

- 弹性模量 `E1`、`E2`、`G12`：Pa；`nu12` 无量纲。
- 热膨胀系数 `alpha1`、`alpha2`：1/K；`delta_T`：K（相对无应力温度）。
- 厚度 `thickness`：m；合力 `Nx`、`Ny`、`Nxy`：N/m；弯矩 `Mx`、`My`、`Mxy`：N。
- 输出应力：Pa；中面应变无量纲（`gamma_xy0` 为工程剪切应变）；曲率：1/m。

## 符号约定

- 铺层顺序按厚度坐标从下到上（`layup[0]` 在最底面）。
- 中面取总厚度中心，`z` 向上为正，底面为 `-t/2`。
- 铺层角从全局 x 轴逆时针转向材料 1 轴，单位为度。
- 厚度方向应变：`eps(z) = eps0 + z * kappa`，采用工程剪切应变。
- 合力/弯矩以拉伸中面、使顶面受拉的曲率为正。
- 均匀温差产生热合力 `N^T` 与热弯矩 `M^T`，求解
  `[A B; B D] {eps0; kappa} = {N + N^T; M + M^T}`，
  非对称铺层的伸弯耦合（B 矩阵）不被忽略。

## 应力恢复与平衡核对

- 每层上下表面分别计算全局轴 `(sigma_x, sigma_y, tau_xy)` 与
  材料轴 `(sigma_1, sigma_2, tau_12)` 应力，已扣除自由热应变 `alpha * delta_T`。
- 界面两侧应力不合并，CSV 中相邻两行（上层 top / 下层 bottom）可直接比较突变。
- 将逐层应力沿厚度回积分得到合力与弯矩，与外载之差作为
  `equilibrium_residual` 写入结果 JSON，量级应在浮点误差附近。

## 输入校验

- `E1`、`E2`、`G12` 必须为正，且 `nu12 * nu21 < 1`（刚度正定）。
- 铺层厚度必须为正，铺层数至少为 1，材料名必须已定义。

## 测试

```sh
python -m unittest discover -s tests -v
```

覆盖对称铺层（B = 0、纯拉伸无曲率）、自由热变形（自由应变等于
`alpha * dT`、无内力）以及反转铺层（A、D 不变，B 变号）。
