# 上线 Readiness 清单（Launch Readiness）

> **存档说明**：本文档是 2026-09 上线阶段的历史快照，只读、不再更新。其中描述的工具链与配置（如密码 SSH、旧部署脚本）可能已被取代；现行口径以文档站首页（docs/index.md）导航的各分册为准。

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
- ✅ **静态页安全头已修复并上线**（2026-09-04）：nginx `add_header` 继承
  陷阱修复（`deploy/nginx.conf`，location 内重复安全头）已同步服务器并
  reload。实测 `/` 与 `/assets/index-*.js` 均带齐 X-Content-Type-Options /
  X-Frame-Options / Referrer-Policy / CSP 四件套，`/api` 无回归
  （后端中间件 + server 级继承，头部重复为既有行为）；浏览器实测 SPA 在
  新 CSP 下正常启动
- ✅ **SSH 加固已执行**（2026-09-04）：root 公钥（ed25519）已装并验证，
  `sshd -T` 实测 `passwordauthentication no` +
  `permitrootlogin without-password`（+ `kbdinteractiveauthentication no`），
  新开密钥连接正常、密码连接实测被拒；配置以 drop-in
  （`sshd_config.d/00-oj-hardening.conf`）落地，回滚件在位。
  **fail2ban 经用户评估后决定不装**（密钥化后密码爆破面已消除，挂账关闭）

## 二、文档

- ✅ README：面向开源访客重写（特性/快速开始/架构/实测数字/实拍截图）
- ✅ docs/ 15 份：技术栈已更新至重构后前端（Tailwind v4 + shadcn-vue），
  handover/maintenance/deploy/api/user-guide/admin-guide 齐备
- ✅ CONTRIBUTING.md / SECURITY.md / LICENSE（MIT）
- ✅ README 截图已落位（2026-09-04，af9439f）：生产站点实拍
  `docs/screenshots/home-light.png`（首页亮色）、`contest-ioi-dark.png`
  （IOI 榜单暗色，总分列 + 格子分）；已核验无 IP/凭据等敏感信息
- 🟡 docs/user-guide.md、admin-guide.md 的界面描述基于旧 UI（Element Plus），
  交互流程不变但视觉已换；建议随使用反馈增量更新

## 三、仓库

- ✅ main 分支干净，全部 PR 已合并，提交历史线性
- ✅ 本地备份 tag `backup/refactor-naive-ui`（未推送，可随时删除）
- ✅ 遗留远端分支已清理：PR 合并时自动删除，远端现仅 `main`（2026-09-04 盘点）

## 四、生产

- ✅ 三服务 active（oj-api / oj-judge / nginx），生产 API 200
- ✅ ufw 白名单生效（22/80/443），8080/9090 仅 127.0.0.1
- ✅ 备份 cron + 每月恢复演练就位
- ✅ 判题机 `OJ_MAX_PARALLEL=1`（3.9GB 单机安全值）
- ✅ 服务器工具链 go1.27.1 / node v22.23.2 在案（重构期间经批准安装，供测试；
  构建类长跑已禁止在该机发起）
- 🔴 TLS/HTTPS 未部署（443 无监听）——公网使用建议尽快上证书（caddy/certbot），
  之后启用 HSTS（nginx.conf 注释位已留）
