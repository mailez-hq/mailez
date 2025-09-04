# Mailez 全面功能验证测试报告 — EE 版 × PostgreSQL 控制面

- 日期: 2026-09-04
- 被测版本: `dev` 分支 `111713f`（含 7791bfb 前端镜像构建修复 + 111713f 白标扩展）
- 环境:
  - 后端: **EE 构建**（`go run -tags mailez_ee ./cmd/server`），http://127.0.0.1:8080
  - 控制面数据库: **PostgreSQL 16**（`DB_DRIVER=postgres`，`postgres://mailez:***@127.0.0.1:25433/mailez`，容器 `mailez-pgqa-postgres-1`，GORM AutoMigrate 40 张表）
  - 引擎: `ghcr.io/mailez-hq/mailez-mailezine-ee:local`（IMAP :143 / SMTP :1587 / POP3 :110 / ManageSieve :4190），Pebble KV 存储
  - 邮件栈依赖: mailez/rspamd:local、mailez/unbound:local、redis（compose 项目 `mailez-pgqa`，独立子网 192.168.216.0/24，数据全部隔离于 `.tmp/mailez-pgqa/data`，未触碰 deploy/data）
  - 前端: webmail Next dev :3001 / admin Next dev :3002（MAILEZ_EDITION=ee），H4 SSR 抽查用
- 方法: 与上一轮一致——以真实用户会话调用与前端同源的 HTTP API（`qa/qa-batch-*.ps1`），QA_BASE 指向 127.0.0.1:8080
- 结果: **58 PASS / 0 FAIL**（A8 + B21 + C4 + D5 + E6 + F6 + G4 + H4）
- EE 特性抽查: PUT/GET `/branding` 200（UTF-8 中文往返正常，清空后回退默认品牌）；GET `/audit/export` 200（`text/csv`，290 行，EE 审计导出）

## 一、本轮在 PostgreSQL 上发现并已修复的缺陷（3 个，均为发布级）

PostgreSQL 对保留字的解析比 MySQL/SQLite 严格，裸 SQL 中以下列名/表名会解析失败或语义改变：

### PG-1｜`/me/logins` 在 PostgreSQL 上恒返回空数组（A7，高）
- 现象: A1 登录成功后 `GET /me/logins` 200 但 rows=0。
- 根因: `backend/internal/user/me.go` 的 `meRecentLogins` 用裸 `Where("user = ? ...")`。`user` 是 PG 保留字，裸引用解析为 `current_user` 特殊值，与邮箱比较恒为假——**不报错、静默返回空**。
- 修复: 表名限定 `audit_log.user = ?`（MySQL/SQLite/PG 三方言通用）。
- 验证: A7 重跑 rows≥1 PASS；未认证访问 401 不变。

### PG-2｜外部邮箱抓取（fetch）调度器在 PostgreSQL 上每轮报错（高）
- 现象: 后端日志周期性 `fetch: list: ERROR: syntax error at or near "." (SQLSTATE 42601)`。
- 根因: `backend/internal/fetch/fetcher.go` 裸 SQL `JOIN user ON user.email = fetch.user_email`——`user` 保留字导致语法错误，抓取调度在 PG 上完全不可用。
- 修复: 改为 GORM 构建的子查询 `Where("user_email IN (?)", enabled)`（标识符由 GORM 按方言加引号）。
- 验证: 重启后日志无 fetch 报错。

### PG-3｜日历事件列表在 PostgreSQL 上 500（C1，高）
- 现象: 建事件/改事件成功，但 `GET /calendar/events?from&to` 返回 500 "db error"，列表恒空。
- 根因: `backend/internal/calendar/handler.go` 裸 `Where("(end IS NULL OR end >= ?)")`——`end` 是 PG 完全保留字（CASE…END），语法错误。pg 直连复现：`ERROR: syntax error at or near "end"`。
- 修复: 表名限定 `calendar_event.end` / `calendar_event.start`。
- 验证: C1 重跑全链路（建/列/改/删 + reminder）PASS；C2/C3/C4 不受影响。

> 说明: GORM 自动生成的查询会对标识符加引号，不受影响；仅手写裸 SQL 暴露此类问题。三处修复均采用对 MySQL/SQLite/PostgreSQL 同时兼容的写法，CE/EE 共用。

## 二、测试环境伪影记录（非产品缺陷）

