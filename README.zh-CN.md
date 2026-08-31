<p align="center">
  <img src="branding/mailez-logo.svg" alt="mailez" width="320">
</p>

# mailez — mail easy

从小用到大：小到个人用户的专属邮箱，大到万人规模的集团企业，对数据主权
要求严格的政企单位同样适用。完全自托管，用自己的域名收发邮件，数据留在
自己手里，再配上一个用得顺手的 Webmail 和后台管理。一条命令部署完整个
邮件系统——收发信、防垃圾、认证、管理后台、网页邮箱全都有。

## 适合谁用？

- **个人用户** —— 想要一个完全属于自己的邮箱，不受第三方邮箱服务的约束
- **企业组织** —— 用自己的域名给全员开邮箱，数据 100% 留在自己手里
- **政企单位** —— 对数据主权和私密性要求严格，同样能够满足

共同点是：自托管、隐私优先，而且都不需要请一位 Linux 邮件专家来运维。

## 你会得到什么

### Webmail：像原生应用一样顺手

- **实时刷新，不用手动** — 新邮件通过实时推送通道自动到达；收信、回信、
  整理全程不刷新页面，上万封的列表照样丝滑滚动
- **会话视图，长帖不刷屏** — 同一主题的往来邮件自动合并成一条会话，点开
  即见完整时间线；回复里的长引用自动折叠成紧凑区块，点击展开，界面始终清爽
- **三栏布局** — 文件夹、邮件列表、阅读区并排展示，列表宽度能拖到你要的尺寸
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
| **community** | mailezine | SQLite + Pebble + 本地 FS（可选 MySQL） |
| **enterprise** | mailezine | MySQL + TiDB + MinIO/S3 |

所有档位运行**同一个 mailezine 引擎**——协议一致、邮件层功能一致、升级
路径一致。版本差别在三点：**存储规模**（单机 Pebble/本地 FS 对分布式
TiDB/MinIO）、**集群形态**（单机 / active-passive 容灾 / multi 全服务多活）
与**授权功能**（合规归档、DLP、AI、LDAP 同步、ActiveSync、S/MIME、委派
代管属企业版）。传统 Postfix+Dovecot 架构的存量部署可用 `mailezine
migrate` 原地迁移到新存储。

```sh
./deploy/mailezctl.sh up              # dev 档
./deploy/mailezctl.sh up community    # 社区版（生产）
./deploy/mailezctl.sh up enterprise   # 企业版（生产）
./deploy/mailezctl.sh up ha           # 企业版 + 控制面多副本
./deploy/mailezctl.sh up multi        # 企业版 + 引擎多活（分布式邮件系统）
```

`ha` 档把控制面横向扩容：backend 副本在网关后负载均衡（无会话粘性），
定时发送在任意副本数下原子认领不双发，后台 worker 走 DB 租约选主
（60s 自动故障转移），超大附件与云盘落共享对象存储。

`multi` 档让 mailez 成为**真正意义的分布式邮件系统**：每个引擎副本在共享
TiDB/MinIO 上服务任意账户——SMTP、IMAP/POP3、Sieve、出站队列全部多活，
无主备、扩容即加副本。出站队列按消息事务认领（投递中途被 kill -9 的
节点，租约到期后由其他副本接管，迟到写回被 fencing 丢弃）；单例 worker
跨节点租约；全文索引每节点 tail 变更日志收敛；按账户写 pin 吸收跨节点
热键竞争。全部语义经真实双进程 + TiDB 的 kill -9 故障演练验证
（mailezine 仓库 `TestMultiActiveFailover`）。选型表与运维手册见
[`docs/scaling.md`](docs/scaling.md)。

首次使用前编辑 `deploy/mailez.env`（由 `mailez.env.example` 复制而来），
至少设置两个密钥——compose 的 `${VAR:?}` 插值要求它们非空，缺失时
mailezctl 会直接报错退出：

```sh
MAILEZ_SECRET_KEY=$(openssl rand -hex 16)    # TokenEnc / 外部账号密码加密
MAILEZ_STACK_SECRET=$(openssl rand -hex 32)  # 引擎↔backend 内部 API 鉴权
MAILEZINE_STACK_SECRET=                      # 必须与上一行同值
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
# 容器化档位：镜像内置 seed 器（默认 admin@example.com / MailezDemo2026!，
# 可用 MAILEZ_ADMIN_EMAIL / MAILEZ_ADMIN_PASSWORD 覆盖）
cd deploy && docker compose --env-file mailez.env -f docker-compose.community.yml exec backend mailez-seed
# 本地 dev 档（SQLite 在宿主机）：
cd backend && go run ./cmd/seed

# 端到端：发一封测试邮件，检查投递、DKIM 签名与防垃圾过滤
go run ./cmd/e2e
```

## 技术栈（开发者）

- 后端：Go + Fiber，GORM，Redis
- 前端：Next.js（React）——管理后台与 Webmail 两个独立应用
- 邮件引擎统一为 **mailezine**（单 Go 二进制），走引擎无关的目录契约
  （`/stack/directory/*`）提供 SMTP/IMAP/POP3/ManageSieve，KV + blob 存储
  均可插拔；dev/社区/企业三档同一引擎，仅在存储规模（SQLite/pebble 对
  MySQL/TiDB/MinIO）与授权功能上有别
- 更多细节：[`docs/dev-setup.md`](docs/dev-setup.md)、
  [`docs/architecture.md`](docs/architecture.md)、
  [`docs/webmail-ui-spec.md`](docs/webmail-ui-spec.md)；
  版本/档位间升级（含 MySQL→SQLite 控制面切换与社区版→企业版路径）：
  [`docs/upgrades.md`](docs/upgrades.md)

## 许可证

[AGPL-3.0](LICENSE) —— GNU Affero 通用公共许可证 v3.0。

- **自托管无负担** —— 自己或本组织部署、修改、使用均无额外义务；
  仅当分发软件或以网络服务形式提供修改版时，需以同协议公开修改
- **Copyleft 设计** —— 任何人分发 mailez 或将其修改版上线提供服务，
  都必须以同一协议公开源码——项目与分叉保持开放
- **商业授权** —— 闭源商用、SaaS/托管服务、OEM 嵌入需要商业授权；
  企业版（合规归档 / DLP / AI / LDAP / ActiveSync / S/MIME / 委派代管 /
  分布式存储与 HA）需 `deploy/licenses/license.lic` 授权文件，自签与
  采购流程见英文 README 的 Enterprise licensing 一节
  （企业版自带）；联系 `contact@mailez.com`
