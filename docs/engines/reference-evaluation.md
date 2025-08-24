# reference server 引擎评估与 PoC 计划

> 状态：**评估完成**（2026-08-24）· 结论：reference server 作为第二引擎（引擎 B）可行，
> 但认证路由是最大决策，需先跑一轮 3-5 天 PoC 验证后再正式集成。
> 默认引擎保持不变：postdove（引擎 A）。

## 1. 背景

mailez 的数据平面默认是经典容器栈（gateway/nginx + legacy MTA + legacy IMAP + rspamd +
unbound），控制平面（用户/域/API/筛选/AI）是 Go 后端。用户希望未来对外销售时提供
两个引擎版本：**引擎 A = legacy MTA + legacy IMAP**（成熟、社区久经考验），**引擎 B =
reference server**（单二进制、Rust、内置 JMAP/反垃圾/配额）。两个引擎共用同一套 mailez
控制平面与目录契约（`/stack/directory/*`）。

## 2. 现状基线（已就位，无需重做）

| 已有能力 | 位置 | 说明 |
|---|---|---|
| 引擎开关 | `backend/internal/core/config.go` | `MAILEZ_MAIL_ENGINE`（当前默认 `postdove`，`SupportedMailEngines` 仅含 postdove） |
| 引擎无关目录契约 | `backend/internal/stack/directory.go` | `/stack/directory/*`：用户/域/别名/中继/发件人/SRS/配额/Sieve |
| SRS codec | backend `srs` 包 | `directorySRSForward` / `directorySRSRestore` 已实现，目前由 legacy MTA canonical 表消费 |
| SRS 应用点 | legacy MTA canonical maps | `sender_canonical_maps`/`recipient_canonical_maps` 经 socketmap → mailez-agent → backend 计算，在信封阶段同步应用；backend 只算不改，不经过消息内容 |
| 密码格式 | 用户表 `$2a$` bcrypt | 与 reference server 支持的 bcrypt 兼容（需 PoC 实测确认） |
| 引擎目录布局 | `deploy/engines/{postdove,reference}` | `backend/cmd/build-images` 按目录自动发现组件，新增组件只需登记镜像名 |
| 构建管线 | `backend/cmd/build-images` | Go 实现，跨平台，无 PowerShell 依赖 |
| Sieve 客户端 | backend sieve 包 | 已修复多行 greeting / STARTTLS / quoted-string AUTHENTICATE；内部端口 11490 |

## 3. 关键约束（reference server 0.16 事实，PoC 必须围绕这些设计）

1. **外部 SQL/LDAP 目录不再携带账户配额**。0.16 起，配额必须通过
   JMAP Management API 写入 reference server 内部目录的 Account 对象（`maxDiskQuota`、
   `maxEmails`、`maxMailboxes`、`maxAppPasswords` 等），外部 SQL 目录只能做认证与
   收件人/别名解析。
2. **all-in-one 模式不支持外部 SMTP/IMAP 认证委托**。"external SMTP or IMAP
   authentication cannot be used when running the all-in-one or JMAP servers"。
   远程认证仅限 standalone 拆分解。
3. **Proxy Protocol 只解决真实客户端 IP**，`proxyTrustedNetworks` 是代理协议
   信任，不是"受信代理跳过认证"。没有干净的 trusted-proxy-skip-auth 标志，
   因此"nginx 认证通过后直接放行到 reference server"这条路走不通，必须重新设计认证路由。
4. **ManageSieve 4190 可用**。reference server 自带 ManageSieve 服务（默认未启用，
   配置 `server.listener.sieve` 即可），另有 JMAP for Sieve（RFC 9661）。mailez
   webmail 的筛选编辑器（走 4190 协议）不需要重写。
5. **JMAP Management API 可完成 provisioning**。账户创建/更新、配额、应用密码
   （AppPassword）、API key 都有 JMAP 对象与 `reference-cli` 封装，适合做控制面适配器。
6. **reference server 0.16 无原生 SRS**。官方支持论坛明确 "reference server doesn't do SRS"；
   MTA hooks / milter 只能挂在**入站**阶段，无法在出站时改写信封；`session.mail.rewrite`
   是静态正则改写，做不了带签名、可逆还原的 SRS0。引擎 B 的 SRS 必须由数据面
   自带的组件承担（见第 10 节），不能依赖 reference server 内置能力。

## 4. PoC 目标（3-5 人天，验证三个核心假设）

1. **目录适配**：mailez 的目录契约能映射到 reference server（SQL 只读解析 + Management
   API provisioning），用户/域/别名/中继全链路可用。
