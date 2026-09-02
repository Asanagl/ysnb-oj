# Security Policy（安全策略）

## 报告漏洞

**不要为安全漏洞开公开 issue。** 请私下联系维护者（见 GitHub 主页资料），
或邮件报告。我们承诺：

- 48 小时内确认收到
- 7 天内给出初步评估与修复计划
- 修复发布后在 release note 致谢报告者（除非要求匿名）

## 范围内 / 范围外

**在范围内**：
- 自研沙箱（`backend/pkg/sandbox`）的逃逸：越出 cgroup/namespaces/seccomp
  限制读写宿主文件、访问网络、提权
- 判题公平性：通过 API 获取隐藏测试数据、未授权重判、榜单计算篡改
- 认证绕过、越权（RBAC）、注入、SSRF（插件出站通道）

**在范围外**：
- 对生产实例的资源耗尽型 DoS（请勿对任何在线实例压测；本项目自带
  限流与防火墙基线，自部署实例请自行加固）
- 需要判题机 root 或网络内的攻击（部署文档已要求仅开放 80/443/SSH）
- 社会工程学

## 自部署安全基线

自建部署请至少完成：

1. **防火墙**：`ufw default deny incoming`，仅放行 SSH/80/443
   （脚本参考 `remote-test/sec-ufw.mjs`，为幂等可审读脚本）。
2. **不要发布内部端口**：docker-compose 已将 api/gRPC 改为 `expose`；
   systemd 路线确认 8080/9090 仅绑定 127.0.0.1。
3. **SSH 加固**：密钥登录 + 禁密码 + fail2ban（操作清单见
   `docs/maintenance.md` 第七节）。
4. **改默认密钥**：`.env` 由 `deploy/one-click.sh` 用 openssl 现场生成，
   手动部署请逐项确认非占位值。
5. **HTTPS**：公网部署建议 caddy/certbot 上 TLS，并在 nginx 打开
   HSTS（`deploy/nginx.conf` 注释位）。

## 已知设计边界

- 全站默认 HTTP（:80），TLS 由部署者按需添加
- 判题机以 root 运行但沙箱内降权 uid 65534 + 双层命名空间隔离；
  判题机与 API 的信任链基于共享 daemon secret，请勿泄露 `OJ_DAEMON_SECRET`
- 公开 API 只读、限流 60/min；隐藏测试数据不存在任何公开读取路径
  （公平性红线，见 `docs/security-audit.md`）