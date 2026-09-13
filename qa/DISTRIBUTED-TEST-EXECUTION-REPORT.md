# 分布式测试执行报告（Distributed Test Execution Report）

- 依据方案：`qa/DISTRIBUTED-TEST-PLAN.md`（138 测试点）
- 执行窗口：2026-09-03（UTC+8 凌晨），单会话自动化 + 活栈演练
- 执行环境：
  - 活栈（被测靶场）：`mailez` compose 企业栈 —— 3 × mailezine 副本（`MAILEZINE_CLUSTER_MODE=multi`、`storage=tidb`、`PROXY_PROTOCOL=all`、受信 LB `192.168.206.250/32`）+ `mailez-engine-lb-1`（haproxy TCP L4，server-template 1-16 + 健康检查摘除）+ backend（auth/directory 模式，127.0.0.1:8081）+ 共享 TiDB（mailez-tidb-1）+ MySQL/Redis/MinIO
  - 被测版本：mailezine `main@a038da2`（本报告执行中产出 1 个修复提交）、mailez `dev@bf9170a`
  - 测试账号（经 backend 管理 API 实供给）：`test.dist` 域 + `user_a` / `user_b` / `qadisabled`（后置禁用）/ `qafull`（后置配额 1000B），口令统一 `DistTest#2026!`
  - 探针：临时 QA 工具 distprobe（SMTP/IMAP/POP3 客户端 + PROXY v1 注入 + LB 内直连单副本能力），执行后已移出仓库（存于 `.distprobe/`，不入库）
- 恢复动作：演练中被 kill -9 的副本均以 `docker start` 恢复；被暂停的 mailez-tidb-1 已恢复 `Up (healthy)`；活栈终态对账 `user_b exists=15` 与演练一致，零丢失。

## 1. 结论总览

| 大类 | 执行口径 | 结果 |
| --- | --- | --- |
| 基线：mailezine `go test ./...`（CE） | 全量套件 | **PASS**（exit 0） |
| 基线：mailezine `go test -tags mailez_ee ./...` | 全量套件（EE） | **PASS**（仓库包 38/38 ok；唯一 FAIL 为执行时误入仓库的 QA 探针目录，已移出，见附录） |
| 基线：mailez/backend `go test ./...` | 全量套件 | **PASS**（exit 0） |
| backend 全链路 E2E（`mailez-backend-e2e.sh`） | login→provision→SMTP→IMAP | **PASS 6/6** |
| TiDB 分片存储 E2E（`tidb-dev.ps1 -Smoke` 语义，EE 构建手动执行） | storage=tidb 收发闭环 | **PASS**（submit→IMAP→POP3；原脚本问题见 D-1） |
| MinIO blob 闭环（`minio-storage-e2e.sh` 语义，EE 镜像手动执行） | 落桶内容寻址 + 跨重启持久化 | **PASS 4/4**（发现 D-2/D-3 已处理） |
| 活栈演练：跨网关投递/一致性/故障转移/队列接管/滚动重启/NDR | 详见 §3 | **核心链路全 PASS** |
| WM-API-001~010（多 backend 实例） | 活栈仅 A1 单副本 | **BLOCKED**（环境缺口，见 §6） |
| SHARD-001~003（账户迁移）、MULTI-003（猴测）、FAULT-002/003/004/007/008、MQ-IDEM-003/004/007~012 | 需迁移编排/背景流量/netem/配置变体 | **BLOCKED / 未执行**（见 §6） |

**总体判定：分布式核心语义（多活一致性、故障转移、队列租约接管、NDR、共享存储收敛）在活栈上全部成立；两个独立存储 E2E 以企业构建全 PASS；无新增产品缺陷，产出 3 项工具链发现（其中 1 项已修复推送）。**

## 2. 基线与独立套件明细

