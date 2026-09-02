# 贡献指南（CONTRIBUTING）

感谢关注 YSNB OJ（"You Submit, Never Be rejected"）。本文档说明如何搭建开发环境、跑通门禁、提交贡献。

## 开发环境

| 组件 | 要求 |
|---|---|
| Go | ≥ 1.27（后端**仅在 Linux 上构建与运行**） |
| Node.js | ≥ 20（前端工具链，任意平台可用） |
| 后端环境 | **Linux-only**：判题沙箱依赖 cgroup v2 + namespaces，Windows/macOS 不受支持；开发/测试一律对着一个 Linux 部署进行（自有服务器、VM 或已部署实例，见 docs/deploy.md） |
| 数据库 | 生产 PostgreSQL 16 + Redis 7；开发实例建议直接对齐生产 |

工作流：**后端不在开发机本地运行**。前端工具链本地跑，通过 `OJ_DEV_API_TARGET`
把 `/api`（含 WebSocket）代理到 Linux 部署；后端改动在 Linux 侧构建、测试、验证。

```bash
# 前端（本地只跑 vite 工具链，/api 代理到 Linux 部署）
cd frontend && npm ci
OJ_DEV_API_TARGET=http://<linux-deploy-origin> npm run dev   # :5173

# 后端与判题机（仅 Linux + root + cgroup v2）
cd backend && go build ./... && go vet ./... && go test ./...
go run ./cmd/api                        # 或按 docs/deploy.md 部署为 systemd 服务
sudo -E go run ./cmd/judge              # 判题机（Linux）

## 提交前的门禁（必须全绿）

```bash
# 后端
cd backend
go build ./... && go vet ./...
go test ./...

# 前端
cd frontend
npx vue-tsc --noEmit
npm run build
```

改动判题核心（尤其 `pkg/sandbox`）时，额外要求见 `docs/judge-sandbox.md`：
seccomp 白名单的任何改动必须带上 cBPF 解释器测试（`seccomp_sim_test.go`），
并在 Linux 上跑一遍逃逸测试集（`escape_test.go`）。

## 代码规范

- Go：注释解释「为什么」而不是「做什么」；新文件带包级说明。
- 判题语义（判定状态机、榜单计算）改动必须补纯函数单测——
  `aggregate` 与 `computeStandings` 都有既有测试可参照。
- 前端遵循现有 Vue 3 `<script setup>` + TS 结构，API 封装收拢在
  `frontend/src/api/client.ts`。
- 涉及判题公平性的改动（测试数据暴露面、榜单计算）请在 PR 描述里
  明确说明，并跑通 `remote-test/` 相关 E2E。

## E2E 测试

`remote-test/*.mjs` 是对真实 API 的端到端脚本（凭据从环境变量
`.ojenv` 读取，绝不硬编码）。跑法：

```bash
cd remote-test
cp .ojenv.example .ojenv   # 填 BASE / ADMIN_USERNAME / ADMIN_PASSWORD
node m4-import-export-e2e.mjs   # 各模块一套
```

新功能请随 PR 附带对应的 E2E 断言，并在 `docs/e2e-report.md` 追加一节结果。

## 安全问题

**请不要为安全漏洞开公开 issue。** 见 README 安全章节与 SECURITY 联系方式
（见仓库 SECURITY.md，如未提供请通过维护者主页联系方式私下报告）。

## 提交信息

- 一行主题（<72 字符）+ 空行 + 正文说明动机与方案。
- 一次 PR 聚焦一件事；大重构先开 issue 对齐。