2. **认证路由**：在 all-in-one 限制下，选出并验证一个可落地的认证方案
   （方案 A 认证前置 vs 方案 B 直接认证，见第 8 节）。
3. **功能等价**：webmail/admin 的邮件功能（读写/搜索/筛选/配额/转发）在 reference server
   引擎下与 postdove 行为一致，或差距可枚举、可接受。

## 5. PoC 环境

- 独立 compose 项目：`deploy/engines/reference/` + `deploy/docker-compose.reference.yml`，
  与 dev 栈（postdove）互不冲突；测试期间 postdove 保持默认运行，随时可回退。
- 独立数据库 schema（或独立 DB），不复用生产用户表，避免污染。
- 测试域 `poc.example.com`：10 个账户、1 个别名、1 个中继、1 个转发账户。
- backend 以 `MAILEZ_MAIL_ENGINE=reference server` 启动（PoC 阶段需先把 `reference server` 加入
  `SupportedMailEngines`，适配器先打桩）。
- 客户端工具：项目内 `go run ./cmd/e2e`（SMTP/IMAP 全链路）+ 少量 Go 小工具，
  不使用 Python。

## 6. Phase 0：准备（0.5 天）

- [ ] 锁定并拉取 `reference/mail-server:0.16.x`（1.0 发布后复查再锁 1.0）。
- [ ] `deploy/engines/reference/` 下落 Dockerfile（基于官方镜像做配置注入与
      健康检查），`build-images` 的 `imageName` 增加 `reference server` 映射。
- [ ] 写 `docker-compose.reference.yml`：端口规划与 postdove 隔离（内网端口如
      10143/10025/14190，由 gateway 代理对外端口）。
- [ ] backend：`SupportedMailEngines` 增加 `reference server`，`stack` 包加一个临时的
      reference server 适配器（先返回契约 JSON，供链路自测）。
- [ ] 确认 backend 现有目录契约测试（`contract_test.go`）在适配器抽象下仍然成立。

## 7. Phase 1：目录认证 PoC（1 天）

- [ ] reference server 配置 SQL 目录，指向 PoC 数据库（用户表 + 别名表 + 域名表）。
- [ ] IMAP 认证：用 Go 小工具（或 `openssl s_client`）验证
      `AUTHENTICATE PLAIN` / `LOGIN`，含 bcrypt `$2a$` 密码与 `recipient_delimiter`
      场景。
- [ ] SMTP 认证：`AUTH LOGIN/PLAIN` 成功，出站能投递到本地账户与外部测试地址。
- [ ] 收件人解析：`test@example.com` 投递落库；别名解析到目标账户；裸域投递
      （post-office）行为与 postdove 一致。
- [ ] 目录契约对照：对同一批数据，`/stack/directory/users/:email`、
      `/stack/directory/aliases/:addr`、`/stack/directory/domains/:domain` 在
      postdove 与 reference server 引擎下返回一致。
- [ ] 记录：SQL 目录模式下的功能边界（配额缺失、密码策略、别名类型）。

> 结论预期：SQL 目录认证与收件人解析可行；但配额必须走 Management API，所以
> Phase 1 只是"收件人解析"验证，配额能力在 Phase 2 补。

## 8. Phase 2：Management API provisioning（1-1.5 天）

- [ ] 启用 JMAP 服务与 Management API（配置管理 token/API key）。
- [ ] 写 Go 临时工具 `backend/cmd/provision-reference`：调用 JMAP Management API
      完成账户创建、`StorageQuota`（`maxDiskQuota`）设置、AppPassword 创建/撤销。
- [ ] 验证配额强制：写入超过 `maxDiskQuota` 时投递/存储被拒（返回 QUOTA 或
      550），`/stack/directory/quota/:email` 回写用量与 reference server 实际一致。
- [ ] 验证密码生命周期：创建 → 登录 → 修改 → 撤销 → 登录失败。
- [ ] **决策点**：provisioning 走全量 Management API（推荐，配额+密码闭环），
      SQL 目录降级为"只读收件人解析"或废弃，所有变更由 backend 统一下发。

## 9. Phase 3：认证路由 PoC（1-2 天，最大决策）

### 方案 A：nginx 认证前置（保留现状思路）

现状：gateway（nginx）对 IMAP/SMTP 做代理，backend 通过认证钩子校验后放行。
reference server 限制：all-in-one 无 remote auth，proxy protocol 只解决真实 IP，
没有"受信代理跳过认证"。PoC 要验证：

