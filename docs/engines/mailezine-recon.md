# mailezine 侦察报告（阶段 1）

> 状态：**侦察完成**（2026-08-24）· 路线修订 2：**借鉴同类引擎 + 参照 reference server 协作
> 模式，从零自研**——不 fork、不嵌入任何上游运行时；JMAP 优先、可插拔存储
> （maildir / RocksDB+MinIO）、日历/联系人/文件内置（不再外挂 Radicale）。
> v1 范围 = 最小闭环（SMTP 收/发 + JMAP Mail + 自研存储 + 配额 + srs-relay）；
> 日历/联系人（CalDAV/CardDAV 兼容层）紧随其后；Sieve 执行引擎与 IMAP 全量 → v2。
> 依据：`D:\code\peer engine` 实盘代码 + reference server 0.16 协作架构（reference docs site 文档）+ 生态查证。

## 1. 定位

mailezine = mailez 的第一方引擎（引擎 C），**参照 reference server 的 all-in-one 协作模式**，
最终成为默认引擎：

- **同类引擎**：活教材——邮件实现细节（SMTP/IMAP/队列/认证/反垃圾）分层参考，
  **不引入为依赖**。
- **reference server**：架构参照——单二进制、**JMAP 优先**（邮件+日历+联系人+文件统一
  协议族）、可插拔存储（文档存储 + S3 对象存储）、内置 CalDAV/CardDAV 兼容层、
  Management API 供给。
  借鉴架构与数据模型，**不抄代码**（AGPL-3.0，与 mailez 的 Apache-2.0 不兼容）。
- **legacy MTA/IMAP stack**：多租户虚拟域/别名/中继模型（mailez 已有）、maildir 可迁移性、
  Sieve 生态——作为兼容目标而非实现来源。

终局：数据面 + 控制面全 Go；rspamd / unbound / macro-scanner 等共享基础设施保留。

## 2. 侦察结论（TL;DR）

1. **不 fork、不嵌入任何上游运行时**：peer engine 核心包耦合 sconf 配置模型与 0.x 未冻结
   存储；reference server 是 AGPL 且 Rust 代码不可直接复用。两者都只当架构参考。
2. **推荐路线：借鉴 + 从零骨架，参照 reference server 协作模式**——三个关键决策：
   - **JMAP 优先**：JMAP Core/Mail（RFC 8620/8621）是核心协议，IMAP 降级为兼容层
     （老客户端用），与"webmail 走 REST 封装 IMAP"的现状解耦。
   - **可插拔存储双后端**：邮件/日历/联系人/文件统一为"账户 → collection →
     document + blob 引用"模型；后端一是 **maildir**（就地挂载 legacy IMAP 存量，
     兼容/迁移零成本），后端二是 **RocksDB + MinIO**（元数据/索引在 LSM KV，
     消息体与附件进 S3 兼容对象存储，云部署/规模路线）。
   - **日历/联系人内置**：JSCalendar/JSContact 数据模型 + CalDAV/CardDAV 兼容层
     （基于 go-webdav，MIT），不再外挂 Radicale；会议邀请走 iTIP 邮件管道。
3. **密码兼容是现成的**：bcrypt 同格式（`$2a$`），供给时直接写入。
4. **供给走 Management API 模式**：控制面通过 mailezine 自己的管理 API 下发
   账户/配额/日历/通讯录变更（对照 reference server 的 JMAP Management API 设计），
   而不是直接写库。
5. **配额原生语义保留**：按账户配额、投递时强制，目录契约语义不变。
6. **缺口与排期**：
   - Sieve：Go 生态只有解析器（emersion/go-sieve，MIT）与 WIP 解释器；
     **执行引擎 v2 自研**，ManageSieve（4190）+ JMAP Sieve（RFC 9661）同批。
   - SRS：复用 mailez 已有 `srsCodec`，数据面带 srs-relay（与 reference server 共用）。
   - POP3：RFC 1939 简单，v2 兼容层。
   - Milter：v1 反垃圾衔接（内置简化 JunkFilter 或前置 rspamd）M0 决策。

## 3. 侦察事实（代码核实）