### 2.1 两仓全量测试基线
- `mailezine`：`go test ./...` exit 0（job 输出 `.test-baseline-mailezine.txt`）。
- `mailezine`（EE 门控套件）：`go test -tags mailez_ee ./...` → `ok mailezine/cmd/mailezine 55.899s` 及 internal 全部 `ok`，含 `internal/ee/{ha,dlp,storeee,archive}`、`internal/{kvlease,queue,snooze,license}`（输出 `.test-baseline-mailezine-ee.txt`）。**注意：CE 基线不编译任何 EE 测试（`multi_ee_test.go` 等整文件 `//go:build mailez_ee`），多活 kill -9 演练类用例只在 EE 套件内运行——基线口径必须包含 EE tag。**
- `mailez/backend`：`go test ./...` exit 0（输出 `.test-baseline-mailez.txt`）。
- §9 继承项（SMTP-INV/IMAP-INV/POP3-INV/MIME-INV/WM-BIZ 等 62 项）的自动化部分由上述套件承载：`internal/{smtp,imap,pop3,mailmime→mailbuffer,directory,quota…}`、`cmd/conformance` 均 ok；活栈补充验证了其中跨实例语义（见 §3）。

### 2.2 backend 全链路 E2E（隔离栈）
`mailezine/deploy/scripts/mailez-backend-e2e.sh`（自建 mailez sqlite 栈 + pebble 容器引擎）：
```
[PASS] backend health / admin login / create domain (201) / create user (201)
[PASS] smtp submission / imap delivery —— 6/6 checks passed
```

### 2.3 TiDB 分片存储 E2E
- 原脚本 `tidb-dev.ps1 -Smoke` 以 CE 构建启动引擎，报 `storage: backend "tidb" requires the enterprise edition`（发现 D-1）。
- 按脚本语义以 `-tags mailez_ee` 手动构建执行（活栈 TiDB 暂停让出 127.0.0.1:4000，事后恢复）：
  - 引擎 `cluster=single storage=tidb directory=dev auth=dev`，health :11480 就绪
  - `storage-smoke -mode send`：`[PASS] smtp submit / imap read / pop3 read`
  - `storage-smoke -mode check`：`[PASS] imap read / pop3 read`
  - **SHARD-001~011 的存储层自动化载体可用性由此确认（EE 构建前提）。**

### 2.4 MinIO blob 闭环 E2E
- 原脚本两处问题：并发执行时宿主端口竞争（发现 D-2 相关）+ 默认构建 CE 镜像（S3 blob 同样受 EE 门控）。
- 按脚本语义以 `--build-arg MAILEZ_EDITION=ee` 构建镜像手动执行：
  - 引擎日志确认 `storage: blob backend=s3 bucket=mailezine`
  - `storage-smoke -mode send -s3-*`：`[PASS] smtp submit / imap read / pop3 read / minio blob present`（4/4）
  - `docker restart` 引擎后 `-mode check`：`[PASS] imap read / pop3 read` —— **跨重启持久化成立**
  - 桶内对象键为内容寻址 `sha256-3f7d…`（542B），`mc cat` 确认对象内容即完整 RFC822 消息（Received/Message-ID/Spam 头/正文）
  - smoke 的 blob 断言过期（期望 `email-` 前缀）→ 已修复并推送（D-3，mailezine `a038da2`）

## 3. 活栈演练明细（对账口径：Message-ID 四方视图 = LB + G1 + G2 + G3）

### GW-DIST-001 SMTP 连续会话跨网关漂移 — PASS
- user_a 经 LB:465（隐式 TLS）三次独立会话提交 `gw001-1/2/3@dist.test`；四方视图各恰好 3 封、MID 齐整、零重复零丢失；入箱可见性 0.9s。

### GW-DIST-007 POP3 轮流连接不同网关一致性 — PASS
- LB:995 与 G1/G2/G3 直连（PROXY v1 + 隐式 TLS）UIDL 全部为 `1 1 / 2 2 / 3 3`。

### GW-DIST-008 POP3 两种模式跨实例 — PASS
- 下载删除模式：POP3 `DELE` 3 封后 IMAP 视图 `exists=0`（删除全局生效）；保留模式由此前多轮 POP3 UIDL/STAT 会话隐式验证（IMAP 视图不变）。

### SHARD-007 / SHARD-008 UID 与快照全局一致 — PASS
- 连续对账四点：`uidvalidity=1` 恒定；`uidnext` 随投递同步推进（4→8→13→16）；三副本逐 UID 的 MID/FLAGS 快照逐字节一致（含 G2 重 join 后全量收敛）。

### MULTI-001 三端标志同步链 — PASS
- G1 `STORE +\Seen` uid=1 → G2 视图即时可见；G2 `STORE +\Flagged` uid=2 → G3 即时可见（共享 TiDB，无脏读窗口）。

