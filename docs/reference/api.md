# 外部 API 文档（v1 · /api/v1/public/*）

> 面向机器人、数据看板、CLI/IDE 工具和 cph 集成的只读公开 API。
> 与站内 JWT 接口完全隔离：`/public/*` 匿名可读，受保护端点用 API Key。
>
> **本文档边界**：只覆盖 `/public/*` 公开 API 与 M5 集成端点（插件 / 外部题
> 导入 / 共享导出 / IOI / 外部刷题）。登录态业务接口与管理端接口
> （如 `GET /admin/logs`）不在本文档范围，管理端能力见
> `guide/admin-guide.md`。

## 鉴权

| 方式 | 说明 |
|---|---|
| 匿名 | 直接调用，读接口全部可用，共享一个每分钟 60 次的限流桶 |
| API Key | 请求头 `X-API-Key: ojk_xxx`（无法设头的工具可用 `?api_key=`）；独立限流桶（60/分钟），有最近使用审计 |

Key 管理：管理后台 → **API 密钥** 页生成/吊销。原始 Key 只在生成时显示一次，
数据库只存哈希；吊销立即生效。

限流：HTTP 429 + `{"error":"API 限流：每分钟最多 60 次请求"}`。

## 端点一览

### GET /api/v1/public/users/:id

用户做题情况。`:id` 支持数字 id 或用户名。

```bash
curl http://<host>/api/v1/public/users/admin
```

响应（节选）：

```json
{
  "user": {"id": 1, "username": "admin", "nickname": "Admin"},
  "by_status": {"AC": 12, "WA": 3},
  "ac_problems": 10,
  "tried_problems": 14,
  "recent_ac": [
    {"problem_id": 1, "submission_id": 42, "time_ms": 14, "language": "cpp", "at": "2026-09-01T12:00:00Z"}
  ],
  "activity": [{"date": "2025-09-02", "submissions": 0, "ac": 0}]
}
```

- `recent_ac`：每题**首次 AC**，最新在前，最多 20 条（与站内个人主页口径一致）
- `activity`：连续 365 天的每日桶（零填充，最老在前），直接喂热力图组件

### GET /api/v1/public/problems

公开题目目录（隐藏题与比赛独立副本永不出现）。`?q=` 标题搜索，
`?page=&size=` 分页（size ≤ 100）。

```bash
curl "http://<host>/api/v1/public/problems?page=1&size=20"
```

响应：`{"total": n, "items": [{"id","title","time_limit_ms","mem_limit_mb","judge_mode","tags":[],"sample_count"}]}`

### GET /api/v1/public/problems/:id/samples  （需 API Key）

题面**公开样例**（结构化 JSON）。完整判题测试数据（.in/.out 全集）**永不**
通过公开 API 暴露——这是判题公平性的红线。

```bash
curl -H "X-API-Key: ojk_xxx" http://<host>/api/v1/public/problems/1/samples
```

响应：`{"problem_id","title","time_limit_ms","mem_limit_mb","samples":[{"input","output"}]}`

### GET /api/v1/public/problems/:id/cph  （需 API Key）

直接可导入 **cph**（VSCode Competitive Programming Helper）的题目 JSON：

```bash
curl -H "X-API-Key: ojk_xxx" http://<host>/api/v1/public/problems/1/cph
```

响应即 cph 的导入格式：`{"name","group","url","timeLimit","memoryLimit","tests":[{"input","output"}]}`。
两种接入方式：

1. 你的工具拉取该 JSON 后写入 cph 的导入入口；
2. 想要「cph 里粘题号自动建题」：写一个小工具（或 qqcot 集成）监听本地
   Competitive Companion 回推（cph 默认监听 127.0.0.1:4244 的浏览器回推），
   收到题号后调本端点、把 JSON 转成 Companion 的回推对象即可与 cph 无缝对接。
   注意 cph 无公开远程源协议，纯服务端无法主动推送进 cph。

## 错误码

| 状态 | 含义 |
|---|---|
| 400 | 参数错误 |
| 401 | 缺 Key / Key 无效或已吊销（仅受保护端点） |
| 404 | 用户或题目不存在（含隐藏题/比赛题——语义上不暴露存在性） |
| 429 | 限流（60 次/分钟/桶） |

## 站内集成接口（M5，JWT 鉴权）