> 以下事实是"借鉴"的基础（架构与实现细节参考），不构成复用承诺。

### 3.1 peer engine 模块与许可

| 项 | 值 |
|---|---|
| 模块 | `github.com/mjl-/peer engine`，Go 1.25 |
| 许可 | MIT / MPL-2.0 双许可（仓库含 LICENSE.MIT、LICENSE.MPLv2.0） |
| 关键依赖 | `mjl-/bstore`（MIT）、`mjl-/adns`、`mjl-/autocert`、`mjl-/sconf`、`mjl-/sherpa` 均 MIT 系 |
| 可嵌入性 | store/smtpserver/imapserver/queue 均依赖 `config` + `moxvar`；`moxvar` 只是版本常量，`config` 是 sconf 配置模型 |

### 3.2 reference server 协作架构（reference docs site 0.12–0.16 文档核实）

- **协议双轨**：CalDAV（RFC 4791）+ CardDAV（RFC 6352）+ WebDAV 文件 + WebDAV ACL
  （0.12 起）；CalDAV Scheduling / iTIP（RFC 6638，0.12.1）；**JMAP Calendars /
  Contacts / File Storage / Sharing 全族**（2025-10 宣布完整实现，首个全 JMAP
  协作族服务器）。
- **数据模型**：按"账户 → collection → document"存储；日历事件用 JSCalendar
  JSON，联系人是 JSContact JSON，对外自动转换 iCalendar/vCard 喂 CalDAV/CardDAV。
- **可选与轻量**：协作服务只在 HTTP listener 启用时加载，可按用户/组/租户关闭。
- **许可**：AGPL-3.0 —— 只借鉴架构与数据模型，不复制代码。
- **存储分层**：文档/索引存数据存储（FoundationDB/RocksDB/SQLite/PG/MySQL 可插拔），
  大对象（消息体、附件）走 S3 兼容对象存储——mailezine 的 RocksDB+MinIO
  双后端即参照此分层。

### 3.3 peer engine 存储/队列/认证（邮件面参考）

- 存储：`data/accounts/<account>/` + bstore 索引 + 消息文件；搜索 WordSearch。
- 队列：出站队列 + 重试 + Routes（可配置中继/relay）。
- 认证：SCRAM-SHA-256/1、CRAM-MD5、PLAIN（bcrypt 校验）。
- 导入导出：maildir/mbox 双向。

## 4. 与 mailez 目录契约的映射（借鉴要点）

| mailez 概念 | 设计要点（不照搬任何上游） |
|---|---|
| user（email/enabled/quota） | 账户对象含配额字段，投递时强制；供给走管理 API |
| alias | 账户的附加投递地址（多对一） |
| domain（本地/替代域名） | 域名对象 + 替代名解析 |
| relay（出站中继） | 出站路由表（transport 指向中继） |
| 发件人限制（sender login/代发身份） | M0 定义映射规则 |
| sieve（筛选） | v2 自研执行引擎 + ManageSieve + JMAP Sieve |
| quota used 回写 | 存储用量统计 → `/stack/directory/quota/:email` |
| 密码 | bcrypt 同格式，直接供给 |
| 日历/通讯录 | 统一 collection/document 存储 + CalDAV/CardDAV 兼容层 |
| 2FA/TOTP | 引擎层只有密码；2FA 留在控制面 SSO/凭据层 |

## 5. 推荐路线：借鉴同类引擎 + 参照 reference server，从零骨架

### 5.1 路线

- mailezine = 独立 Go 模块，主程序 + 数据面 + 协作面全部自研；peer engine 与 reference server
  只读参考，不引入依赖。
- 供给：控制面内部 API 下发变更 → mailezine 管理 API 应用（Management API 模式），
  配置模型归自己。
- 存储：可插拔后端。抽象层 = "账户 → collection → document + blob 引用"；
  **maildir 后端**就地读写 legacy IMAP 兼容目录（消息即文件），**RocksDB+MinIO
  后端**把元数据/索引放 LSM KV、消息体与附件放 S3 对象存储。两者对上层协议
  （JMAP/IMAP/SMTP 投递）暴露同一存储接口。
- 协议：JMAP 核心（mail）→ CalDAV/CardDAV 兼容层 → IMAP 兼容层（v2）。

