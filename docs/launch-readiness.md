# 上线 Readiness 清单（Launch Readiness）

> 状态标记：✅ 完成 · 🟡 待用户动作 · 🔴 已知缺口（带修复方案）
> 本清单对应 2026-09-03 全量审计与上线准备，审计证据见 docs/security-audit.md。

## 一、代码与安全

- ✅ 后端 Linux-only 边界明确：本地不运行 OJ，测试全在云服务器执行
- ✅ Mimosa deep 复扫（前端重构后）：4 findings，全部为已研判的运维参数误报，
  0 业务逻辑候选；封印 `3c9b9439657c…`（scan-2026-09-03T17-40-01.663Z-fb55b2766125）
- ✅ govulncheck：0 个代码可达漏洞
- ✅ npm audit（前端重构后全新依赖树）：0 vulnerabilities
- ✅ 全端口扫描（1-65535，公网实测 83s）：仅 **22 / 80** 开放，无遗漏监听口
  （443 已放行但未部署 TLS，无监听）
- ✅ 公开 API 面枚举：匿名仅可达设计内只读端点；keyed 端点匿名 401；全受保护
  路由 401；第 61 发限流 429 生效
- ✅ 前端 XSS 人工面：markdown-it `html:false`（原始 HTML 转义）、v-html 内容
  全部来自该渲染器、无第三方脚本、无 `target="_blank"` 外链、highlight.js/
  KaTeX 输出转义
- ✅ 凭据扫描：六组真实凭据特征全仓 tracked 文件 0 命中；gitignore 覆盖 8/8
- 🔴 **静态页安全头缺失（P2，修复方案已备待执行）**：nginx `add_header` 继承
  陷阱导致 `/`、`/assets`、`/index.html` 不带 CSP/X-Frame 等头（/api 由后端
  中间件兜底不受影响）。修复已提交在 `deploy/nginx.conf`（location 内重复安全头），
  **待用户批准**后同步到服务器并 reload
- 🔴 **SSH 加固未执行（高优先挂账）**：`sshd -T` 实测 `permitrootlogin yes` +
  `passwordauthentication yes`，无 fail2ban。修复清单在 docs/maintenance.md §7，
  需要用户配合（先配密钥验证，再禁密码）

## 二、文档

- ✅ README：面向开源访客重写（特性/快速开始/架构/实测数字/截图占位）
- ✅ docs/ 15 份：技术栈已更新至重构后前端（Tailwind v4 + shadcn-vue），
  handover/maintenance/deploy/api/user-guide/admin-guide 齐备
- ✅ CONTRIBUTING.md / SECURITY.md / LICENSE（MIT）
- 🟡 README 截图占位 2 张（首页亮色 / IOI 榜单暗色）待用户截图填入
  `docs/screenshots/home-light.png`、`contest-ioi-dark.png`
- 🟡 docs/user-guide.md、admin-guide.md 的界面描述基于旧 UI（Element Plus），
  交互流程不变但视觉已换；建议随使用反馈增量更新

## 三、仓库

- ✅ main 分支干净，全部 PR 已合并，提交历史线性
- ✅ 本地备份 tag `backup/refactor-naive-ui`（未推送，可随时删除）
- 🟡 遗留远端分支 `origin/frontend-redesign`、`origin/fix/contest-detail-tdz`
  （已合并，可安全删除——由上线流程执行）

## 四、生产

- ✅ 三服务 active（oj-api / oj-judge / nginx），生产 API 200
- ✅ ufw 白名单生效（22/80/443），8080/9090 仅 127.0.0.1
- ✅ 备份 cron + 每月恢复演练就位
- ✅ 判题机 `OJ_MAX_PARALLEL=1`（3.9GB 单机安全值）
- 🟡 服务器工具链 go1.27.1 / node v22.23.2（重构期间经批准安装，供测试构建）
- 🔴 TLS/HTTPS 未部署（443 无监听）——公网使用建议尽快上证书（caddy/certbot），
  之后启用 HSTS（nginx.conf 注释位已留）