- [ ] 能否用 proxy protocol + 内部网络 + reference server 自身认证做"二次校验"实现
      前置认证？预期：要么放行后 reference server 仍要认证（双认证），要么彻底绕过
      reference server 认证（不可行/不安全）。结论预期：**方案 A 不可行或复杂度远超收益**。

### 方案 B：reference server 直接认证（推荐候选）

认证闭环全部交给 reference server：

- [ ] backend 登录时通过 Management API 验证用户密码，成功后为会话签发短期
      应用密码（或复用用户 AppPassword），webmail 的 IMAP/SMTP 连接用它认证。
- [ ] 验证：登录 → 取 app password → IMAP/SMTP 全链路正常；密码轮换与撤销
      即时生效；限制每个会话的 app password 数量与过期时间。
- [ ] 验证与现有 SSO 的衔接：3001 登录后 3000 免登录的会话逻辑不受影响。

### 产出

- [ ] 写 ADR（架构决策记录）：A/B 取舍结论、理由、对 nginx 配置与 backend
      auth 模块的影响清单。

## 10. Phase 4：SRS 边界策略（0.5-1 天，引擎无关）

SRS 只在"转发/重定向"路径上必要（普通用户直发不需要）。两个引擎各自负责自己
的边界改写，backend 只做 secret 与 codec 的单一来源：

- **postdove（引擎 A）不动**：legacy MTA canonical maps + socketmap 已在信封阶段
  同步完成 SRS，零额外跳数、零改造。
- **reference server（引擎 B）**：数据面新增一个小型 Go SRS 中继（srs-relay），只承接
  出站转发/重定向消息：接受 SMTP → 用 backend 下发的 secret 复用 `srsCodec`
  改写 MAIL FROM → 投递出去；入站 bounce 恢复同理（`/stack/directory/srs/restore/*`
  已在）。实现可参照 `D:\code\peer engine` 的 smtpserver/smtpclient 包。
- **明确不做**：把 SRS 改写搬进 backend API 服务本身。backend 是控制面，一旦
  承担消息热路径（接收/改写/投递 SMTP 负载），就要引入队列、重试、超时、
  背压等数据面职责，并让转发可用性取决于 API 服务、放大攻击面——这是净负担，
  且不是 reference server 集成的必要条件。

- [ ] 验证（引擎 B PoC 内做）：srs-relay 改写后外部 SPF 检查通过；bounce 回信
      经 `directorySRSRestore` 恢复原始收件人。
- [ ] 决策点：srs-relay 作为独立小镜像（`deploy/engines/reference/` 下），还是
      复用 mailez-agent 的 socketmap 模式，正式集成时定。

## 11. Phase 5：网关 PoC（0.5 天）

- [ ] nginx 保留（TLS 终止 + 对外端口 25/465/587/110/995/143/993/4190），
      reference server 只监听内网端口；验证代理后 TLS 证书、STARTTLS、客户端兼容性。
- [ ] 验证 webmail 后端连接串：`MAIL_IMAP_ADDR` / `MAIL_SMTP_ADDR` /
      `MAIL_SIEVE_ADDR` 指向 reference server 内网端口后的全链路。

## 12. Phase 6：回归与迁移（1-2 天）

- [ ] 用 reference server 官方 migration proxy（或 imapsync）从 postdove（maildir）
      迁移测试账户邮件，记录耗时与格式差异。
- [ ] 跑现有 e2e（`go run ./cmd/e2e ...`）在 reference server 引擎下，枚举失败项：
      IMAP 扩展（搜索/线程/ACL/稍后处理）、Sieve（4190 与执行）、POP3、推送。
- [ ] 前端冒烟：webmail 收件箱/阅读/搜索/写信/筛选编辑 + admin 9 个页面。
- [ ] 记录资源占用（CPU/内存/磁盘）与 postdove 的对照。
- [ ] 整理功能差距清单与补救计划（如走 JMAP for Sieve 替代 4190）。

## 13. PoC 出口验收清单

1. 10 账户 + 别名 + 中继 + 转发在 reference server 上正常收发。
2. webmail 的邮件功能（列表/阅读/搜索/标记/文件夹/写信/附件/筛选）可用。
3. 配额强制生效（超限拒绝投递，用量回写 backend 一致）。
4. 密码/应用密码生命周期闭环（创建、验证、修改、撤销）。
5. SRS 转发 + bounce 恢复验证通过。
6. e2e 在 reference server 引擎下通过率与差距清单明确。
7. 认证路由 ADR 已写，方案 A/B 有数据支撑的取舍结论。
8. 迁移工具链可用（maildir → reference server），回退方案明确（保留 postdove 副本）。

## 14. 决策门与时间盒

