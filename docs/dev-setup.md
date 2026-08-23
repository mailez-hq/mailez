# mailez 混合开发模式

目标是：**邮件栈基础设施（自建镜像）用 Docker 编排、基本不动；mailez
自研部分（backend / webmail / admin / SSO）在宿主机以开发模式运行**，改代码
即热更新。

## 国内网络加速（可选）

邮件栈镜像构建和依赖下载在国内可能很慢，已按下面的方式处理：

- **apk（Docker 镜像构建内）**：各组件 Dockerfile 已内置阿里云镜像源
  （`APK_MIRROR` ARG，构建时可用 `--build-arg` 覆盖）。
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
| 后端 API（本地） | `http://localhost:8080` | **必须 8080**：邮件栈镜像通过 `BACKEND_ADDRESS:8080` 访问内部 API |
| webmail（本地 dev） | `http://localhost:3001` | `npm run dev -- -p 3001` |
| admin（本地 dev） | `http://localhost:3000` | `npm run dev -- -p 3000` |
| IMAP 代理（容器→宿主映射） | `127.0.0.1:10143` | gateway 容器的内部代理端口 |
| SMTP 提交（容器→宿主映射） | `127.0.0.1:10025` | gateway 容器的内部提交端口 |
| ManageSieve | `127.0.0.1:4190` | gateway 容器 |

## 一次性准备

1. 安装 Docker Desktop（WSL2 后端）。
2. 准备 `deploy/mailez.env`：
   ```
   cd deploy
   copy mailez.env.example mailez.env
   ```
   至少修改 `SECRET_KEY`（≥16 字节）；本地开发保持 `TLS_FLAVOR=notls`。

## 启动邮件栈（基础设施，只起一次）

```
cd deploy
docker compose -f docker-compose.dev.yml up -d
```

这会启动 redis / gateway(nginx) / mail-store(dovecot) / mta(postfix) /
mail-filter(rspamd) / macro-scanner / resolver，并自动把内部代理端口
10143 / 10025 / 4190 映射到宿主机。`docker-compose.dev.yml` 里的 gateway
容器通过 `host.docker.internal` 访问宿主机的 mailez 后端（8080）。

## 启动 mailez（本地开发模式）

后端（终端 1）：
```
cd backend
$env:MAILEZ_PORT='8080'
$env:MAIL_STORE_ADDRESS='192.168.206.5'   # 固定 IP：nginx auth 只认 IP（Auth-Server）
$env:MTA_ADDRESS='192.168.206.4'          # 与 docker-compose.dev.yml 的静态 IP 对应
$env:MAIL_IMAP_ADDR='127.0.0.1:10143'
$env:MAIL_SMTP_ADDR='127.0.0.1:10025'
$env:MAIL_SIEVE_ADDR='127.0.0.1:4190'
$env:DB_DSN='D:\code\mailess\backend\mailez.db'   # 建议绝对路径，避免工作目录歧义
go run ./cmd/server
```

`MAIL_STORE_ADDRESS` / `MTA_ADDRESS` 会被内部 API 以 `Auth-Server` 头返回给
nginx/dovecot 代理；nginx 1.26+ 的 mail auth 模块只接受 IP 字面量，所以
必须是固定容器 IP（compose 里 mail-store/mta 已配静态地址，见
`docker-compose.dev.yml`）。`MAIL_IMAP_ADDR` 等则是后端自己连邮件栈用的
宿主机映射端口，两者别混。

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
go run ./cmd/e2e -api-port 8080 -smtp-port 25 -imap-port 10143 --domain e2e.example.com --alias team
```

`-smtp-port 25`：开发栈 TLS_FLAVOR=notls，587 未监听，25 走
`smtp_auth none` 入站路径；`-imap-port 10143`：143 默认关闭，10143 是
gateway 容器里的 imap-webmail 代理端口。

登录：`admin@example.com` / `MailezDemo2026!`（webmail 与 admin 共用 SSO）。

## 为什么后端必须监听 8080

邮件栈镜像内部把控制面地址写为 `BACKEND_ADDRESS:8080`（nginx 认证代理、
dovecot passdb、postfix 查询都走它）。开发模式下 `BACKEND_ADDRESS` 被
`docker-compose.dev.yml` 覆盖为 `host.docker.internal`，因此宿主机后端必须
监听 8080，前端 `next.config.ts` 的默认 `API_TARGET` 也指向
`http://localhost:8080`。容器化部署时用环境变量 `API_TARGET=http://backend:8080`
覆盖即可（见 `docker-compose.yml`）。

## 常见问题

- `imap dial: lookup gateway: no such host`：后端没配置 `MAIL_IMAP_ADDR` 为
  宿主机映射端口，或邮件栈没启动。按上文环境变量设置并确认
  `docker compose -f docker-compose.dev.yml ps` 全部 healthy。
- 想直接改邮件栈配置：挂载目录 `deploy/overrides/`（nginx/rspamd）与
  `deploy/data/mail`（邮箱数据）都保留在项目里，方便日后调整。

## 自建镜像（完全本地构建，无外部镜像仓库依赖）

邮件栈组件（nginx / dovecot / postfix / rspamd / oletools / unbound）的
Dockerfile 与静态配置在 `deploy/vendor/mailstack/`，全部为多阶段自建镜像
（Go 编译 agent + 官方 `alpine:3.21`，无任何第三方邮件栈镜像依赖）。两个
compose 文件默认就引用本地构建的 `mailez/*:local`：

```
# 1. 本地构建全部组件（首次较慢）
powershell -ExecutionPolicy Bypass -File deploy/scripts/build-images.ps1
# 2. 直接启动（compose 已默认 mailez/*:local）
docker compose -f docker-compose.dev.yml up -d
```

镜像内的配置生成与协议代理全部由自研 Go agent 实现（模板内嵌进二进制），
不依赖任何第三方邮件栈代码。`pull_policy: missing` 保证本地已有镜像时
**不联网**。
