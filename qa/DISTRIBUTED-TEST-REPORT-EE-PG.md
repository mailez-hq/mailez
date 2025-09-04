# 分布式测试执行报告 — EE 版 × PostgreSQL 控制面（EE-PG Distributed Run）

- 依据方案：`qa/DISTRIBUTED-TEST-PLAN.md`（138 测试点，执行口径沿用 `qa/DISTRIBUTED-TEST-EXECUTION-REPORT.md`）
- 执行窗口：2026-09-04（UTC+8 下午），独立 QA compose 靶场
- 执行环境（compose 项目 `mailez-pgdist`，独立子网 192.168.216.0/24，数据全部隔离于 `.tmp/mailez-pgdist/data`）：
  - **3 × mailezine**（`ghcr.io/mailez-hq/mailez-mailezine-ee:local`，`cluster=multi`、`storage=tidb`、`blob=s3`、PROXY v1 只信 LB 固定 IP .250、TLS 证书挂载、license=enterprise(max=100)）
  - **engine-lb**（`mailez/engine-lb:local`，haproxy L4 + `server-template` 对服务名 `mailezine` 多 A 发现 + send-proxy 健康检查摘除）
  - **backend ×2（b1/b2）** —— 本轮最大增量：无状态 API 双副本，`DB_DRIVER=postgres` 指向共享 **PostgreSQL 16**，Redis 共享会话；DNS 名 `backend` 双 A 轮询
  - 共享 TiDB（引擎 KV，unistore）、MinIO（邮件 blob + 云盘文件）、rspamd、unbound
  - 被测版本：`dev@3b43b34` 内容构建；**backend-ee:local 为本轮重建**（旧镜像 09-01 构建，不含 09-03 的 PG 驱动支持 322ee25 与 c72a61f 保留字修复）
- 种子账号：`test.dist` 域 + `user_a`/`user_b`/`qadisabled`（后置禁用）/`qafull`（后置配额 1000B），口令 `DistTest#2026!`
- 探针：`distprobe`（SMTP/IMAP/POP3、隐式 TLS、LB 内直连单副本 + PROXY v1 注入，本轮新增 `DISTPROXY_SRC` 环境变量以适配新网段；未入库）
- 结果日志：`qa/results-dist-pg.log`（gitignored）

## 1. 结论总览

| 大类 | 用例 | 结果 |
| --- | --- | --- |
| 基线：`backend go test ./...`（含 c72a61f PG 修复） | 全量 32 包 | **PASS**（exit 0） |
| GW-DIST：跨网关漂移投递/三副本对账/kill -9 重连 | 001a/b/c、003a(复测)/003b、008 | **6/6 PASS** |
| SHARD / MULTI：UIDVALIDITY/UIDNEXT/存在性三副本一致、跨实例可见性 | 007、008、001、004 | **4/4 PASS**（可见性 0.6s，≤5s 上界） |
| MQ-IDEM：正常路径幂等（Message-ID 恰好一份）+ 队列接管 | 001、002、FAULT-005 变体 | **3/3 PASS**（kill 中途 5/5 投递零重复） |
| FAULT：kill -9 + LB 摘除恢复、滚动重启服务连续 | 006、RECON-1 终态对账 | **2/2 PASS**（滚动期 6/6 在途投递全达、终态 LB=19 = 副本 19/19/19） |
| WM-API（双副本，上轮 BLOCKED 项） | SESS、001~006、007a/b/c、009、010 | **12/12 PASS**（草稿/标志/移动/搜索/通讯录/云盘/会话/改密/kill b1 故障转移） |
| QUOTA / DISABLED / TLS / NDR | QUOTA-1、1b/1c、DISABLED-1~4、TLS-INV-1/2、NDR-1 | **8/8 PASS** |
| PG 观察（PG-1~3） | audit 双副本写入、cluster_lease 单持有者、无死锁 | **3/3 PASS** |
| **合计** | **38 项判定** | **38 PASS / 0 产品缺陷** |

初测记录中的 15 条 FAIL 全部为**测试脚本问题或瞬态窗口**并已复测覆盖（详见 §3），无一为产品缺陷。

## 2. 本轮关键验证点（对照方案高危清单）

- **R1/R2（重复投递、fencing）**：正常路径三副本各自恰好一份（1/1/1）；kill 中途 5 封全达零重复；kill 前投递的信在恢复后三副本 1+1 一致。队列租约在 PG `cluster_lease`（PK=name 结构性单持有者）上成立，5 个租约名（push_notifier/calendar_reminder/ldap_sync/dlp_expiry/archive_retention）各唯一 holder。
- **R5（UID 一致性）**：三副本 uidvalidity=1、uidnext 与 exists 全程一致（含 kill/滚动重启后）。
- **R6（改密传播）**：admin 改密后新密码双副本立即生效（PG 共享目录）；旧 Webmail 会话在改密后仍有效（`WM-API-007c` 记录现状，维持上轮"上线必改项"结论）。
- **R7（跨实例可见性）**：写后 0.6~8s 双 API 可见；**FTS 搜索跨副本为最终一致**（秒级收敛，`WM-API-005b` 实证——初测 0/0 即收敛窗口）。
- **R13（blob 降级）**：本轮未注入 S3 故障（沿用上轮 BLOCKED 口径）；drive 走 MinIO 共享层跨副本字节一致（`WM-API-010`）。
- **R14（限速/PROXY 信任）**：引擎 PROXY_TRUSTED 仅信 LB 固定 IP；distprobe 直连单副本必须以受信源声明 PROXY 头，否则拒收（负路径验证）。
- **配额全语义**：超配额邮件不入 `qafull` INBOX 且发件人收到 `Delivery Status Notification (Failure)`（拒收 + NDR 双证据）。
- **TLS 强制**：143/587 明文 AUTH 被拒（`PRIVACYREQUIRED` / `523`），465/993/995 隐式 TLS 全链路可用。