- PoC 总时间盒：**3-5 人天**；若 Phase 3 认证路由在 2 天内无法给出可行结论，
  暂停 reference server 集成。SRS（Phase 4）不设为前置任务：postdove 现状无需改造，
  引擎 B 的 srs-relay 随正式集成一起做。
- 正式集成：约 20-30 人天（容器部署 2-3、provisioning 适配器 4-6、认证路由
  5-8、SRS 上移 3-5、网关策略 2-3、迁移/测试/文档 5-8）。
- 版本策略：锁 0.16 具体小版本；reference server 1.0 发布后复查配额/认证 API 是否变化。

## 15. 风险与缓解

| 风险 | 影响 | 缓解 |
|---|---|---|
| 认证路由（方案 A）在 all-in-one 下不可行 | 需改 webmail/客户端认证链路 | PoC Phase 3 前置，尽早出 ADR；方案 B 兜底 |
| SQL 目录无配额 | 产品配额功能失效 | 全量走 Management API provisioning |
| 存储格式迁移锁定 | 邮件迁入即绑定 reference server | PoC 保留 postdove 副本；迁移工具链验证后才切换 |
| Sieve 兼容性（4190 细节） | webmail 筛选编辑器回归 | reference server 自带 ManageSieve；若差异大，备选 JMAP for Sieve |
| reference server 无原生 SRS | 引擎 B 转发邮件 SPF/DMARC 失败 | 数据面自带 Go srs-relay（复用 backend srsCodec），不把改写搬进控制面 |
| 0.16 未稳定（1.0 未发布） | API/配置漂移 | 锁版本号；对外只把 reference server 当"第二引擎选项"，不当默认 |

## 16. 相关工作

- 引擎抽象与目录契约：`backend/internal/stack/directory.go`、
  `backend/internal/core/config.go`
- 引擎目录布局：`deploy/engines/`
- 构建管线：`backend/cmd/build-images/main.go`
- 同类评估：`docs/engines/peer-evaluation.md`

## 17. 引擎抽象收益：一次性成本 vs 每次成本

接入 reference server 的 20-30 人天里，大部分是**平台通用化税**——把抽象从"纸面契约"
变成"真实可用的接缝"。这笔税只付一次；之后每接入一个新引擎，成本约 5-8 人天。

### 一次性投入（reference server 这次付掉，所有引擎复用）

| 工作块 | 为什么是一次性 |
|---|---|
| 认证路由决策 + 引擎无关认证契约（约 5-8 人天） | 今天 `/auth/*` 是 nginx auth_request / socketmap 形态（postdove 形状）。reference server 逼我们定出"控制面签发凭据、引擎负责校验"的通用模型；定了以后 peer engine / 新引擎直接复用 |
| SRS relay 组件 | 引擎无关的独立 Go 中继，任何没有原生 SRS 的引擎都能挂；有原生 SRS 的引擎只需配 secret |
| 网关策略（nginx 保持） | 一次定案，不随引擎变化 |
| e2e 引擎无关化 + 前端冒烟 | 同一套用例跑所有引擎，前端零改动 |
| build-images 构建管线 | 已完成（目录自动发现） |

### 每次都要做（但都小）

| 工作块 | 量级 | 性质 |
|---|---|---|
| 引擎容器 Dockerfile + compose 覆盖 | 1-2 人天 | 机械性工作 |
| 目录/provisioning 适配器 | 3-5 人天 | 唯一真正的引擎专属代码：把契约翻译成该引擎的 provisioning API（reference server = JMAP Management API，peer engine = 文件配置 + 自有 admin API） |
| 引擎特有迁移工具 + 回归验证 + 文档 | 1-2 人天 | 每个引擎各做一次 |

### 前提（否则每次都是 20-30 人天）

1. **认证契约必须抽象**：落地 app-password / 会话凭据模型，reference server 适配器里
   不得出现只在 postdove 成立的假设（如 Auth-Server 固定 IP、socketmap 形状）。
2. **provisioning 抽象成接口**：`CreateUser / SetPassword / SetQuota /
   ProvisionAlias / ProvisionRelay`，每个引擎一个实现，后端业务不感知引擎。
3. **e2e 只走协议端口**（IMAP/SMTP/4190），不碰引擎内部存储与内部 API。
4. **目录契约只增不改**（additive），避免为适配新引擎而改契约导致全量回归。

当前接缝状态：目录契约（`/stack/directory/*`）、引擎开关、构建管线已就位；
缺口在**通用认证契约**与**provisioning 接口**——这正是 reference server 集成时要补的
两件事，补完即完成平台通用化。
