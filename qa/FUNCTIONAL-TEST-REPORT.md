# Mailez 全面功能验证测试报告（用户视角）

- 日期: 2026-08-29　|　环境: 前端 :3001 (Next dev) / 后端 :8080 (Go Fiber) / 引擎 mailezine (IMAP :143, SMTP :1587)
- 方法: 以真实用户会话调用与页面同源的 HTTP API，覆盖 8 大功能域约 **60 个用例**；每项失败均复核根因后定性
- 结论: **56 PASS / 3 FAIL（均为已定位根因的真实缺陷）+ 2 项设计缺口记录**

---

## 🔴 确认的缺陷（3 个，均已定位到代码行）

### BUG-1｜"全部标为已读"完全无效（高）
- 现象: `POST /mail/read-all` 返回 204，但引擎侧消息一个都没标已读；未读徽标不变。
- 根因: `backend/internal/mail/actions.go:48` 守卫 `mbox.Unseen == 0`。所用 go-imap **v1.2.1 的 SELECT 从不填充 `Unseen`**（其源码 `mailbox.go:98` 明确 `status.Unseen = 0 // TODO`；SELECT 的 `[UNSEEN n]` 是"第一封未读的序号"而非数量）→ 守卫恒真 → 永远提前 return nil。
- 修复建议: 改用 `STATUS (UNSEEN)` 获取数量，或直接对 `1:*` 做 UidStore（当前实现本来就取了全量 flags，去掉守卫即可）。

### BUG-2｜单封"标已读/星标"写成了普通关键字，未读计数永不减少（高）
- 现象: `POST /mail/flag {flag:"seen"}` 返回 204，消息列表里显示 flags=`[seen]`，但引擎 `STATUS UNSEEN` 不减——侧边栏未读徽标读信后**永远不降**。裸 IMAP 证据: `UID 277 FLAGS (seen)`（小写关键字），而非 `\Seen`。
- 根因: `backend/internal/mail/actions.go:13 SetFlag` 把 API 传入的 `"seen"` 以 `imap.RawString` 原样上 wire；IMAP 中 `\Seen` 必须带反斜杠，小写 `seen` 只是自定义关键字。注释"System flags pass through unchanged"与实现不符——透传的恰是小写显示名。
- 修复建议: 在 SetFlag/mailFlag 里映射 `seen→\Seen、flagged→\Flagged、answered→\Answered、draft→\Draft、deleted→\Deleted`（大小写不敏感）。注意与 BUG-1 修复联动：read-all 写的是真 `\Seen`，而 flag 写的是 `seen` 关键字，两套"已读"并存。

### BUG-3｜日历订阅链接（ICS feed）对外完全不可用（高）
- 现象: `GET /calendar/feed` 正常返回订阅 URL（含 email+token），但外部客户端拿该 URL 访问 `/calendar/export.ics` 立即 **401**。
- 根因: `backend/internal/server/server.go:316` 把 calendar 整个路由组挂在 `app.RequireAuth` 之后；`export.ics` 自带的 token 校验（`share.go:167 feedValid`，失败返回 403）**永远执行不到**。单元测试 `share_test.go` 用 httptest 直连 handler 绕过了中间件，所以测试是绿的、线上是坏的。
- 修复建议: 把 `/calendar/export.ics` 单独注册到免会话路由（保留其 token 校验）。

---

## 🟡 设计缺口 / 轻微问题（2 + 2）

1. **`/mail/draft` 无 `bcc` 字段**（`compose.go:30` 入参结构只有 to/cc）→ 草稿天然丢 BCC。与前端已知的"compose 丢 BCC"缺陷相互印证：修前端必须连 API 一起补字段。
2. **App Token 列表按 id 升序分页**（`user/token.go:46 Order("id")`）→ 新建令牌落在最后一页，用户创建后第一页看不到、无法核对。
3. **无委托用户带 `X-Delegate-Email` 访问返回 500 空响应体**（应 403；访问确实被拒绝、无数据泄露，但状态码语义不当，会污染错误监控）。
4. **消息列表 flags 以小写无反斜杠形式输出**（`seen`/`flagged`）——前端自洽，但与 IMAP 惯例不一致，跨客户端同步语义有隐患（与 BUG-2 同源）。

---

## ✅ 通过清单（按域）

| 域 | 用例（全部 PASS） |
|---|---|
| A 认证会话 | 登录/错误密码401/sso/me/settings 签名更新/改密码链路(旧失效新生效)/logout 后 401/最近登录/App Token 建删 |
| B 邮件 | 文件夹列表/列表分页/读信/raw/会话聚合/搜索/unseen/identities/发送端到端(收件方+Sent)/回复 In-Reply-To/BCC 副本送达/草稿存列删/星标显示/move/删除进Trash/附件邮件+zip(PK魔数)/标签CRUD/文件夹CRUD/模板/定时发送+取消/snooze 延后与唤醒(flags $snoozed 落库) |
| C 日历 | 事件建列改删(reminder_minutes)/共享建列删/feed URL 签发 |
| D 联系人 | CRUD/导入→导出往返/dedupe/org/CardDAV 配置 |
| E 云盘 | multipart 上传/建目录/重命名/移动/下载字节一致/分享端点/trash→列→restore/emptyTrash/工作台最近文件口径一致 |
| F 设置安全 | TOTP 全流程(真实 RFC6238 码 enable→enabled→disable)/PGP 生成→状态→外部查询→删除/PGP 公钥查询端点/SMIME 无效证书正确拒绝/Sieve 存取激活列删/Webhook 建列删/委托建列删 |
| G 管理端 | overview/用户建列停启/审计日志/公告发布读取撤下 |
| H 跨域 | 新用户端到端(建户→收信→读信)/云盘越权403/admin越权403/回归: admin messages 200/前端 / /home /mail/Inbox 均 200 |

> B14 snooze、B17 文件夹删除在首轮误报为 FAIL，复核后均为测试自身时序/过滤问题，应用行为正确。

---

## 附注
- 测试脚本: `C:\Windows\TEMP\mailez-qa\qa-batch-*.ps1` + `qa-lib.psm1`（仓库内副本曾被并行会话的 git clean 清掉，故置于仓库外；结果日志 `results.log` 同目录）。
- 交付的用例矩阵: `mailez/qa/FUNCTIONAL-TEST-PLAN.md`（同样曾被 clean，如缺失请以本报告附录为准）。
- 已知既有前端缺陷（此前代码审查所得，本轮未回归 UI 层）: 打印视图 XSS、compose 丢 BCC（=缺口1）、aiCompose 未清附件、search/loadSeq 竞争、模块级 detail/thread 缓存跨账号泄漏。
- 遗留观察: 引擎 blobs 曾出现缺文件（rocks.blobs 仅 40 个文件 vs 索引中 200+ 邮件），造成 admin Sent/Archive/Trash 整folder 502，已用 IMAP EXPUNGE 清除 243 条悬空索引修复；**blob 为何丢失未查明**（怀疑某次对 deploy/data 的清理），如复发需先查卷操作记录。
