# 交互题 / SPJ 约定

## SPJ（特判题）

- checker 是单个 C++17 源文件，在题目管理里保存，判题机按 sha256 缓存编译产物。
- 调用约定（testlib 风格，与 DOMjudge/Kattis 兼容的常见写法）：

```
./checker <input> <answer> <output>
exit 0        -> AC
exit != 0     -> WA（stderr 第一行作为反馈信息展示给用户）
```

## 交互题

- interactor 是单个 C++17 源文件，读入测试输入由 argv 传入：

```
./interactor <input>
```

- 用户程序的 stdin/stdout 与 interactor 的 stdout/stdin 通过管道直连，
  双方都在沙箱内运行（interactor 也禁网、限资源）。
- **退出码约定**：
  - `0`：用户通过；若用户程序自身超时/崩溃，按用户侧状态判。
  - `1`：WA（stderr 第一行作为反馈信息）。
  - 其他：SE（判题系统错误，请在判题机日志查看）。
- 防死锁：任一进程退出，daemon 立即终止另一方；整体受 wall-clock 看门狗约束
  （用户时限 = 题目时限 × 语言倍率；interactor 固定 30s）。
- 测试点 zip 允许没有 `.out` 文件（interactor 自己判断正确性）。

## 与出题工具链对接

题目包导入导出已上线（题目管理页「导入题目包」/编辑器「导出 zip」）：

- **自有格式**（导出即此格式）：`statement.md` + `meta.txt` + `testdata/`
  + `checker.cpp`/`interactor.cpp`（SPJ/交互题自动带上，导入时还原判题模式）。
- **DOMjudge 题包**：解析 `problem.yaml`（name/title、limits.timeout、
  limits.memory）、`statements/*.md`（仅 .md，TeX/PDF 会提示手工补题面）、
  `data/sample`+`data/secret` 的 `.in/.ans`（嵌套目录自动平铺重编号）。
  `validator/checker` 源码会被识别导入为 checker——但注意 DOMjudge validator
  与本 OJ checker 的**参数约定不同**（本 OJ：`./checker <in> <ans> <out>`，
  exit 0=AC），导入后需人工核对。
- **Hydro 题包**：`problem.yaml`（title/time/memory）+ `problem[_zh].md`
  + `testdata/*.in/.out|.ans` + `check/checker.cpp`（识别为 spj）。
- 三种格式自动识别，测试点统一按文件名自然排序重编号；答案缺失的 .in
  整包忽略（交互题请用自有格式或导入后单独传）。
- checker/interactor 支持在编辑器直接上传 `.cpp` 文件（内容进文本框，
  随表单保存）。
