# mailez 横向扩容运行手册（HA / 多副本 / 引擎多活）

本文描述控制面（backend/frontend）多副本、KV 与 blob 横向扩容、以及引擎
（mailezine）多活的部署形态、工作原理与运维要点。单机社区版不受影响：所有
多副本机制在单副本下自动退化为直通。

## 一、目标拓扑（docker-compose.ee.yml + docker-compose.ha.yml）

```
            gateway(nginx)                       ← resolver 动态解析，副本即加即用
           /      |       \
     backend×N  admin×N  webmail×N             ← 控制面/前端：无状态副本
         |   \___________/
   MySQL(共享库)  MinIO(共享对象存储)             ← 超大附件 + 云盘 + 正文 blob
   TiDB(引擎 KV)  mailezine(active-passive)     ← 存储层横向组件 + 引擎单写者
```

启动：

```bash
cd deploy
./mailezctl.sh up ha
# 扩容（gateway 30s 内自动感知新副本）：
docker compose --env-file mailez.env \
  -f docker-compose.ee.yml -f docker-compose.ha.yml up -d --scale backend=4
```

## 二、backend 多副本的三个机制

### 1. 定时发送原子认领（outbox）

`internal/compose/outbox.go`：每条到期的投递先以单条 UPDATE 原子认领
（`pending → sending`，`RowsAffected==1` 才投递），因此任意数量副本并发
flush 不会双发。副本在投递中途崩溃会留下 `sending` 状态，5 分钟后被任意
副本回收重投（at-least-once；SMTP 本就是 at-least-once 语义，见引擎 D47）。

### 2. singleton worker 的 DB 租约选主（`internal/cluster`）

周期性 worker 在每个 tick 先调 `cluster.TryHold(db, name, 60s)`：
抢到租约的副本执行，其余副本跳过。租约行带 TTL，leader 崩溃后其余副本在
≤60s 内自动接管，无人工介入。已接入的 worker：

| worker | 租约名 | 说明 |
|---|---|---|
| 外部邮箱聚合拉取 | `fetch` | 双副本会重复下载上游邮件 |
| web push 通知 | `push_notifier` | 双副本会双推送到设备 |
| 日历提醒 | `calendar_reminder` | check-then-insert 竞态 |
| 超大附件清理 | `uploads_cleanup` | 幂等但避免并发删 |
| DLP 审批过期(EE) | `dlp_expiry` | — |
| 归档 retention(EE) | `archive_retention` | — |
| LDAP 目录同步(EE) | `ldap_sync` | — |

发送 undo 队列（outbox）不走租约而走认领：它需要多副本分摊吞吐。
webmail 实时流（SSE EventWatcher）是副本本地的：每个副本只轮询连在自己
身上的用户，天然多副本安全。

### 3. 对象存储共享（超大附件 + 云盘）

`MAILEZ_DRIVE_BACKEND=minio`（复用 `MAILEZINE_S3_*` 连接配置）时：
- 超大附件（uploads）与云盘（drive）都落 MinIO/S3；
- 上传落 A 副本、下载走 B 副本不再 404；
- 本地盘后端（默认）行为与历史版本完全兼容（键 = 上传目录下的相对路径）。
  从本地切到 MinIO 时，历史文件需要一次性 `rsync uploads/` 到桶根目录
  （键即对象名）；之后写入的对象无需再迁移。

## 三、各层横向能力一览

| 层 | 组件 | 横向方式 | 说明 |
|---|---|---|---|
| 网关 | gateway(nginx) | 单实例或 keepalived/上层 LB | 模板用变量+resolver(30s) 反代 backend，副本自动纳管 |
| 控制面 | backend | **副本数随意** | DB_DRIVER=mysql 必须设置；SSE 靠 X-Accel-Buffering: no 直通，无需粘性 |
| 前端 | admin / webmail | **副本数随意** | 无服务端状态 |
| 队列/异步 | 控制面 worker | 内置选主/认领 | 见上表 |
| 引擎 KV | TiDB | TiDB 集群原生横向 | compose 档为单节点 unistore，生产换 TiUP 集群，DSN 不变 |
| 引擎 blob | MinIO/S3 | MinIO 分布式模式 / 云 S3 | 内容寻址 + 幂等写，天然多节点安全 |
| 引擎服务 | mailezine | active-passive（租约）**或多活（multi 档）** | 单写者容灾见 §六；`MAILEZINE_CLUSTER_MODE=multi` 下全服务多副本多活 |

