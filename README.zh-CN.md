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

- **三栏布局** — 文件夹、邮件列表、阅读区并排展示；收发、整理邮件无需刷新
  页面，列表宽度还能拖到你要的尺寸
- **搜索不用学语法** — 直接输关键词，也能用 `from:`、`to:`、`has:attachment`、
  日期等条件精确筛选；常用搜索可保存，一键切换未读/星标/带附件视图
- **键盘优先** — 按 `/` 搜邮件、`⌘K` 打开命令面板、`?` 查看全部快捷键；
  上万封邮件的列表滚动也不卡
- **日常操作不折腾** — 会话线程、批量移动/归档/删除（带撤销提示）、下拉刷新
  看新邮件
- **AI 助手（可选）** — 长邮件一键摘要、按你想要的口吻起草回复、自动给收件箱
  排序，或按意思搜而不是按关键词搜
- **隐私功能内置** — PGP 签名/加密、两步验证、远程图片拦截、Sieve 过滤器
  编辑器、按发件人聚合的通讯录
- **离线也能用** — 可安装为 PWA；亮/暗主题和三档列表密度随你调整

### 管理后台：不像"做管理"的管理后台

- **一个地方管所有** — 域名、邮箱账号、别名、中继、外部邮箱收信、应用令牌
- **一键 DKIM** — 自动生成签名密钥并显示状态，让邮件不再被丢进垃圾箱
- **谁做了什么一目了然** — 管理员操作审计日志，基于角色的权限
  （admin / manager / user）
- **备份迁移很简单** — 整套配置可一键导出、导入

### 安全与可信，藏在细节里

- **防垃圾是有效的** — Rspamd 会从你的举报中学习；DKIM / DMARC / ARC 签名与
  校验保证邮件能送达
- **传输安全** — MTA-STS 和 DANE 保护邮件在途安全；每账号配额和发信限速让
  系统保持健康
- **恶意文件扫描** — 附件中的宏和已知威胁会被识别拦截

## 快速开始

需要 Docker（Compose v2）。

```sh
cd deploy
cp mailez.env.example mailez.env   # 设置 MAILEZ_SECRET_KEY、MAILEZ_DOMAIN、MAILEZ_HOSTNAMES
docker compose up -d --build
```

| 端口 | 是什么 |
|---|---|
| http://localhost:8082 | 管理后台 |
| http://localhost:8083 | Webmail |
| http://localhost:8081 | 后端 API（开发者用） |
| 25/587/143/993/4190 … | 邮件协议（SMTP / IMAP / ManageSieve） |

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
- 邮件投递与过滤：nginx、Postfix、Dovecot、Rspamd、Unbound
- 更多细节：[`docs/dev-setup.md`](docs/dev-setup.md)、
  [`docs/webmail-ui-spec.md`](docs/webmail-ui-spec.md)

## 许可证

[mailez License](LICENSE) —— Apache License 2.0 附加以下使用条件：

- **禁止 SaaS** —— 不得以托管/受管/SaaS 形式向第三方提供本软件
- **仅限自身使用** —— 只能为自己或所在组织运营邮件服务，部署位置不限
  （自建机房、私有云、公有云均可）
- **禁止对外多租户服务** —— 单套部署不得作为邮件服务商服务多个独立组织；
  为自身组织运营多个域名/邮箱不受此限
