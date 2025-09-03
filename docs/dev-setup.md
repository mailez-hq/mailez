# mailez 混合开发模式

目标是：**邮件栈基础设施（自建镜像）用 Docker 编排、基本不动；mailez
自研部分（backend / webmail / admin / SSO）在宿主机以开发模式运行**，改代码
即热更新。

## 国内网络加速（可选）

邮件栈镜像构建和依赖下载在国内可能很慢，已按下面的方式处理：

- **apk（Docker 镜像构建内）**：各组件 Dockerfile 已内置阿里云镜像源
  （`APK_MIRROR` ARG，bake 构建时用同名环境变量覆盖，如
  `APK_MIRROR=dl-cdn.alpinelinux.org docker buildx bake`）。
- **npm（前端）**：`frontend/.npmrc` 已指向 npmmirror。
- **Go modules（后端）**：本机执行一次，写入全局 Go 配置：
  ```
  go env -w GOPROXY=https://goproxy.cn,direct
  ```
- **Docker Hub 基础镜像（alpine / redis 等）**：当前直连可用时无需配置；如果
  拉取慢，可在 Docker Desktop → Settings → Docker Engine 中配置
  `registry-mirrors`（如 `https://docker.1ms.run`），改完需重启 Docker Desktop。
  注意：DaoCloud 公共镜像（`docker.m.daocloud.io`）有白名单限制，部分镜像会
  被拒绝（如 `library/system`），不建议依赖。
- 如果本机有 Clash 等代理软件，也可以给 Docker Desktop 配代理（Settings →
  Resources → Proxies），或在本终端设置 `HTTP_PROXY` / `HTTPS_PROXY`，构建
  脚本会继承。

## 端口约定

| 服务 | 地址 | 说明 |
|---|---|---|
| 后端 API（本地） | `http://localhost:8080` | **必须 8080**：邮件栈镜像通过 `MAILEZ_BACKEND_ADDRESS:8080` 访问内部 API |
| webmail（本地 dev） | `http://localhost:3001` | `npm run dev -- -p 3001` |
| admin（本地 dev） | `http://localhost:3000` | `npm run dev -- -p 3000` |
| IMAP（引擎直发布） | `127.0.0.1:143` | mailezine 容器（gateway 只做 HTTP/ACME） |
| SMTP 提交（引擎直发布） | `127.0.0.1:1587` | mailezine 容器 |
| ManageSieve（引擎直发布） | `127.0.0.1:4190` | mailezine 容器 |

## 一次性准备

1. 安装 Docker Desktop（WSL2 后端）。
2. 准备 `deploy/mailez.env`：
   ```
   cd deploy
   copy mailez.env.example mailez.env
   ```
   至少修改 `MAILEZ_SECRET_KEY`（≥16 字节）；本地开发保持 `MAILEZ_TLS=off`。

## 启动邮件栈（基础设施，只起一次）

```
cd deploy
docker compose -f docker-compose.dev.yml up -d
```

开发档只有两个组件职责：redis / mail-filter(rspamd) / resolver 作为引擎
依赖，mailezine 引擎直接发布邮件端口（25 / 1587 / 110 / 143 / 4190）到
宿主机。引擎通过 `host.docker.internal:8080` 访问宿主机 mailez 后端的
目录/认证接口（SQLite 存储）。两个生产版本是独立文件（全容器化）：
`docker-compose.ce.yml`（社区版 mailezine + SQLite 默认控制面 +
单节点存储）与 `docker-compose.ee.yml`（企业版 mailezine +
MySQL + TiDB + MinIO/S3），见 `deploy/scripts/README.md`。

## 启动 mailez（本地开发模式）

后端（终端 1）：
```
cd backend
powershell -File .\dev-start.ps1    # 默认企业版全功能；内含 SQLite DSN + 引擎端口直连配置
powershell -File .\dev-start.ps1 -Ce  # 社区版（企业功能位显示降级提示）
```

等效的手动环境变量（与 `dev-start.ps1` 一致）：
`MAIL_IMAP_ADDR=127.0.0.1:143`、`MAIL_SMTP_ADDR=127.0.0.1:1587`、
`MAIL_SIEVE_ADDR=127.0.0.1:4190`、`DB_DSN=<绝对路径>/mailez.db`
（SQLite 开发档）。