## 四、运维要点

1. **NTP**：租约过期比较副本本地时钟，多副本主机必须开 NTP（多机部署常规要求）。
2. **MySQL**：`MAILEZ_DB_DRIVER=mysql` + `MAILEZ_DB_DSN=mailez:…@tcp(mysql:3306)/…`；
   schema 迁移自带 `GET_LOCK` 串行化，多副本同时启动安全。
   PostgreSQL 同样可用：`MAILEZ_DB_DRIVER=postgres` +
   `MAILEZ_DB_DSN=postgres://…`；注意迁移互斥锁目前仅 MySQL 实现
   （PG 为 no-op），多副本场景先让单个副本完成启动迁移后再扩容。
   企业版 compose 的 backend 接受 `MAILEZ_DB_DRIVER` / `MAILEZ_DB_DSN`
   覆盖（指向外部 MySQL/PostgreSQL；捆绑的 mysql 容器仍会启动，可忽略）。
3. **Redis**：限流/缓存走 DB 的部分（如登录限流）随 DB 共享；Redis 仅加速，
   丢失可重建。
4. **备份**：MySQL + MinIO + TiDB 三者都要备（企业档 `./data/` 整体备份覆盖
   compose 形态）。
5. **升级**：滚动更新安全——认领与租约保证新旧副本共存期间不双发/不漏发；
   迁移由 GET_LOCK 保证只跑一次。
6. **限制**：active-passive 档（§一拓扑）的引擎服务层（IMAP 会话/投递队列）
   是单写者：多引擎副本不会提升邮箱读写吞吐（standby 只做容灾）。要吞吐
   横向扩展用 §六 的多活档。

## 五、HA 档与多活档怎么选

| | ha 档（docker-compose.ha.yml） | multi 档（docker-compose.multi.yml） |
|---|---|---|
| 引擎形态 | 1 主 + N standby（租约选主，60s 故障转移） | N 副本全活，无主备 |
| 吞吐 | 单写者，standby 不提升吞吐 | SMTP/IMAP/投递随副本线性分摊 |
| 存储要求 | TiDB + MinIO（同左） | TiDB + MinIO（同左） |
| 配置 | `MAILEZINE_HA_ENABLED=true` | `MAILEZINE_CLUSTER_MODE=multi`（与 HA 互斥，启动时校验拒绝同时开启） |
| FTS | 正常（bleve，单写者） | 追踪收敛（见下），最终一致 |
| 适用 | 改动最小、只要容灾 | 真正的分布式多活，扩容即加副本 |

## 六、引擎多活（multi 档）

拓扑（docker-compose.ee.yml + docker-compose.multi.yml）：

```
 MX/L4-LB ──> engine-lb(haproxy) ──PROXY v1──> mailezine×N  ← 邮件端口按连接轮询
                    ↑
 backend×N ─ IMAP/SMTP/Sieve/内部 MTA 也经 LB；管理 API(8090) 直连
 MySQL   MinIO(共享对象存储：正文 blob)
 TiDB(引擎 KV：队列元数据 / 邮箱索引 / 租约)
```

启动：

```bash
cd deploy
./mailezctl.sh up multi
# 扩容（默认 2 副本）：
MAILEZ_ENGINE_REPLICAS=3 ./mailezctl.sh up multi
```

### 多活的引擎机制

