# 工程：Go 重写邮件栈容器代理（去 Python）

> 状态：**待办 / 未开始** · 优先级：中（功能稳定后推进）

## 背景与目标

当前邮件栈容器（nginx / legacy IMAP / legacy MTA / rspamd）的启动与配置代理层是
Python：每个容器 `CMD /start.py`，依赖 vendor 进来的 `podop`（内部 API 通信）
与 `socrate`（配置生成/降权）两个库，以及整个 Python venv。

目标：把这层全部重写为 **Go 静态二进制**，镜像内不再需要 Python 运行时。

- 每个容器一个 `mailez-<component>-agent` 二进制（编译进镜像）
- 共享一个 Go 库：模板渲染、podop 协议、健康检查、降权
- 后端仍是现有 Go 控制平面，内部 API 契约不变
- 验收：e2e 10/10 + 邮件收发/管理操作回归 + 镜像体积与启动时间对比

## 现状盘点（需被替换的 Python 逻辑）

位置：`deploy/vendor/mailstack/<component>/`

| 组件 | 入口 | 关键逻辑 |
|---|---|---|
| nginx | `start.py` + `config.py` | 按 env 生成 nginx.conf（TLS 策略/证书路径）、mail 协议代理、认证转发 |
| legacy IMAP | `start.py` | 生成 dovecot.conf、起 podop socket、quota/sieve 关联 |
| legacy MTA | `start.py` | 生成 main.cf/master.cf、MTA-STS、查询 socket |
| rspamd | `start.py` | Jinja 渲染 `/conf/*` → `/etc/rspamd/local.d`、等后端就绪 |
| base（共享） | `socrate` 库 | `set_env`/`conf.jinja`/`drop_privs_to`/`system.py` |
| base（共享） | `podop` 库 | unix socket 代理：legacy IMAP/legacy MTA 查询 → 后端 `/internal/*` HTTP |

## 架构设计

```
┌─────────────────────────────────────────────┐
│ mailez-agent（单个 Go 二进制，静态编译）      │
│  ├─ config：读取 env → 生成组件配置          │
│  │    （Go text/template 或直接重写生成逻辑） │
│  ├─ podop：unix socket 代理 → 后端 /internal │
│  ├─ health：就绪探测/健康检查                │
│  ├─ privs：uid/gid/capabilities 降权         │
│  └─ exec：拉起 nginx/legacy IMAP/legacy MTA/rspamd  │
└─────────────────────────────────────────────┘
```

推荐做法：**不翻译 Jinja 模板，而是直接重写配置生成逻辑**（Go 代码按 env
生成配置），配置内容可控、可审计；podop 协议按 vendored 源码复刻查询格式
（`L<table>\t<key>\t<args>`）。

## 里程碑

- [ ] **M1 骨架**：Go agent 框架 + 模板/配置生成接口 + podop socket 代理 +
      降权 + 健康检查；用一个组件（先 nginx）端到端验证
- [ ] **M2 nginx agent**：替换 `start.py` + `config.py`（TLS 策略、证书路径、
      协议代理、认证转发）
- [ ] **M3 legacy IMAP agent**：passdb/userdb/quota/sieve 关联、podop socket
- [ ] **M4 legacy MTA agent**：main.cf/master.cf 生成、别名/中继查询
- [ ] **M5 rspamd agent**：配置渲染（替代 Jinja）、DKIM vault 关联
- [ ] **M6 去 Python**：base 镜像移除 python3/pip/venv/requirements，镜像
      体积与启动时间对比，更新构建脚本

## 验证方式（每阶段）

- `go vet` + 单测（配置生成、podop 协议解析）
- 构建镜像 → `docker compose up` → e2e 10/10
- 回归：收发邮件、别名/转发/自动回复、DKIM 签名、垃圾过滤、Sieve 规则、
  webmail 全功能

## 注意事项

- 内部 API 契约（`backend/internal/internalapi`，contract_test 钉死）**保持
  byte-compatible**，不可借机改动
- podop 查询格式以 vendored `podop/` 源码为准，先写协议单测再实现
- 降权逻辑参考 `socrate/system.py` 的 `drop_privs_to`（uid/gid 与 base 镜像
  的 `mailez` 用户保持一致）
- 逐组件替换，禁止一次性全换；每个组件替换后独立跑通 e2e
- 完成前保留 Python 方案作为 fallback（compose 可回切）

## 参考

- 待替换源码：`deploy/vendor/mailstack/`（各组件 `start.py`、`podop/`、`socrate/`）
- 内部 API 契约测试：`backend/internal/internalapi/contract_test.go`