## 引擎与存储档位

存储按版本选择：

| 版本 | compose 文件 | 引擎 | 控制面 | 存储 |
| ---- | ------------ | ---- | ------ | ---- |
| 开发 | `docker-compose.dev.yml` | mailezine | SQLite（宿主机） | Pebble + 本地 FS |
| 社区版 | `docker-compose.ce.yml` | mailezine | SQLite 默认（可选 MySQL） | Pebble + 本地 FS |
| 企业版 | `docker-compose.ee.yml` | mailezine | MySQL | TiDB + MinIO/S3 |

引擎与全部组件镜像构建：仓库根目录 `docker buildx bake`（构建定义
`docker-bake.hcl`，默认 CE 档、tag `:local`；引擎上下文经 `MAILEZINE_CONTEXT`
指向相邻 mailezine 仓库）。冒烟：`deploy/scripts/smoke-mailezine.sh`
按当前档位端口传参。

初始化用户（首次）：
```
go run ./cmd/seed
```

webmail（终端 2）：
```
cd frontend/apps/webmail
npm run dev -- -p 3001
```

admin（终端 3）：
```
cd frontend/apps/admin
npm run dev -- -p 3000
```

发一封测试邮件验证端到端（终端 4）：
```
cd backend
go run ./cmd/e2e -api-port 8080 -smtp-port 25 -imap-port 143 --domain e2e.example.com --alias team
```

`-smtp-port 25`：开发栈 MAILEZ_TLS=off，587 未监听，25 走
`smtp_auth none` 入站路径；`-imap-port 143`：mailezine 直发的 IMAP 端口。

登录：`admin@example.com` / `MailezDemo2026!`（webmail 与 admin 共用 SSO）。

## 为什么后端必须监听 8080

邮件栈镜像内部把控制面地址写为 `MAILEZ_BACKEND_ADDRESS:8080`（nginx 网关、
mailezine 引擎的目录/认证查询都走它）。开发模式下 `MAILEZ_BACKEND_ADDRESS` 被
`docker-compose.dev.yml` 覆盖为 `host.docker.internal`，因此宿主机后端必须
监听 8080，前端 `next.config.ts` 的默认 `API_TARGET` 也指向
`http://localhost:8080`。容器化部署时用环境变量 `API_TARGET=http://backend:8080`
覆盖即可（见 `docker-compose.ee.yml`）。

## 常见问题

- `imap dial: lookup gateway: no such host`：后端没配置 `MAIL_IMAP_ADDR` 为
  宿主机映射端口，或邮件栈没启动。按上文环境变量设置并确认
  `docker compose -f docker-compose.dev.yml ps` 全部 healthy。
- 想直接改邮件栈配置：挂载目录 `deploy/overrides/`（nginx/rspamd）与
  `deploy/data/mailezine`（邮箱数据）都保留在项目里，方便日后调整。

## 自建镜像（完全本地构建，无外部镜像仓库依赖）

邮件组件（nginx / rspamd / macro-scanner / unbound）的 Dockerfile 与静态
配置在 `deploy/images/`（共享基础设施）下，mailezine 引擎镜像从相邻仓库
`../mailezine`（bake 的 `MAILEZINE_CONTEXT` 指向它）构建，全部为多阶段自建镜像
（Go 编译 agent + 官方 `alpine:3.21`，无任何第三方邮件镜像依赖）。构建
入口为仓库根目录的 `docker buildx bake`（定义见 `docker-bake.hcl`），
本地构建产物为 `ghcr.io/mailez-hq/mailez-*:local`：

```
# 1. 本地构建全部 CE 组件（首次较慢；需相邻 ../mailezine 仓库）
docker buildx bake
# 2. 直接启动（dev 档默认使用 :local 镜像）
docker compose -f docker-compose.dev.yml up -d
```

镜像内的配置生成与协议代理全部由自研 Go agent 实现（模板内嵌进二进制），
不依赖任何第三方邮件栈代码。`pull_policy: missing` 保证本地已有镜像时
**不联网**。