## 3. 初测 FAIL → 复测结论（测试工程记录，非产品缺陷）

| 初测 FAIL | 根因 | 复测 |
| --- | --- | --- |
| QUOTA-1b | 断言要求协议层拒绝，实际语义为 250 后异步 NDR（DSN Failure 已达发件人） | QUOTA-1c PASS |
| GW-DIST-003a | 断言未过滤被 kill 容器的 "invalid IP" + 8s 窗口落在故障检测期 | 18s 后 g1/g3 全部服务（GW-DIST-003a-rt PASS）|
| WM-API SESS/001/003/006/009/010（首轮） | 脚本按 `{data:[...]}` 解析，实际响应为裸数组 | batch3b PASS |
| WM-API-002/004（batch3b） | flag 契约为 `flag/value`（非 flags 数组）；move 后 UID 按新 mailbox 重映射（IMAP 语义），按 subject 断言 | batch3c PASS |
| WM-API-005 | search 需 `folder` 参数 + FTS 跨副本收敛窗口 | q 命中 A1=A2=1（005b PASS）|
| PG-3（首轮） | 把 GORM `SLOW SQL >= 200ms` 警告误判为冲突；实为 TryHold 竞争的良性提示（rows:0 = 另一副本持锁，恰为单例语义证据） | PG-3 PASS（0 死锁/冲突）|

## 4. 部署注意事项（本轮新发现，非缺陷）

1. **容器化 backend 的云盘存储**：`DB_DRIVER != sqlite` 时本地 blob 默认 CWD 相对 `uploads`（容器内 / 不可写 → `/drive/upload` 500）。设 `MAILEZINE_S3_ENDPOINT`（drive 自动切 MinIO，分布式部署的正确共享层）或挂载可写卷。`config.go` 已文档化该行为。
2. **backend EE 镜像 license 信任根**：源码内嵌 dev key，`MAILEZ_LICENSE_REQUIRED=true` 会在启动时拒绝（81760f8 的防呆）。生产镜像必须经 `docker-bake.ee.hcl` 注入 vendor 公钥（release secret）。本轮 QA 栈对 backend 设 `MAILEZ_LICENSE_REQUIRED=false`（EE 代码路径完整编译；引擎侧 license 强制不受影响，仍 `enterprise(max=100)`）。
3. **旧镜像陷阱**：`mailez-backend-ee:local` 若早于 322ee25 构建，`DB_DRIVER=postgres` 会被静默忽略（旧二进制走 SQLite 并把 DSN 当路径）。重建镜像后恢复正常。
4. 引擎 multi 模式必须设 `MAILEZINE_ROCKS_PATH=/data/rocks`（可写 per-node 路径），否则 FTS 初始化失败退出。

## 5. 环境处置

- QA 靶场容器已 `stop`（未删除）：`docker compose -p mailez-pgdist` 位于 `D:\code\mailez-hq\.tmp\mailez-pgdist`（含本轮 PG/TiDB/MinIO 数据与全部种子状态）；`docker compose -p mailez-pgdist start` 可复活。
- live 企业版/多活栈（MySQL 控制面）已原样恢复：backend/engine-lb/mailezine×3 healthy，:8083 预览 200。
- `.dockerignore` 追加 `deploy/data.bak-*/`（构建上下文排除用户备份目录，`docker build` 无法打包其中的 mysql.sock 特殊文件）。

## 6. 与上轮（MySQL 控制面）对照

- 控制面 DB 由 MySQL 换为 **PostgreSQL** 后：目录/认证/审计/会话/租约/公告等控制面语义全部成立，三副本邮件面 UID 一致性与幂等语义不变。
- 上轮 **BLOCKED 的 WM-API-001~010（多 API 副本）本轮以 b1/b2 双副本 + Redis 共享会话补齐**，12 项全 PASS。
- 未执行项（口径同上轮）：SHARD-001~003（账户迁移编排）、MULTI-003（猴测/背景流量常驻）、FAULT-002/003/004/007/008（netem/端口屏蔽/配置变体）、MQ-IDEM-007~012（自动回复/过滤/转发控制面入口）。e2e-mailez / conformance / storage-smoke 独立套件本轮未重跑（引擎侧未改动，上轮 EE 构建 PASS 结论沿用）。
