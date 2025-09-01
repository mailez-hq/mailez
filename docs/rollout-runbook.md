# 部署推演 runbook：CE 从零安装 → CE→EE 升级 → EE→分布式扩容

本文是一次**全程实跑**的沉淀（Docker 29 + Compose v5，Windows/macOS/Linux
通用），给出三阶段的最短可复现路径、每阶段验证清单、以及实跑中确认的坑。
阶段细节的官方文档：CE 安装见 `README.ce.md`，升级背景见
`docs/upgrades.md`，多活机制见 `docs/scaling.md`。

前置条件（所有阶段共用）：

- Docker + Compose；`mailez` 与 `mailezine` 两个仓库同级 checkout
  （compose 构建上下文引用 `../../mailezine`）；
- `deploy/mailez.env` 由 `mailez.env.example` 复制而来，必填三项：
  `MAILEZ_SECRET_KEY`（16 字节以上）、`MAILEZ_STACK_SECRET` 与
  `MAILEZINE_STACK_SECRET`（**同值**，内部 /stack API 与引擎管理 API 共用）；
- 宿主端口被占用时改 `MAILEZ_HTTP_PORT` / `MAILEZ_HTTPS_PORT` /
  `MAILEZ_ADMIN_PORT` / `MAILEZ_WEBMAIL_PORT` / `MAILEZ_API_PORT`；
- Linux 宿主需预建 `deploy/data` 并让 uid 1000 可写（见 README.ce.md）。

---

## 阶段一：从零安装社区版（community 档）

```sh
cd deploy
cp mailez.env.example mailez.env        # 编辑上面三项 + 端口
./mailezctl.sh up community             # 自带 --build：构建 backend/前端/引擎等
docker compose --env-file mailez.env -f docker-compose.community.yml \
  exec backend mailez-seed              # 初始管理员 admin@example.com
```

验证清单（实跑全过）：

| 检查 | 命令/预期 |
|---|---|
| 容器 | `docker compose ps`：7 容器全部 healthy |
| API | `curl :8081/api/v1/health` → `{"status":"ok"}` |
| 前端 | `:18082`（admin）、`:8083`（webmail）均 200 |
| 邮件端口 | `25/465/587/143/993/995/4190` 全 OPEN，25 端口 banner `220 … ESMTP` |
| 管理登录 | `POST :8081/api/v1/sso/login` → 200 + `mailez_session` cookie |
| 端到端 | SMTP 投递到 admin@example.com → IMAP LOGIN 后 `SELECT INBOX` 可读 |

---

## 阶段二：CE → EE（enterprise 档）

**诚实边界先读**（`docs/upgrades.md` §3）：跨 KV（Pebble→TiDB）邮件数据
没有原地迁移工具；控制面走"配置导出 → 导入"。本阶段推演验证的是
**license/镜像/功能三件套的切换链路**，生产数据搬运按 upgrades.md 的
IMAP 聚合路径执行。

```sh
# 1. 停 CE 栈（注意 mailezctl down 只清 dev 档，见"坑"清单）
cd deploy
docker compose --env-file mailez.env -f docker-compose.community.yml down

# 2. 签发 license（DEV 内置签名 key；生产用 HQ 签发的正式授权）
cd ../backend
go run ./cmd/license issue --out ../deploy/licenses/license.lic \
  --licensee <你的名字> --mailboxes 100
go run ./cmd/license inspect -in ../deploy/licenses/license.lic
cd ../deploy

# 3. 起 EE 栈（--build 会以 MAILEZ_EDITION=ee 重建 backend/前端/引擎镜像）
./mailezctl.sh up enterprise
docker compose --env-file mailez.env -f docker-compose.enterprise.yml \
  exec backend mailez-seed              # EE 控制面是新的 MySQL，需重新 seed
```

验证清单（实跑全过）：

| 检查 | 预期 |
|---|---|
| 容器 | 12 容器 healthy（新增 mysql/tidb/minio/mail-filter/resolver） |
| 引擎日志 | `msg=starting summary="… storage=tidb … license=enterprise(max=100) junk=rspamd"` |
| backend 构建 | 构建日志出现 `[ "ee" = "ee" ] && … -tags mailez_ee` |
| EE API | `/api/v1/announcement` 204；`/api/v1/dlp/rules`、`/api/v1/archive/messages`、`/api/v1/delegations`、`/api/v1/branding` 均 200 |
| 审计导出(EE) | `GET /api/v1/audit/export` → 200 `text/csv` + `Content-Disposition: attachment` |
| 前端 | admin `/announcement` `/dlp` `/archive` 页面 200（CE 构建无这些路由） |
| 端到端 | 同阶段一（引擎换成 TiDB/MinIO/rspamd 链路） |

