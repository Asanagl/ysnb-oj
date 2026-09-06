<div align="center">

# YSNB OJ

**You Submit, Never Be rejected.**

一套“完全自研”的自托管竞赛评测系统（OJ），内置从零编写的判题沙箱。

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/Asanagl/ysnb-oj)](https://github.com/Asanagl/ysnb-oj/releases)
[![Docs](https://img.shields.io/badge/Docs-online-646CFF)](https://asanagl.github.io/ysnb-oj/)

[English](README.md) · [简体中文](README_ZH.md) · [问题反馈](https://github.com/Asanagl/ysnb-oj/issues)

<img src="docs/screenshots/home-light.png" alt="首页（亮色主题）" width="49%"/><img src="docs/screenshots/contest-ioi-dark.png" alt="IOI 榜单（暗色主题）" width="49%"/>

</div>

---

## 简介

YSNB OJ 是为一支校内 ACM 集训队从零构建、现已开源的在线评测系统。判题
沙箱完全自研——cgroup v2 + namespaces + seccomp cBPF 白名单 + stage2
重执行，并配有常态化运行的逃逸测试集——而不是套壳 go-judge 或二开现有
评测器。其余部分刻意保持朴素：后端 Go 1.27 + Gin + GORM，前端 Vue 3 +
Tailwind CSS v4 + shadcn-vue，数据层 PostgreSQL 与 Redis。

训练队需要的能力开箱即用：

- ACM 与 IOI 赛制、组队赛、封榜与滚榜揭晓、赛后补题模式
- SPJ 特判与交互题，跑在沙箱化的双向管道里
- 题目包：自有格式，以及 DOMjudge、Hydro 导入
- 题单、小组共享题单、365 天个人热力图
- Codeforces / 洛谷 / AtCoder / 牛客 刷题记录聚合
- WebSocket 实时判题状态：提交页原地从排队推进到出结果，测试点圆点
  随判题逐个点亮
- 公开只读 API（带 API Key）与 cph 桥接（Competitive Companion /
  VSCode cph）
- gRPC 多判题机、找回超管的维护 CLI（oj-cli）、结构化日志与后台日志
  查看器

在一台 2C4G 云主机上的实测：64 连发提交全部 AC，API 延迟 4 ms 以内，
六套 E2E 全绿。

## 快速开始

前置：一台 Linux 服务器（Ubuntu 22.04+ / Debian 12+），Docker ≥ 24，
cgroup v2 已启用（`stat -fc %T /sys/fs/cgroup` 输出 `cgroup2fs`）。

```bash
# 1) 开发机构建前端，并同步整个仓库（含 frontend/dist/）到服务器
cd frontend && npm ci && npm run build

# 2) 服务器上，root 执行
bash deploy/one-click.sh
```

脚本自动生成含随机密钥的 `.env`，用 Docker Compose 启动 postgres / redis
/ api / web，等待 API 就绪，走一遍判题机自检，安装每日备份与每月恢复
演练的 cron，最后打印站点地址与初始管理员密码。脚本幂等，可重复执行。

不想用 Docker？[systemd 裸机部署](docs/operations/deploy.md) 同样完整。
判题二进制仅支持 Linux（沙箱依赖 cgroup v2 与 namespaces），不在开发机
运行——本地开发工作流见
[docs/en/development/handover.md](docs/en/development/handover.md)。

## 文档

完整文档：<https://asanagl.github.io/ysnb-oj/>（**简体中文** ·
[English](https://asanagl.github.io/ysnb-oj/en/)）：部署与运维、选手与
管理员手册、判题管线与沙箱内部实现、公开 API 参考。

## 安全

选手代码完全不可信，沙箱按这个前提设计：只读根 + 敏感路径 tmpfs 遮盖 +
uid 65534 降权 + cgroup v2 限额 + rlimit + wall-clock 看门狗 + seccomp
cBPF 白名单（默认 SIGKILL），判题网络命名空间内无网络。逃逸测试集
（路径穿越、/proc 探测、fork 炸弹、写宿主、联网尝试）作为常规测试资产
运行。**完整判题测试数据永不通过任何公开通道出站。**

审计记录见 [docs/archive/security-audit.md](docs/archive/security-audit.md)。

## 参与贡献

欢迎 Issue 与 PR，流程见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 开源协议

[MIT](LICENSE) © Asanagl