### 5.2 自研清单（必须自己写）

| 面 | 内容 | 理由 |
|---|---|---|
| 编排/生命周期 | 配置加载、监听器、优雅退出、健康检查 | mailezine 主程序，无现成可搬 |
| 供给/管理 API | 账户/配额/日历/通讯录变更下发（Management API 模式） | 控制面归自己 |
| 存储层 | 存储抽象接口 + 双后端（maildir / RocksDB+MinIO）+ 索引 + quota 强制 | 不复用 peer engine store |
| SMTP 收/发 | 会话状态机、投递、出站队列 + 重试 + 中继 | 核心差异化与控制权 |
| JMAP Core/Mail 服务端 | RFC 8620/8621：认证、邮件读写、搜索基础 | **核心协议**（替代 IMAP-first） |
| CalDAV/CardDAV 兼容层 | go-webdav server 之上做 iCalendar/vCard 转换 | 苹果/mainstream clients/手机可用 |
| 日历/联系人对象 | JSCalendar/JSContact 数据模型 + 基本 CRUD | 协作面内置 |
| Sieve 执行引擎 | 解释器 + ManageSieve + JMAP Sieve | **v2** |

### 5.3 借鉴架构清单（照着重构，不抄代码）

- peer engine：SMTP 会话/队列/认证分层；反垃圾 JunkFilter 设计（贝叶斯 + 声誉）。
- reference server：账户 → collection → document 存储模型；JMAP-first 协议分层；
  CalDAV/CardDAV 作为兼容视图而非核心；Management API 供给；共享/ACL 模型；
  文档存储与对象存储分离（元数据 KV + blob S3）。

### 5.4 复用原语清单（MIT，建议引库）

- `dkim` / `dmarc` / `spf` 签名与校验（peer engine 对应包或等价生态库）。
- `message`（MIME 解析/序列化：peer engine 的 `message` 包或 emersion/go-message）。
- **go-webdav**（emersion/go-webdav：caldav/carddav server，MIT）→ 兼容层底座。
- **rrule-go**（recurrence 展开）→ 日历重复规则。
- go-sieve（emersion/go-sieve）解析器 → v2 执行引擎建立在它之上。
- RocksDB 绑定（cgo，如 grocksdb/gorocksdb）或纯 Go 的 LSM 替代 **Pebble**；
  MinIO 官方 SDK `minio-go`（S3 兼容）。
- 加密/随机原语（golang.org/x/crypto，bcrypt 已有）。

### 5.5 明确不做

- 不 fork peer engine / reference server、不 vendor 任何上游存储、不引入 sconf。
- 不引入 Radicale（日历/联系人由 mailezine 内置；postdove 引擎才需要它过渡）。
- v1 不做：Sieve/ManageSieve、IMAP 全量、POP3、JMAP Calendars/Contacts 全量、
  全文搜索。maildir 后端 v1 即是一等后端（非仅导入格式）。

## 6. 里程碑拆解（v1 ≈ 40-70 人天，协作面另计）

> 参照 reference server 的开发曲线（4 年、多人 + NLnet 资助），mailezine 是**年级项目**；
> 下表 v1 以"跑通邮件最小闭环 + 协作骨架"为准，正确性与互操作打磨另计。

| 里程碑 | 内容 | 量级 | 出口标准 |
|---|---|---|---|
| M0 | 侦察收尾：JMAP 服务端 Go 生态、存储抽象（maildir / RocksDB+MinIO 接口）、RocksDB 绑定选型（cgo vs Pebble）、sender 映射定稿 | 1-2 天 | 四项决策有结论 |
| M1 | 骨架：mailezine 模块 + 编排 + 端口矩阵 + 健康检查 + 管理 API 骨架 | 4-6 天 | 端口冒烟 + 供给对照测试 |
| M2 | 存储抽象 + 双后端：账户/collection/document + blob 引用；maildir 后端 + RocksDB+MinIO 后端 + 索引 + quota | 12-16 天 | 双后端存储单测 + 崩溃恢复冒烟 |
| M3 | SMTP 收/发：收信（SPF/DKIM 校验 + 投递）+ 发信（队列 + 重试 + 中继 + DKIM 签名） | 8-12 天 | e2e 收发/中继子集通过 |
| M4 | JMAP Core + JMAP Mail 最小闭环：认证、邮箱/邮件读写、基础搜索 | 8-12 天 | webmail 经 JMAP 读写可用 |
| M5 | 日历/联系人对象 + CalDAV/CardDAV 兼容层（go-webdav）+ iTIP 基础 | 8-12 天 | 苹果/mainstream clients 可连、建事件/联系人 |
| M6 | srs-relay + 后端间迁移工具（maildir ↔ RocksDB+MinIO）+ e2e + 文档 | 3-5 天 | 迁移演练 + 全量 e2e |

