# Mailez 用户视角功能验证测试计划

- 环境: 前端 http://localhost:3001（Next dev），后端 http://localhost:8080（Go/Fiber），引擎 mailezine（IMAP :143 / SMTP :1587）
- 测试账号: admin@example.com / MailezDemo2026!（种子管理员）；测试中按需创建 `qa-<random>@example.com` 临时用户
- 执行方式: 以真实用户会话走 HTTP API（与前端页面同源调用），辅以 :3001 页面 SSR 抽查
- 结果标记: PASS / FAIL(含证据) / SKIP(环境不支持)

## A. 认证与会话
| # | 用例 | 期望 |
|---|---|---|
| A1 | 正确密码登录 | 200，种下会话 cookie |
| A2 | 错误密码登录 | 401/错误响应，不种会话 |
| A3 | 登录后 GET /sso/me | 200 且 email 正确 |
| A4 | PUT /me/settings 改签名/显示名 | 200 且再读生效 |
| A5 | 改密码→旧密码登录失败→新密码成功→改回 | 各步状态符合 |
| A6 | POST /sso/logout 后 /sso/me | 401 |
| A7 | GET /me/logins | 200 数组含本机登录 |
| A8 | App Token 创建/列表/删除 | 200 链路完整 |

## B. 邮件核心
| # | 用例 | 期望 |
|---|---|---|
| B1 | GET /mail/folders | 200 含 Inbox/Sent/Drafts/Trash/Junk |
| B2 | GET /mail/messages 分页+排序 | 200，total 与条数一致 |
| B3 | GET /mail/message 读信 | 200 含 subject/body |
| B4 | GET /mail/raw | 200 原始 RFC822 |
| B5 | GET /mail/thread 会话聚合 | 200 |
| B6 | POST /mail/send 端到端发信 | 接受→对方收件箱可见 |
| B7 | 回复/转发（send 带 In-Reply-To/引用） | 200 且头正确 |
| B8 | POST /mail/draft 草稿保存→列表→删除 | 200 链路完整；BCC 不丢（已知历史 bug 回归点） |
| B9 | POST /mail/flag 星标/已读切换 | 200 且列表反映 |
| B10 | POST /mail/read-all | 200 且 unseen=0 |
| B11 | POST /mail/move | 200 且源少目标多 |
| B12 | POST /mail/delete → Trash 可见 | 200 |
| B13 | GET /mail/search 关键字 | 200 命中 |
| B14 | POST /mail/snooze → snoozed 列表 | 200 |
| B15 | GET /mail/attachments/zip | 200 zip 魔数 PK |
| B16 | labels CRUD + 打标 | 200 链路完整 |
| B17 | 文件夹 建/重命名/清空/删除 | 200 链路完整 |
| B18 | GET /mail/unseen | 200 与未读数一致 |
| B19 | 邮件模板 存/列/删 | 200 |
| B20 | 定时发送 scheduled + outbox 取消 | 200 |
| B21 | GET /mail/identities | 200 |

## C. 日历
| # | 用例 | 期望 |
|---|---|---|
| C1 | 建事件→列表含→改→删 | 200 链路完整 |
| C2 | shares 建列删 | 200 |
| C3 | /calendar/export.ics | 200 text/calendar |
| C4 | /calendar/feed | 200 |

## D. 联系人
| # | 用例 | 期望 |
|---|---|---|
| D1 | 建→列→改→删 | 200 链路完整 |
| D2 | 导入→导出 往返一致 | 200 |
| D3 | dedupe | 200 |
| D4 | org 联系人 | 200 |
| D5 | CardDAV get/set/sync | 200 |

## E. 云盘
| # | 用例 | 期望 |
|---|---|---|
| E1 | 上传→树列表可见 | 200 |
| E2 | 建目录→重命名→移动文件 | 200 |
| E3 | 下载内容与上传一致 | 字节一致 |
| E4 | share 链接 | 200 |
| E5 | trash→restore→emptyTrash | 200 |
| E6 | 工作台最近文件 = drive 顶层非目录非回收站前5 | 一致 |

## F. 设置与账户安全
| # | 用例 | 期望 |
|---|---|---|
| F1 | PGP 密钥上传→/pgp/key 查询命中 | 200 |
| F2 | S/MIME 证书→/smime/cert 查询 | 200 |
| F3 | 委托 CRUD；被委托人会话可见主人邮箱 | 200 |
| F4 | Sieve 脚本 存/激活/取/删 | 200 |
| F5 | Webhooks CRUD + test | 200 |
| F6 | TOTP 2FA 开启→登录要求验证码→关闭 | 200 |

## G. 管理端
| # | 用例 | 期望 |
|---|---|---|
| G1 | /admin/overview | 200 含统计 |
| G2 | 用户创建/列表/停用/启用 | 200 |
| G3 | 审计日志有记录 | 200 |
| G4 | 公告发布/读取/撤下 | 200 |

## H. 跨域与回归
| # | 用例 | 期望 |
|---|---|---|
| H1 | 全新用户端到端（建户→收信→读信） | 全链路 200 |
| H2 | 权限隔离：A 读不到 B 的邮件/云盘 | 403/404/空 |
| H3 | 回归：admin /mail/messages 502 已修复 | 200 |
| H4 | 页面 SSR 抽查 /、/home、/mail/Inbox | 200 渲染标记 |