1. **出站队列按消息认领**（`mailezine/internal/queue`）：调度器扫描到期
   索引后，每条消息先在 KV 事务里原子认领（`QUEUED/DEFERRED → ACTIVE`，
   记录 owner + 租约，到期索引同时挪到租期时刻）。认领失败（他副本持有
   有效租约）就跳过——任意时刻一条消息全集群只有一个活跃 worker。持有者
   崩溃后，租约到期的消息被任意副本接管重投（接管计一次尝试，crash-loop
   不会无限重投）；接管后原 worker 的迟到写回会被 ownership fencing 丢弃，
   不会覆盖新状态。消息 ID 分配也在事务里自增，多副本不重号。
2. **单例 worker 跨节点租约**（`mailezine/internal/kvlease`）：snooze
   sweeper 等全局唯一 worker 用 KV 租约（owner + TTL，tick 即续约）选主，
   全集群同一时刻只有一个副本执行；失联副本的租约过期后自动接管
   （at-least-once）。
3. **FTS 收敛**（`mailezine/internal/ftssync`）：bleve 索引是节点本地的，
   多活下每个节点跑一个 changelog 追踪器——按账户水位增量读取变更日志
   （INV-CHANGE：按账户追加、无缝、有序），create/update 重索引、delete
   （扩展编码携带被删副本的 mailbox+UID）精确删除。投递路径不再内联索引，
   每副本恰好索引一次；邮件落进集群后至多一个同步周期（默认 2s）内可在
   任意节点搜到；副本重建/换机后水位从 0 重放自动全量重建。
4. **邮箱写路径事务冲突重放**（既有能力）：UID/modseq/changelog 的
   read-modify-write 走 TiDB 乐观事务冲突检测 + 重放，多副本并发写同一
   账户正确（冲突多时重放增多，热账户可在后续按账户 pin 写优化）。

### 运维要点（multi 档）

1. **NODE_ID**：多机部署时每台主机配唯一 `MAILEZINE_CLUSTER_NODE_ID`；
   compose 副本共享 env，必须留空（引擎按容器 hostname 自动生成）——
   两个副本配同值会破坏认领 fencing。
2. **投递语义 at-least-once**：租约过期瞬间的接管可能造成个别重复投递，
   与单机崩溃恢复语义一致（默认租期 10 分钟，`MAILEZINE_QUEUE_CLAIM_LEASE_SECONDS`
   可调，须大于最慢单次投递）。
3. **FTS 最终一致**：多活档搜索经每节点的 changelog 追踪索引（机制见上），
   新邮件至多一个同步周期（2s）后可搜；副本重建后自动全量重放。
4. **客户端 IP**：engine-lb 对全部后端连接注入 PROXY protocol v1 头，
   引擎只接受来自 engine-lb 固定 IP 的 PROXY 连接（`MAILEZINE_PROXY_TRUSTED`
   锁定，默认 `192.168.206.250/32`）——入站邮件与 IMAP/SMTP 会话记录的都是
   真实客户端地址，DNSBL、限流、审计按真实来源评分。替换为云 L4 LB /
   keepalived 时保持同款语义（云 LB 开 PROXY protocol，
   `MAILEZINE_PROXY_TRUSTED` 收窄到 LB 网段）。
5. **节点观测**：`GET /health` 返回 `"role":"active"`、`"cluster":"multi"`，
   队列消息带 `owner` 字段可直接定位持有副本；`owner` 还出现在“认领丢失”
   告警日志里。
6. **多活健康指标**（Prometheus，每节点）：
   - `mailezine_queue_claims_total{outcome=claimed|stolen|lost}`——认领/
     接管/丢失计数。**stolen/lost 持续增长 = 租约过短或有节点反复死在
     投递中途**（调大 `MAILEZINE_QUEUE_CLAIM_LEASE_SECONDS` 或排查节点）；
   - `mailezine_kv_txn_replays_total`——KV 乐观事务因写冲突的重放计数
     （多写者争用同一账户热键的信号，见下条决策）。
