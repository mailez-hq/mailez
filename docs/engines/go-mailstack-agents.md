# 工程：Go 重写邮件栈容器代理（去 Python）

> 状态：**M1 unbound ✅ / M2 nginx ✅ / M3 legacy IMAP ✅（e2e 10/10）**（2026-08-23）· 优先级：高

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

## Python 链的坑（代码事实）

逐行读 `start.py` / `socrate` / `podop` 后，这层 Python 的问题不是"语言慢"，
而是**工程性缺陷**，Go 重写不止是换语言，是把这些坑一次性填掉：

1. **子进程无人监督**：legacy MTA / legacy IMAP 用 `multiprocessing.Process` 起 podop
   后就没有任何守护逻辑——podop 一旦崩溃，legacy MTA/IMAP stack 照跑，但认证、
   别名、配额查询全部静默失败，故障靠用户发现。
2. **SIGTERM 处理粗暴**：`system.py` 收到 SIGTERM 直接 `sys.exit(143)`，子进程
   （nginx/legacy MTA/IMAP stack）收不到信号，队列、索引可能来不及落盘；exit 143 还
   会让 Docker 显示"非正常退出"。
3. **`os.system()` 大量 shell 字符串**：`chown`、`postconf -e`、`flock`、`killall`
   全是拼接字符串交给 shell，无错误检查、无引号安全，一行写错整个启动失败。
4. **日志用正则过滤**：`LogFilter` 用正则吞掉"噪音"行（legacy MTA 连接断开、
   nginx 解析失败），真出问题时这些被吞的行恰恰是线索——为了"干净"牺牲可观测性。
5. **Jinja + 全量 env 渲染**：整个 `os.environ` 灌进模板，无类型、无校验；
   `TLS_FLAVOR` 拼错要到 nginx 起不来才知道。配置错误全部推迟到运行时。
6. **启动死等无退避**：rspamd `while True: sleep(1)` 死等后端，letsencrypt
   `sleep(5)` 后每 24h 循环——没有超时上限、没有熔断，依赖方挂了就无限刷日志。
7. **运行时脆弱（已踩过的坑）**：CRLF 破坏 shebang、90+ 个 pip 依赖
   （cryptography 等重量级编译包）、venv 体积、pip 源网络问题——整条链的任何
   一环断裂，镜像就构建不出来。
8. **动态代码执行**：oletools `exec(olefy.read())` 直接执行外部脚本且未降权
   （root 运行）——这是 RCE 级隐患，noinet 网络只能防外联，防不了内部篡改。
9. **证书链验证 300 行 Python**：`config.py` 内嵌 ISRG 根证书做链校验/PIN
   检查，Python 标准库做这事又长又绕，Go 的 `crypto/x509` 几行就能完成。
10. **权限处理粗糙**：降权是一次性 `setuid/setgid`；legacy MTA 为了读 `.netrc`
    直接 `chmod /root/ 0755`——把 root 目录放开给低权限用户，不干净。
11. **零单测**：配置生成全靠起容器验证，改一行逻辑要等整条链跑完。

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

## Go 重写的设计原则（利用 Go 优势）

