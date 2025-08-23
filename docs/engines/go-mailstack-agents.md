# 邮件栈容器代理（Go agents）

## 定位

mailez 的每个邮件栈容器（gateway / mta / mail-keeper / mail-filter /
macro-scanner / resolver）都由**同一个静态 Go 二进制**（`mailstack-agent`）
驱动：读取环境变量 → 渲染配置 → 拉起对应服务 → 代理内部协议。配置模板
通过 `go:embed` 编译进二进制，镜像内不携带任何脚本解释器或运行时依赖。

```
容器
├── /mailstack-agent <component>   （唯一进程入口）
├── /etc/<component>/*             （agent 渲染出的配置）
└── /conf 模板（嵌入二进制，可在 /overrides 覆盖）
```

## 组件职责

| 组件 | agent 行为 |
|---|---|
| gateway（nginx） | 渲染 nginx.conf（HTTP + SMTP 邮件代理 + 认证转发）；内嵌 ACME 客户端处理证书；启动 legacy IMAP 登录代理作为 IMAP/POP3/Sieve/Submission 前门 |
| mta（legacy MTA） | 渲染 main.cf / master.cf；实现 socketmap 查询代理；运行 MTA-STS 策略守护 |
| mail-keeper（legacy IMAP） | 渲染 dovecot.conf；实现 dict 代理（passdb/userdb/quota/sieve 回源到控制面） |
| mail-filter（rspamd） | 渲染 `/conf` 模板到 `/etc/rspamd/local.d`；等待控制面就绪 |
| macro-scanner（oletools） | 实现 OLEFY/1.0 扫描协议，供 rspamd 调用 |
| resolver（unbound） | 渲染 unbound.conf；拉取 root hints / 信任锚 |

## 协议契约

内部协议在实现时以**行为契约**为准，并用单测钉死：

- **nginx 邮件认证**：`auth_http` 回源 `/internal/auth/email`；`Auth-Server`
  必须返回 IP 字面量（解析后的地址），不能返回主机名。
- **legacy IMAP dict**：回复格式为 `O<value>\n`；sieve data 键带引号；
  quota 通过 unix socket 代理回源控制面。
- **legacy MTA socketmap**：走 netstring 协议；`${mailez}xxx` 映射全部回源
  控制面 `/internal/*`。
- **rspamd 宏扫描**：OLEFY/1.0 TCP 协议（`Method: oletools`），返回
  olevba 风格 JSON 数组。
- **MTA-STS**：独立策略守护（socketmap），按域查询并缓存策略。

## 通用能力

- **配置渲染**：`text/template` + `go:embed`，支持 `/overrides` 覆盖；
  模板与渲染逻辑分离，便于审计。
- **降权**：agent 以容器内低权限用户运行（非 root），不依赖外部工具。
- **健康检查**：每个组件监听自身状态，供容器 healthcheck。
- **信号处理**：`SIGTERM/SIGINT` 优雅退出；子进程由 agent 统一管理。

## 验证方式

- `go vet` + 单测（配置生成、内部协议解析）
- 构建镜像 → `docker compose up` → e2e 10/10
- 回归：收发邮件、别名/转发/自动回复、DKIM 签名、垃圾过滤、Sieve 规则

## 目录约定

- `backend/cmd/mailstack-agent/`：agent 入口与各组件驱动
- `backend/cmd/mailstack-agent/templates/`：嵌入的配置模板
- `backend/internal/agent/`：共享运行时（socketmap、dict、原子写、降权）
- `deploy/vendor/mailstack/`：各组件 Dockerfile 与静态配置
- `deploy/scripts/build-images.ps1`：本地构建全部镜像