以下端点走普通登录态（非 /public/*），供前端与机器人代理调用。

### 插件系统（编译期注册表）

| 端点 | 权限 | 说明 |
|---|---|---|
| `GET /admin/plugins` | setter+ | 已注册插件清单 + webhook 目标 |
| `GET /admin/plugins/sources` | admin+ | 平台 id 列表（爬虫 / 刷题适配器） |
| `PUT /admin/hooks/:name` | admin+ | 设置 webhook 目标 `{"url": "https://…"}`（SSRF 校验） |
| `DELETE /admin/hooks/:name` | admin+ | 删除目标 |

判题完成后向所有 webhook 目标 POST JSON：
`{"id","status","score","time_ms","memory_kb","problem_id","contest_id","user_id","at"}`
（score 为 IOI 部分分，ACM 恒 0）。QQ 机器人等第三方只需挂一个 URL。

### 外部题面导入（题库爬取）

| 端点 | 权限 | 说明 |
|---|---|---|
| `POST /external/problems/preview` | setter+ | `{"source":"codeforces","external_id":"1900A"}` → 预览元数据，不落库 |
| `POST /external/problems/import` | setter+ | 同参数 + 可选 `visibility`（默认 members）→ 建题 |

导入题仅含**题面 + 公开样例**（外部平台拿不到完整测试数据），Source 标注
`[外部训练题·平台 ID]`，默认仅登录可见。补齐测试数据后方可用于正式评测。

### 共享导出（题库共享，红线保护）

`GET /problems/:id/export?shared=1` —— 返回**不含完整测试数据**的共享包：
statement.md + meta.txt + 公开样例生成的 testdata/。隐藏数据与 checker.cpp
永不通过该通道出站（判题公平性红线）；站内完整导出不带 `shared` 参数，行为不变。

### IOI 赛制

- 建比赛：`POST /contests` 传 `"mode": "ioi"`（创建后不可改）。ACM 比赛不传即默认 acm。
- 分值表：`PUT /problems/:id/case-scores` `{"scores":{"1":30,"2":70}}`（setter+），
  `GET` 读回。分值随挂题副本一起克隆。
- 判题：每测试点 AC 得该点分值，提交 score = Σ 通过点分值；verdict 仍按
  ACM 语义（首错决定 WA/TLE/…，全对 AC），榜单按每题**历史最高分**求和排名。
- 榜单字段复用：`penalty_ms` 在 IOI 比赛中是**总分**，cell 的 `solved_ms`
  是该题最佳得分；前端 ScoreBoard 按 `contest.mode` 切换表头。

### 外部刷题统计（刷题报表）

| 端点 | 说明 |
|---|---|
| `GET /external/bindings` | 我的平台绑定 + 可用平台列表 |
| `PUT /external/bindings` | 绑定 `{"platform":"codeforces","handle":"tourist"}`，绑定即首次全量同步 |
| `POST /external/bindings/:platform/sync` | 手动立即同步（增量） |
| `DELETE /external/bindings/:platform` | 解绑（历史记录保留） |
| `GET /external/report/:id` | 刷题统计报表：各平台提交/AC/解决数、365 天合并活动桶、跨平台最近 AC |

首批平台：**codeforces / luogu / atcoder / nowcoder**。洛谷填用户名，
牛客填数字 ID 或主页链接，AtCoder 填用户 ID。服务端每小时自动增量同步，
只读公开数据，不存任何平台密码/cookie。

个人主页热力图（`GET /users/:id/profile`）新增 `platforms` 字段：per-platform
每日活动桶（365 天零填充），前端可多选合并展示本站 + 任意外部平台。

## Python 示例（qqcot 机器人）

```python
import requests

BASE = "http://<host>/api/v1"
KEY = "ojk_xxx"

def user_summary(name_or_id):
    r = requests.get(f"{BASE}/public/users/{name_or_id}", timeout=10)
    r.raise_for_status()
    return r.json()

def problem_cph(pid):
    r = requests.get(f"{BASE}/public/problems/{pid}/cph",
                     headers={"X-API-Key": KEY}, timeout=10)
    r.raise_for_status()
    return r.json()
```

## JS 示例（看板）

```js
const res = await fetch(`/api/v1/public/users/${name}`)
const { ac_problems, activity, recent_ac } = await res.json()
```