### MULTI-002 IMAP 删除全局传播 — PASS
- G1 `UID EXPUNGE` uid=3 → G2/G3 `exists=14`、目标消息消失，`uidnext=16` 一致（EXPUNGE 不回填 UID）。

### MQ-IDEM-001/002 变体：投递中副本 kill -9 的租约接管 — PASS
- 绕过 LB 直打 G1:25 连续收下 5 封（`takeover-1..5`），3 秒后 `kill -9` G1；30s 后 G2/G3 接管：5 封全部入箱（uid 8-12）、两副本一致、**零重复零丢失**（at-least-once 队列 + 恰好一次落库）。
- 同 Message-ID 两次独立会话提交 → 入箱 2 份（uid 4/5）：**预期语义**（"恰好一次"约束单次投递的接管重试，不构成跨提交去重义务；R1 边界文档应写明，见 O-2）。

### MQ-IDEM-005 / XDOMAIN-005 NDR 恰好一封 — PASS
- user_a 认证提交至不可达域 `ext@outside.fake`（outbound=true）→ 队列重试后生成 DSN，user_a 收到恰好 1 封 `Delivery Status Notification (Failure)`（`mailezine-dsn-3@…`）。

### FAULT-005 变体：单副本队列 worker 全灭 — PASS（变体口径）
- 完整"全部副本 kill 30s 后拉起"未在活栈执行（会中断共享靶场）；单副本投递中 kill + 接管已覆盖同语义（上条）。全灭矩阵留待独立演练窗口。

### FAULT-006 单副本滚动重启 — PASS
- 依次 `docker restart` G3→G2→G1（各间隔 12s），期间 3 封提交（`roll-1/2/3`）全部成功；三副本终态 `exists=15`、`uidnext=16` 完全一致，无用户可见错误。

### MULTI-002 关联：故障转移期间的副本重 join — PASS
- G2 kill 后 LB 提交零失败（摘除生效秒级）；`docker start` 后 18s 内 G2 视图与 G1/G3 全量一致（含 FLAGS）。

### GW-DIST-003 / MULTI-004 可见性延迟（轮询口径） — PASS（变体）
- 以轮询 SELECT 度量投递可见性：0.6s / 0.7s / 0.9s（门槛 p95 ≤5s）。IDLE 推送语义本身由 `internal/imap` 套件（IMAP-INV-015）承载；真 IDLE 挂连接 + 中途 kill 的演练留待 MULTI-004 完整执行。

### §9.7 QUOTA / DISABLED — PASS（认证面）+ 观察项
- `qadisabled`（enabled=0）：SMTP AUTH `454 invalid credentials`、IMAP LOGIN `NO [AUTHENTICATIONFAILED]` —— 禁用即拒，双协议一致。
- 入站投给禁用/满额用户：RCPT 均接受（引擎侧队列收下）；满额拒绝路径与入箱后的配额拦截留 §9.7 套件深测（O-3）。

### TLS 强制（SMTP-INV-003 语义 + 部署策略）— PASS
- 明文端口认证一律拒绝：SMTP `523 TLS is required`、IMAP `NO [PRIVACYREQUIRED]`；465/993/995 隐式 TLS 全链路可用；PROXY v1 头在 TLS 前解析（D47 顺序）经 LB 与 LB 内直连双路径验证。

## 4. 发现清单

