<p align="center">
  <img src="branding/mailez-logo.svg" alt="mailez" width="320">
</p>

# mailez — mail easy

面向团队/企业的自托管邮件平台：用自己的域名收发邮件，数据留在自己手里，
再配上一个用得顺手的 Webmail 和后台管理。一条命令部署完整个邮件系统——
收发信、防垃圾、认证、管理后台、网页邮箱全都有。

## 适合谁用？

- 想用自己的域名收发邮件、不想把数据交给第三方邮箱服务的团队和企业
- 想要自托管、隐私优先，但不想请一个 Linux 邮件专家来运维的人

## 你会得到什么

### Webmail：像原生应用一样顺手

- **实时刷新，不用手动** — 新邮件通过实时推送通道自动到达；收信、回信、
  整理全程不刷新页面，上万封的列表照样丝滑滚动
- **三栏布局** — 文件夹、邮件列表、阅读区并排展示，列表宽度能拖到你要的
  尺寸；阅读区像 Gmail 一样折叠引用内容，长帖一目了然
- **搜索不用学语法** — 直接输关键词，也能用 `from:`、`to:`、`has:attachment`、
  日期等条件精确筛选；常用搜索可保存，一键切换未读/星标/带附件视图
- **键盘优先** — 按 `/` 搜邮件、`⌘K` 打开命令面板、`?` 查看全部快捷键
- **日常操作不折腾** — 会话线程、操作栏内直接快捷回复和 AI 摘要、延后处理、
  定时发送、批量移动/归档/删除带撤销提示；草稿完整保留全部收件人（含密送）
- **AI 助手（可选）** — 长邮件一键摘要、按你想要的口吻起草回复、自动给收件箱
  排序，或按意思搜而不是按关键词搜
- **不只是收件箱，还是工作台** — 首页仪表盘汇总最近文件和即将到来的日程，
  点击直达上下文
- **隐私功能内置** — PGP 签名/加密（与个性签名相互独立，支持自动签名）、
  两步验证、远程图片拦截、Sieve 过滤器编辑器、按发件人聚合的通讯录
- **离线也能用** — 可安装为 PWA；亮/暗主题和三档列表密度随你调整

### 邮件之外，一样能打

- **日历** — 带提醒的日程、共享日历，还能用私有 ICS 链接订阅到
  Apple 日历或 Google 日历
- **通讯录** — vCard 导入/导出、重复联系人合并、手机 CardDAV 同步
- **云盘** — 上传整理文件、链接分享、回收站恢复；最近文件直接出现在工作台

### 任何设备、任何客户端

- **标准协议** — SMTP / IMAP / POP3（支持隐式 TLS），外加 CardDAV / CalDAV
  与 Exchange ActiveSync 手机同步；Thunderbird、Outlook、Apple Mail 走
  autoconfig/autodiscover 自动配置
- **邮箱委托** — 把邮箱全权委托给同事（或管理共享邮箱），无需共享密码
- **应用令牌** — 按客户端签发令牌，随时在设置里吊销

### 管理后台：不像"做管理"的管理后台

- **一个地方管所有** — 域名、邮箱账号、别名、中继、外部邮箱收信、应用令牌；
  删除用户会自动级联清理引擎侧邮箱数据
- **一键 DKIM** — 自动生成签名密钥并显示状态，让邮件不再被丢进垃圾箱
- **谁做了什么一目了然** — 管理员操作审计日志、基于角色的权限
  （admin / manager / user），外加全站公告横幅
- **备份迁移很简单** — 整套配置可一键导出、导入

### 安全与可信，藏在细节里

- **防垃圾是有效的** — Rspamd 会从你的举报中学习；DKIM / DMARC / ARC 签名与
  校验保证邮件能送达
- **传输安全** — MTA-STS 和 DANE 保护邮件在途安全；每账号配额和发信限速让
  系统保持健康
- **恶意文件扫描与内容管控** — 附件中的宏和已知威胁会被识别拦截；企业版
  还提供出站 DLP 与合规归档

## 快速开始

三个自包含的部署档位以 compose 文件形式提供，由统一入口管理：

| 档位 | 引擎 | 存储 |
|---|---|---|
| **dev**（默认） | mailezine | SQLite + Pebble + 本地 FS |
| **community** | Postfix + Dovecot | MySQL + maildir |
| **enterprise** | mailezine | MySQL + TiDB + MinIO/S3 |

```sh
./deploy/mailezctl.sh up              # dev 档
./deploy/mailezctl.sh up community    # 社区版（生产）
./deploy/mailezctl.sh up enterprise   # 企业版（生产）
```

dev 档需要宿主机 `:8080` 上先起后端（镜像只需构建一次：
`cd backend && go run ./cmd/build-images`，细节见
[`docs/dev-setup.md`](docs/dev-setup.md)）；两个生产档完全容器化，发布端口：

| 端口 | 是什么 |
|---|---|
| http://localhost:8082 | 管理后台 |
| http://localhost:8083 | Webmail |
| http://localhost:8081 | 后端 API（开发者用） |
| 25/465/587/143/993/4190 … | 邮件协议（SMTP / IMAP / ManageSieve） |

默认关闭 TLS，适合本地试用；生产环境按
[`deploy/certs/README.md`](deploy/certs/README.md) 配置自动证书即可。

启动后可跑一遍端到端验证，确认整个邮件链路正常：

```sh
cd backend
go run ./cmd/seed   # 初始化管理员账号（只需一次）
go run ./cmd/e2e    # 发一封测试邮件，检查投递、DKIM 签名与防垃圾过滤
```

## 技术栈（开发者）

- 后端：Go + Fiber，GORM，Redis
- 前端：Next.js（React）——管理后台与 Webmail 两个独立应用
- 邮件引擎可插拔，统一走引擎无关的目录契约（`/stack/directory/*`）：
  **mailezine**（单 Go 二进制，dev/企业档默认）提供 SMTP/IMAP/POP3/
  ManageSieve，KV + blob 存储均可插拔；**postdove**（Postfix + Dovecot，
  前置 nginx 网关）支撑社区版；Stalwart 适配器也可接入同一契约
- 更多细节：[`docs/dev-setup.md`](docs/dev-setup.md)、
  [`docs/architecture.md`](docs/architecture.md)、
  [`docs/webmail-ui-spec.md`](docs/webmail-ui-spec.md)

## 许可证

[mailez License](LICENSE) —— Apache License 2.0 附加以下使用条件：

- **禁止 SaaS** —— 不得以托管/受管/SaaS 形式向第三方提供本软件
- **仅限自身使用** —— 只能为自己或所在组织运营邮件服务，部署位置不限
  （自建机房、私有云、公有云均可）
- **禁止对外多租户服务** —— 单套部署不得作为邮件服务商服务多个独立组织；
  为自身组织运营多个域名/邮箱不受此限