1. **`::1:8080` 被占用导致首轮全 401**: Windows 上 `localhost` 优先解析 IPv6，`[::1]:8080` 由 wslrelay（WSL 内某历史进程）应答了 mailez 风格的 401。对策：`QA_BASE` 显式使用 `127.0.0.1`。
2. **D5 首轮 502 同源**: CardDAV sync 的 url 用 `localhost:8080`，后端服务端回调时命中同一 IPv6 占位者。已将 `qa-batch-d.ps1` 的 davUrl 改为 `127.0.0.1`（并保留注释说明原因）。
3. **登录限流**: `/sso/login` 每 IP 15 分钟 30 次（硬编码）。全量 9 批约 30 次登录恰好触顶（首轮 H2 的 429 即此）。对策：分两组执行，组间重启后端（内存限流计数随之清零）。
4. **H4 端口参数化**: `qa-batch-h.ps1` 的 SSR 抽查端口改为 `QA_WEBMAIL_URL` / `QA_ADMIN_URL` 环境变量可覆盖（默认 dev 档 3001/3002 可用），compose 档可传 8083/18082。
5. **脚本编码**: qa-batch-*.ps1 原为 GBK 编码，编辑后统一转为 UTF-8 with BOM（Parser 校验 0 错误），PS 5.1/7 均可运行。
6. **PS 解码伪影（备忘）**: Invoke-RestMethod 对无 charset 的 `application/json` 响应按 Latin-1 解码，中文显示为乱码；qa-lib 的 `Api()` 显式按 UTF-8 解码不受影响。pg 直查 `encode(convert_to(summary,'UTF8'),'hex')` 证明服务端存储为正确 UTF-8。

## 三、上一轮已定位缺陷的回归确认（MySQL → PostgreSQL 均通过）

| 历史 bug | 上轮状态 | 本轮（PG）|
|---|---|---|
| BUG-1 全部标为已读无效（read-all 守卫恒真） | FAIL | **PASS**（B10：read-all 后 Inbox 未读=0）|
| BUG-2 单封 flag 写成小写 `seen` 关键字 | FAIL | **PASS**（B9：flag=204 且 starred=1）|
| BUG-3 日历订阅 ICS 对外 401 | FAIL | **PASS**（C3：带 token 200 text/calendar，无 token 403）|

## 四、通过清单（58/58，按组）

| 组 | 结果 |
|---|---|
| A 认证与会话（8） | 登录/错密 401/sso/me/settings/改密链路/logout/最近登录（PG 修复后）/App Token 全 PASS |
| B 邮件（21） | 读路径 8 项 + 写路径 13 项（端到端收发、回复 In-Reply-To、草稿、flag、read-all、文件夹 CRUD、Trash、snooze、附件 zip、标签、模板、定时发送、BCC 副本）全 PASS |
| C 日历（4） | 事件 CRUD+reminder（PG 修复后）、shares、export.ics、feed 全 PASS |
| D 联系人（5） | CRUD、导入导出往返、dedupe、org、CardDAV 全 PASS |
| E 云盘（6） | 上传/目录操作/下载字节一致/分享链接/trash 恢复/最近文件 全 PASS |
| F 设置安全（6） | PGP 全流程、S/MIME、委托、Sieve、Webhooks、TOTP 2FA 全 PASS |
| G 管理端（4） | overview、用户建列停启、审计日志、公告 全 PASS |
| H 跨域回归（4） | 全新用户端到端、权限隔离（403/404/无泄漏）、admin messages 200（历史 502 回归点）、SSR 抽查 全 PASS |

## 五、环境处置

- live 企业版/多活栈（MySQL）已原样恢复：backend / engine-lb / mailezine×3 已重启，:8083 webmail 200，品牌预览数据未动。
- QA 栈容器已 `stop`（未删除）：`docker compose -p mailez-pgqa` 位于 `D:\code\mailez-hq\.tmp\mailez-pgqa`，含本轮 PostgreSQL 数据；`docker compose -p mailez-pgqa start` 可复活。
- 本轮 results.log 在 `qa/results.log`（gitignored）。

## 附注

- 已知设计缺口（上轮记录）维持现状：/mail/draft 无 bcc 字段、App Token 按 id 升序分页、无委托用户带 X-Delegate-Email 返回 500（应 403）、消息列表 flags 小写无反斜杠。本轮未回归 UI 层，前端历史缺陷（打印视图 XSS、aiCompose 未清附件等）不在本轮范围。