| 编号 | 级别 | 内容 | 处置 |
| --- | --- | --- | --- |
| D-1 | 工具链 | `tidb-dev.ps1 -Smoke` 用 CE 构建启动引擎，TiDB 存储被企业门控拒绝（`internal/app/storage.go` registry seam），脚本的 SHARD 自动化载体实际不可用 | 未改仓库脚本；以 `-tags mailez_ee` 手动执行验证通过。**建议**：脚本 `-Smoke` 构建追加 `-tags mailez_ee` 或文档注明 EE 前提 |
| D-2 | 工具链 | `minio-storage-e2e.sh` 默认 `docker build`（CE 镜像），S3 blob 同受门控；且与其它 E2E 并发时宿主端口竞争（11025/11587/11143） | 以 `--build-arg MAILEZ_EDITION=ee` 手动执行验证通过；建议脚本串行锁 + EE 构建参数 |
| D-3 | QA 工具（已修复） | `storage-smoke s3Check` 断言 blob 键以 `email-` 开头，而生产写路径为内容寻址 `sha256-<hex>`（`internal/mailstore/kv.go`），导致 MinIO 闭环必然误报 FAIL | 已修复（接受两种键形）并推送 `mailezine@a038da2`；修复后 4/4 PASS |
| O-1 | 观察 | 引擎把 haproxy 健康检查的正常连接关闭记为 ERROR `connection reset by peer`（每 2s × 5 端口），日志噪声大，会淹没真实错误 | 建议按 PROXY/健康检查 UA 或检查端口降噪 |
| O-2 | 契约文档 | 同 MID 跨提交不去重（入箱多份）。符合 RFC 语义，但 R1"至少一次 + 业务恰好一次"的表述应明确只约束**单次投递的接管重试** | 建议在架构文档 R1 边界补一句 |
| O-3 | 测试缺口 | 满额配额用户：入站 RCPT 接受，配额拦截发生在后续投递/入箱阶段，本次未断言满额拒绝与 NDR 行为 | 留 §9.7 套件 + QUOTA 变体深测 |

## 5. 风险清单（方案【上线高危风险】）映射更新

- R1（提交/入队边界）：接管演练佐证"队列提交→租约→恰好一次落库"成立；前端防重（WM-API-006）仍依赖前端幂等键，未测。
- R2（fencing/双投）：接管零双投证据 +1（EE 套件 `kvlease/queue` ok）；迟到写回 fencing 专项未在活栈执行。
- R4（迁移 cutover）：BLOCKED（无迁移编排环境）。
- R5（认证/缓存 TTL）：活栈为目录直查模式（auth=mailez 直连 backend），缓存 TTL 变体（F7）未执行；EE 套件 ok。
- R6（改密不吊销会话）：未执行（继承已知 P1 缺口，方案本身已列为上线必改项，非本次新发现）。
- 其余（自动回复风暴 R3、blob GC R10 等）：BLOCKED 或由 EE 套件覆盖。

## 6. BLOCKED / 未执行清单（含环境缺口）

| 用例 | 阻塞原因 |
| --- | --- |
| WM-API-001~010 | 活栈 backend 仅 A1 单副本、8081 固定宿主映射；需 compose 覆盖扩 A2 + webmail 流量入口才能执行多实例语义 |
| SHARD-001~003（账户迁移/UID 迁移语义/cutover 在途） | 需迁移编排工具与双分片目标环境 |
| MULTI-003（10 分钟三端猴测）、MULTI-006（幽灵监控常驻） | 需背景流量脚本与对账监控常驻（本轮以定向对账替代） |
| FAULT-002/003/004/008（网络黑洞/TiDB 屏蔽/S3 屏蔽/TTL 竞态） | 需 `tc netem`/端口屏蔽/配置变体重启的独立演练窗口（本次窗口内不中断共享靶场） |
| FAULT-007（LB 误配置摘除 2/3 副本） | 需临时改 haproxy 配置，影响共享靶场 |
| MQ-IDEM-003/004/007/008/009/010/011/012 | 依赖自动回复/过滤/转发的控制面入口（backend API 路径待定）与死信可见性管理端点；queue/snooze/kvlease 语义已由 EE 套件覆盖 |
| SMTP-INV-014（delayed DSN） | 需队列滞留阈值配置变体（MAILEZINE_QUEUE_DELAY_WARNING_SECONDS） |

## 7. 附录

- 执行产物：`.test-baseline-mailezine.txt`、`.test-baseline-mailezine-ee.txt`、`.test-baseline-mailez.txt`、`.disttest-smoke.log`、`.disttest-minio.log`、`.disttest-bee2e.log`、`.distprobe/`（探针二进制 + 源码，未入库）。
- 探针要点：LB 侧标准客户端；单副本定向从 `mailez-engine-lb-1` 容器内执行并注入 PROXY v1 头（`DISTPROXY=1`，源声明为受信 LB IP .250）；隐式 TLS 按端口（465/993/995）自动启用。
- 活栈恢复记录：G1/G2/G3 各被 kill -9 一次并恢复；mailez-tidb-1 为 TiDB smoke 让位暂停约 10 分钟后恢复 `Up (healthy)`；终态对账零丢失。
- mailezine 推送：`a038da2`（storage-smoke 断言修复）→ gitee main。