| Python 现状 | Go 方案 | 收益 |
|---|---|---|
| shebang + venv + pip | 静态二进制（`CGO_ENABLED=0`）+ `go:embed` 模板 | 镜像体积降 90%+；无解释器/依赖/行尾问题；天然跨架构（arm64 信创直接 buildx） |
| `os.system()` shell 字符串 | `os/exec` 直接执行 + 显式错误处理 | 无注入面；失败即报错停止，不静默 |
| `multiprocessing.Process` 裸起子进程 | 内置 supervisor：`signal.NotifyContext` + 子进程重启策略 | SIGTERM 优雅转发、等待落盘；子进程崩溃自动拉起 |
| `LogFilter` 正则吞日志 | `slog` 结构化日志，按 level 降噪 + metrics | 不再丢线索；可观测性保留 |
| Jinja 全量 env 无校验 | env → 强类型 `Config` struct + 校验（枚举/端口/必填） | 配置错误启动即报，行号定位 |
| 运行时才暴露模板错 | 模板输出 golden file 单测 + 启动前 `nginx -t` / `postconf -n` / `legacy IMAP -t` | 改配置先过测试再发版 |
| 死等依赖 | 指数退避 + 超时上限；`/healthz` 聚合子进程状态 | 就绪语义清晰，依赖挂了快速失败 |
| certbot 子进程 + 300 行链验证 | `go-acme/lego` 客户端 + `crypto/x509` 标准库 | 一个二进制管 ACME + 链裁剪 + DANE 摘要，内嵌过期监控 |
| `exec()` 动态代码 | 静态代码路径，无动态执行 | 消除 RCE 面 |
| `chmod /root 0755` | 最小权限：capabilities/指定目录 | 安全边界干净 |

额外一条：配置生成器支持**原子写入**（临时文件 + rename），启动失败不破坏
现有配置，配合 compose 的 restart 可安全重试。

## 里程碑

- [x] **M1 unbound agent（Pilot）**：最薄组件，验证框架（env→Config→生成
      conf→exec→健康检查），e2e 10/10 验证通过（容器 COMMAND 已是
      `/mailstack-agent unbound`）
- [x] **M2 nginx agent**：替换 `start.py` + `config.py` + 证书链逻辑；
      Go template 重写 5 个 Jinja 模板（字节级语义一致），单测覆盖
      notls 渲染/端口派生/证书链裁剪；镜像多阶段构建（含静态资源与
      dhparam），e2e 10/10 验证通过（容器 COMMAND 已是
      `/mailstack-agent nginx`）
- [x] **M3 legacy IMAP agent（已验证）**：passdb/userdb/quota/sieve 关联；
      Go 实现 podop legacy IMAP dict 协议（H/L/B/S/C + tabescape，转发
      `/internal/legacy IMAP/`）；dovecot.conf 模板转换 + ham/spam 脚本渲染；
      单测 12/12 + e2e 10/10（含别名投递）
- [ ] **M4 legacy MTA agent**：main.cf/master.cf 生成、别名/中继查询
- [ ] **M5 rspamd agent**：配置渲染（替代 Jinja）、DKIM vault 关联
- [ ] **M6 去 Python**：base 镜像移除 python3/pip/venv/requirements，镜像
      体积与启动时间对比，更新构建脚本

## M3 验证时修掉的三个协议坑（记录在案）

1. **dict 回复格式**：legacy IMAP 官方协议是 `O<value>\n`（无 tab），vendored
   podop 的 `O\t<value>` 在 alpine legacy IMAP 2.3.21.1 上会把 value 解析为空，
   报 `Missing '}'`。Go 实现按官方格式回复。
2. **Auth-Server 必须是 IP**：nginx 1.26+ 的 mail auth 模块用
   `ngx_parse_addr` 解析响应头，只接受 IP 字面量（mainstream suite 的 `get_server`
   会先 `resolve_hostname`，Go 移植版漏了这步）。`serverFor` 现已补上
   主机名→IP 解析；开发模式下后端在宿主机，`IMAP_ADDRESS`/`SMTP_ADDRESS`
   需指向 compose 里固定的容器 IP（192.168.206.5/.4），见
   `backend/scripts/dev-server.ps1`。
3. **sieve data 键带引号**：pigeonhole 的 dict 存储用 JSON 编码的脚本名做
   data 键（`priv/sieve/data/"default"` 带字面引号），后端路由是
   `sieve/data/default/<user>`；Go agent 的 `dictKey` 做引号规范化后才拼
   URL。同时 dev 的 nginx `RESOLVER_ADDRESS` 不再指向 unbound（会 NXDOMAIN
   掉 host.docker.internal/内部服务名），改由 Docker 内嵌 DNS 127.0.0.11
   解析。

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
