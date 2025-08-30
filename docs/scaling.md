# mailez 横向扩容运行手册（HA / 多副本）

本文描述控制面（backend/frontend）多副本、KV 与 blob 横向扩容的部署形态、
工作原理与运维要点。单机社区版不受影响：所有多副本机制在单副本下自动退化
为直通。

## 一、目标拓扑（docker-compose.enterprise.yml + docker-compose.ha.yml）

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
docker compose --env-file mailez.env \
  -f docker-compose.enterprise.yml -f docker-compose.ha.yml up -d --build
# 扩容（gateway 30s 内自动感知新副本）：
docker compose --env-file mailez.env \
  -f docker-compose.enterprise.yml -f docker-compose.ha.yml up -d --scale backend=4
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
| 引擎服务 | mailezine | active-passive（租约） | 服务层单写者；standby 部署在第二台主机、指向同一 TiDB/MinIO 即自动接管 |

## 四、运维要点

1. **NTP**：租约过期比较副本本地时钟，多副本主机必须开 NTP（多机部署常规要求）。
2. **MySQL**：`MAILEZ_DB_DRIVER=mysql` + `MAILEZ_DB_DSN=mailez:…@tcp(mysql:3306)/…`；
   schema 迁移自带 `GET_LOCK` 串行化，多副本同时启动安全。
3. **Redis**：限流/缓存走 DB 的部分（如登录限流）随 DB 共享；Redis 仅加速，
   丢失可重建。
4. **备份**：MySQL + MinIO + TiDB 三者都要备（企业档 `./data/` 整体备份覆盖
   compose 形态）。
5. **升级**：滚动更新安全——认领与租约保证新旧副本共存期间不双发/不漏发；
   迁移由 GET_LOCK 保证只跑一次。
6. **限制**：引擎服务层（IMAP 会话/投递队列）当前是单写者：多引擎副本
   不会提升邮箱读写吞吐（standby 只做容灾）。引擎边缘(SMTP 收信)与
   分片多活见 ROADMAP。