**升级语义**：EE 档换了存储底座，CE 的邮件数据不在新栈里（上面验证里的
邮件是新投递的）。生产搬迁 = 配置导出/导入 + IMAP 聚合拉取（upgrades.md
§3），license 文件本身独立于数据，可随时回退 CE。

---

## 阶段三：EE → 分布式（multi 档，引擎多活）

**零数据搬迁**：multi 档只是给 EE 栈叠加引擎副本 + LB，TiDB/MinIO/控制面
原样复用——这是与 CE→EE 的本质区别（存储不动，扩的是无状态服务层）。

```sh
cd deploy
./mailezctl.sh up multi                 # enterprise + multi 叠加，默认 2 副本
# 扩到 3 副本（上限 16 = engine-lb haproxy 槽位）：
MAILEZ_ENGINE_REPLICAS=3 ./mailezctl.sh up multi
```

验证清单（实跑全过，2→3 副本各验一遍）：

| 检查 | 预期 |
|---|---|
| 引擎副本 | `mailez-mailezine-1/2/3` 全部启动 |
| 节点角色 | 每副本 `docker exec mailez-mailezine-N wget -qO- http://127.0.0.1:11480/health` → `{"status":"ok","role":"active","cluster":"multi"}` |
| 端口收敛 | 邮件端口宿主侧只剩 `engine-lb` 一个发布者（引擎 `ports: !override []`） |
| 数据保留 | 原 IMAPS 登录后 INBOX 邮件原样在（TiDB/MinIO 未动） |
| 负载分流 | 连续两次连 25 端口，banner 的 hostname 不同 = lb 在副本间轮询 |
| 端到端 | SMTP 投递（经 lb，落某副本）→ IMAPS 读取（可经另一副本） |

multi 档**行为差异**（实测确认，客户端/监控会感知）：

- 引擎邮件端口只接受带 PROXY v1 头的 LB 连接，backend 的
  `MAIL_*_ADDR` 已由 multi.yml 自动改指 `engine-lb`；
- **143 端口明文 LOGIN 被拒**（`NO [PRIVACYREQUIRED] TLS is required`）：
  隐式 TLS 端口（465/993/995）由引擎终止，依赖 `deploy/certs/cert.pem +
  key.pem`——**缺证书文件时引擎不监听这三个端口，engine-lb 对应后端
  DOWN**。客户端须改用 993/995 或 STARTTLS；
- engine-lb 日志里 `server sieve/node12 … No IP for server` 一类 WARNING
  是 server-template 预留槽位（2..16 尚无对应副本），属预期噪音，副本数
  扩上去后自动纳管。

---

## 踩坑清单（实跑确认）

1. **`mailezctl down` 只清 dev 档**：脚本默认 `TARGET=dev`，只 down
   dev compose 文件里的服务。跑过 community/enterprise/multi 的栈必须
   显式指定文件清理，否则残留容器占端口/网络：
   ```sh
   docker compose --env-file mailez.env -f docker-compose.community.yml down
   docker compose --env-file mailez.env -f docker-compose.enterprise.yml -f docker-compose.multi.yml down
   ```
2. **`cmd/build-images` 产出的是 CE 引擎**：它只构建 nginx/rspamd/
   unbound/macro-scanner/mailezine 五个镜像，且构建 mailezine 时不传
   `MAILEZ_EDITION`——`mailez/mailezine:local` 永远是 CE 档；它也不构建
   backend 与前端。**enterprise/multi 档必须用 `mailezctl up`（自带
   `--build` 与 edition build-args）重建镜像**，"build-images + up
   enterprise"的组合会拿到 CE 引擎/CE 前端。
3. **backend 镜像的 EE 构建依赖 compose build-args**：backend
   Dockerfile 默认 `MAILEZ_EDITION=ce`；enterprise.yml 已传
   `MAILEZ_EDITION: ee`（编译期 `-tags mailez_ee`）。运行时
   `MAILEZ_EDITION` 环境变量只是标记，不改变编译产物——用错了会得到
   "看起来是企业版、实际没有 EE 路由"的镜像。
4. **license 工具参数**：`inspect` 的参数是 `-in <file>`（不是
   `--file`）；`--out` 是相对当前 shell cwd 解析的，在仓库根目录执行时
   注意 `--out deploy/licenses/license.lic` 与 `cd backend` 后执行写出的
   位置不同。DEV key 签发的授权带 WARNING，仅限演练。
5. **`MAILEZ_STACK_SECRET` 留空无法启动生产/社区档**（compose
   `:?` 插值强制非空）——这是防内部 API 裸奔的保护，不是 bug。
6. **EE 首启 SLOW SQL 刷屏**：TiDB/MySQL 首次建表 DDL 慢（200ms+），
   属首次迁移正常现象。
7. **验证脚本注意**：IMAP 143 在 multi 档拒明文 LOGIN（见上）；
   haproxy 多行应答按 `^\d{3}-` 续行读取，别按单行解析。
