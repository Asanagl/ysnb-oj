# 技术栈总览

> 一页看清整个 OJ 用了什么、版本多少、为什么选它。架构与数据流详见
> `architecture.md`；判题机/沙箱深水区见 `judge-sandbox.md`。

## 后端

| 组件 | 版本 | 用途 / 选型理由 |
|---|---|---|
| Go | **1.27**（go.mod） | 单静态二进制，`CGO_ENABLED=0` 可在 Windows 开发机交叉编译 Linux 判题程序 |
| Gin | v1.10 | HTTP 路由 + 中间件（JWT/banGate/限流/body 上限） |
| GORM | + glebarez/sqlite v1.11 | 生产 PostgreSQL、自测 SQLite 双方言共用 schema；JSON 类字段存 text |
| go-redis | v9 | Redis Stream 判题队列、会话、限流（开发用内存队列替代，`internal/queue` 抽象） |
| gRPC | v1.83 | `proto/oj.proto` 定义 `JudgeRelay.Connect` 双向流（任务下发/结果回传/心跳） |
| golang-jwt v5 + x/crypto | — | JWT 会话（`OJ_JWT_EXPIRE_HOURS` 控制有效期）+ bcrypt 密码 |
| gorilla/websocket | v1.5 | 实时推送：`submission:<id>`、`contest:<id>`、`admin:daemons` 主题 |
| x/sys | v0.47 | 沙箱底层：namespaces/seccomp/cgroup/rlimit 系统调用封装 |

后端包结构（`backend/`）：`cmd/api`、`cmd/judge` 两个入口；`internal/` 下
handler（REST）、judgehub（调度+租约+重排）、daemon（判题机客户端）、
queue、wsq（WS 广播）、auth、model/store（GORM）、config；`pkg/sandbox`
（沙箱，仅 Linux 实现，其他平台 stub）与 `pkg/judge`（判题编排，纯逻辑可测）。

## 前端

| 组件 | 版本 | 用途 |
|---|---|---|
| Vue | ^3.5 | SPA，组合式 API + `<script setup>` |
| TypeScript | ~6.0 | 全量类型化（`vue-tsc --noEmit` 作门禁） |
| Vite | ^8.2 | 构建/开发服务器 |
| Element Plus | ^2.13 | 后台型 UI 组件 |
| Pinia | ^3.0 | 状态（auth store 等） |
| TipTap | ^3.30 | 题面/题解所见即所得编辑器（shallowRef 持有 Editor） |
| axios | ^1.13 | API 客户端（`src/api/client.ts` 集中封装） |

## 基础设施（生产 <your-server-ip>）

| 项 | 生产实际 | 备注 |
|---|---|---|
| 宿主机 | Debian 11，systemd | 单机部署：API + 判题机同机 |
| 数据库 | **PostgreSQL 13.23** | compose 模板默认 postgres:16，裸机为系统源 13 |
| Redis | 127.0.0.1:6379 | 判题队列/限流 |
| Nginx | 系统 apt 版 | 静态资源 `/opt/oj/web` + 反代 `127.0.0.1:8080`（含 WS Upgrade） |
| API | `/opt/oj/oj-api`，`:8080`，gRPC `:9090` | systemd 单元 `oj-api`，env `/opt/oj/oj.env` |
| 判题机 | `/opt/oj/oj-judge` | systemd 单元 `oj-judge`，env `/opt/oj/oj-judge.env`，`OJ_MAX_PARALLEL=2` |
| 数据目录 | `/opt/oj/data`（testdata/代码） | 备份根 `/opt/oj/backup`；判题工作区 `/oj-work` |
| 判题工具链 | build-essential、openjdk-17-jdk-headless、python3 | 新语言先改 `pkg/judge/languages.yaml` 再装工具链 |

## 配置项速查（OJ_* 环境变量）

| 变量 | 侧 | 含义 |
|---|---|---|
| `OJ_LISTEN` / `OJ_GRPC_ADDR` | API | HTTP/gRPC 监听地址 |
| `OJ_MODE` | 双方 | `dev`（SQLite+内存队列）/ `prod` |
| `OJ_DATA_DIR` | 双方 | 测试数据与代码根目录 |
| `OJ_DB_DRIVER` / `OJ_DB_DSN` | API | `postgres`/`sqlite` + DSN |
| `OJ_REDIS_ADDR` | API | Redis 队列 |
| `OJ_JWT_SECRET` / `OJ_JWT_EXPIRE_HOURS` | API | 签名密钥/会话时长 |
| `OJ_DAEMON_SECRET` | API | 判题机共享密钥（API 侧） |
| `OJ_FETCH_BASE` | 双方 | 判题机回源拉取 testdata/代码的 HTTP 基址 |
| `OJ_API_ENDPOINT` | 判题机 | gRPC 目标 `host:9090` |
| `OJ_DAEMON_NAME` / `OJ_DAEMON_TOKEN` | 判题机 | 注册名/共享密钥（=API 侧 secret） |
| `OJ_WORK_ROOT` | 判题机 | 沙箱工作区根（必须不在 /tmp 下） |
| `OJ_MAX_PARALLEL` | 判题机 | 并行判题数 |
| `OJ_LANGUAGES_FILE` | 判题机 | 可选 languages.yaml 覆盖（默认用内嵌） |

## 内建限流

| 维度 | 阈值 |
|---|---|
| 登录 | 10 次/分钟/IP |
| 注册 | 5 次/分钟/IP |
| 提交 | 15 次/分钟/用户 |
| 请求体 | 全局 160MB（测试数据 zip 上限 128MB） |