**v2**：Sieve 执行引擎 + ManageSieve（4190）+ JMAP Sieve（RFC 9661）、IMAP 兼容层
（CONDSTORE/QRESYNC）、POP3、JMAP Calendars/Contacts/Sharing 全量、全文搜索、
推送（JMAP Push）。

## 7. 风险与决策点

| 风险 | 影响 | 缓解/决策 |
|---|---|---|
| 全栈自研工作量 | 年级项目，进度风险最高 | 骨架先行、JMAP 最小闭环先立；postdove 保持默认直到 mailezine e2e 通过 |
| JMAP 服务端 Go 生态年轻 | 协议正确性风险（RFC 8620/8621 细节多） | 先做最小闭环 + 官方 JMAP 测试套件/互操作矩阵 |
| CalDAV/CardDAV 互操作 | 苹果/Google/mainstream clients 行为差异 | go-webdav 底座 + 真实客户端矩阵；范围先砍"建事件/联系人" |
| 存储双后端自研 | 索引/崩溃恢复/并发正确性翻倍 | M0 定稿存储抽象接口与后端选型；maildir 后端先行、RocksDB+MinIO 后端跟进 |
| RocksDB cgo 依赖 / Pebble 取舍 | 全 Go 目标 vs 成熟度 | 全 Go 优先则选 Pebble（纯 Go LSM，CockroachDB 生产验证）；接受 cgo 才用 RocksDB 绑定 |
| MinIO 部署依赖 | 云/自托管都要多一个组件 | 对象存储抽象为 S3 接口，MinIO 与云 S3 可互换；小规模可先用本地盘后端 |
| maildir 并发语义 | 多进程/多实例写入冲突 | 单进程引擎内使用；集群/高可用走 RocksDB+MinIO 后端 |
| Sieve 执行引擎自研 | v2 最大工作量与正确性风险 | 先支持 mailez 筛子模板扩展子集（fileinto/redirect/vacation/reject），逐项补 |
| 反垃圾衔接 | rspamd 无 Milter | M0 决策：内置简化 JunkFilter 或前置代理 |
| 发件人/代发身份映射 | 与 mailez 权限模型偏差 | M0 定义 sender 映射规则 |
| 上游许可 | reference server AGPL / SnappyMail AGPL | 只借鉴架构与数据模型，不复制代码 |

> 上游运行时风险已消除：不 fork、不嵌入 peer engine / reference server / Radicale，仅参考。

## 8. 与其他引擎的关系

- **接缝复用**：mailezine 作为适配器 #3，复用目录契约（`/stack/directory/*`）、
  认证契约与 e2e 引擎无关化（由 reference server 集成定义）。
- **日历能力分化**：postdove 引擎用 Radicale 过渡；reference server 引擎原生自带；
  mailezine 自研内置——webmail 日历 UI 与控制面契约统一，协议按引擎适配。
- **组件共用**：srs-relay 与 reference server 引擎 B 共用；供给 agent 为 mailezine 独有。
- **演进路径**：postdove（默认/稳定）→ reference server（引擎 B/市场验证）→ mailezine
  （第一方/最终默认）。mailezine 上线后，postdove 与 reference server 转为"老客户兼容模式"。

## 9. 相关文档

- 同类评估：`docs/engines/peer-evaluation.md`、`docs/engines/reference-evaluation.md`
- 引擎抽象：`backend/internal/stack/directory.go`、`backend/internal/core/config.go`