7. **跨机部署**：每台主机跑一个 mailezine 指向同一 TiDB/MinIO/控制面，
   MX 指向 L4 LB 即可；无任何节点间专线要求（协调全部经共享存储事务）。
8. **客户端明文登录（实测）**：multi 档引擎带证书后 IMAP 143 拒绝明文
   LOGIN（`NO [PRIVACYREQUIRED] TLS is required`）——客户端须改用 993/
   995 隐式 TLS 或 STARTTLS。隐式 TLS 端口（465/993/995）依赖
   `deploy/certs/cert.pem + key.pem`，缺文件时引擎不监听这三个端口，
   engine-lb 对应后端会 DOWN。engine-lb 日志中 `node12…No IP for
   server` 一类 WARNING 是 server-template 预留槽位（副本数未占满 16）
   的预期噪音，扩容后自动纳管。

### 按账户写 pin（account gate）

多活写正确性由事务冲突重放保证，pin 写是**尾延迟优化**：多活模式下
（`MAILEZINE_ACCOUNT_GATE_ENABLED`，默认开）入站投递的每账户存储段在
“账户所有权租约”下执行——先到节点持有账户租约（TTL 30s、后台续约、
空闲 60s 释放），其他节点对同一账户的写入**排队等待**而不是在 UID/
modseq/quota/changelog 热键上冲突重放。语义：

- **咨询式**：等待有上限（默认 5s，`MAILEZINE_ACCOUNT_GATE_WAIT_SECONDS`），
  超时或存储故障则照常写入（正确性回落到事务重放）——邮件永不被门闸
  阻塞或丢弃；
- **跟随流量**：租约懒获取、热账户保持、静默账户零成本；持有者失效后
  租约到期即被接管；
- **观测**：`mailezine_account_gate_events_total{wait|fallback}`（wait=
  出现跨节点竞争被吸收；fallback=等待超时照常写入）与
  `mailezine_account_gate_held`（本节点当前持有的账户数）。fallback
  持续增长说明 Wait 过短或某节点长期占住账户。

覆盖范围为入站投递路径（跨节点冲突的主源）；IMAP 会话本身天然单节点，
其写冲突继续走事务重放（正确且罕见）。

### 跨机部署清单（每台引擎主机）

```bash
# 与集群共享的三个地址（全部主机同值）：
MAILEZINE_STORAGE_DSN=root@tcp(tidb.example:4000)/mailez
MAILEZINE_S3_ENDPOINT=minio.example:9000
MAILEZINE_BACKEND_ADDRESS=backend.example:8080

# 本机唯一（hostname 即可）：
MAILEZINE_CLUSTER_NODE_ID=mail-01

# 前置 LB 保留真实客户端 IP（LB 须注入 PROXY v1 头，与 engine-lb 同款语义）：
MAILEZINE_PROXY_PROTOCOL=25,465,587,143,993,110,995,4190
MAILEZINE_PROXY_TRUSTED=<LB 网段>
MAILEZINE_CLUSTER_MODE=multi
```

要点：每台主机本地磁盘只存 FTS 索引与水位（可随时重建）；节点上下线
无需协调——新节点上线后 tail changelog 追平索引，队列认领自动参与；
`NTP` 必开（租约比较各节点本地时钟）。

### 验证：多活故障演练

`mailezine` 仓库的 `TestMultiActiveFailover`（`cmd/mailezine`，需
`MAILEZINE_TEST_TIDB_DSN` 指向真实 MySQL 协议服务）以**两个真实引擎
进程 + 共享 TiDB + 门控 MX** 复现生产多活语义并断言：

1. 两节点均 `role=active`（无主备）；
2. 经节点 A 投递的邮件从节点 B 的 IMAP 读出（跨节点邮箱状态）；
3. 节点 A 认领出站消息后、投递中途被 kill -9，节点 B 在租约到期后
   接管并完成投递——sink 恰好收到一次，队列状态经节点 B 到达
   `delivered`。

```bash
go test -tags mailez_ee ./cmd/mailezine/ -run TestMultiActiveFailover -v
```
