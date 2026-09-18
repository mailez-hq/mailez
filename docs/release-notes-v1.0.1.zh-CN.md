# Mailez v1.0.1 发布说明

- **发布日期**：2026-09-18
- **版本类型**：维护版本（Patch）
- **上一版本**：v1.0.0（2026-09-14）
- **升级方式**：将 `deploy/mailez.env` 的 `MAILEZ_IMAGE_TAG` 改为 `v1.0.1`，再执行 `./deploy/mailezctl.sh up ce`

v1.0.1 以**运维自动化**和**缺陷修复**为主：管理台新增出站网络体检、变更式健康告警、IP 封禁引擎、定时加密备份四项能力；客户端接入新增 Delta Chat 扫码登录；大信箱的批量标记、移动、删除显著提速；并修复了 15 处覆盖邮件收发、排序、部署与权限边界的缺陷。

## 一、新增：管理台运维四件套

### 1. 出站网络体检

健康页新增三项**出站方向**的探测，专治「本机看着正常、对方收不到信」这类问题：

- `outbound25`：到外部 25 端口的可达性探测（默认拨测 `aspmx.l.google.com:25`，可用 `MAILEZ_OUTBOUND_PROBE_HOST` 指定其他目标）
- `ptr`：邮件主机名的 PTR 与 EHLO 是否一致
- `resolver`：当前是否在用公共 DNS 解析器（主流信誉类 DNSBL 会拒绝来自公共解析器的查询，导致黑名单检查失效）

### 2. 变更式健康告警

后台 worker 按计划重跑健康检查，**只在状态发生变化时**告警：首次运行只记录基线，状态需连续两次巡检保持一致才对外推送，恢复同样会通知。

- 默认每小时一次，`MAILEZ_HEALTH_ALERT_INTERVAL_MIN` 可调
- 送达运维邮箱，并可同时推送 JSON Webhook（Slack / 钉钉 / 企业微信兼容）
- `MAILEZ_HEALTH_ALERT_MUTE=system:database,domain:example.com:spf` 可静音不关心的检查项（键格式为 `system:<id>` 或 `domain:<域名>:<id>`）
- `MAILEZ_HEALTH_ALERT=off` 完全关闭

### 3. IP 封禁引擎

登录失败按来源 IP 计数，**Web 登录与邮件代理 SASL 共用同一套计数**：默认 600 秒内失败 20 次即封禁该地址，封禁时长按 30 天内的历史累犯逐级升级（15 分钟起，最长 24 小时）。

- 回环地址、部署自身的地址以及 `MAILEZ_BAN_WHITELIST` 中的 CIDR 一律豁免，监控探针不会被自己封掉
- 管理台新增 **Bans** 页面：列出当前封禁、手动解封，操作全部记入审计
- 封禁事件同时进入健康中心

### 4. 定时加密备份

每日备份 worker 会为控制面做一致性快照（SQLite `VACUUM INTO`）并连同附件/云盘目录一起打包，采用分块 AES-256-GCM 加密后投递到目标存储：

- `MAILEZ_BACKUP_TARGET=local:<目录>` 或 `s3`（S3 兼容对象存储）
- **未配置 `MAILEZ_BACKUP_KEY` 时不会执行备份**，不存在明文归档
- 默认保留 14 份（`MAILEZ_BACKUP_KEEP`），按 `MAILEZ_BACKUP_HOUR` 指定的整点运行
- 管理台 **Backups** 页展示配置与运行历史，支持手动触发，并可从目标端重新读取、解密来**校验归档可用性**
- 健康中心按备份新鲜度评分：超过 7 天告警、超过 14 天判失败

## 二、新增：Delta Chat 扫码登录

Webmail 设置对话框中新增 **DCLOGIN v1 二维码**，一次扫入邮箱地址、一次性应用令牌以及来自本部署 autoconfig XML 的 IMAP/SMTP 参数；管理台用户页也提供每用户的「Delta Chat QR」入口，方便运维代为下发。

## 三、新增：ActiveSync 端点（EE）

网关此前没有 `/Microsoft-Server-ActiveSync` 路由，手机只能访问回环上的后端端口，邮件主机名返回 404。现已通过 `deploy/overrides/nginx/eas.conf` 在 443 上发布该端点（EAS 12.1/14.0/14.1），后端端口保持仅回环；手机以 Exchange 账户接入可直连，Outlook 桌面端的 autodiscover 行为不变。

## 四、性能：大信箱批量操作提速

批量旗标操作不再「每封一个事务」：引擎把一批旗标变更合并为一次写入，控制面改为按窗口遍历大文件夹，而不是一次性扫全夹。

| 操作（300 封信箱） | v1.0.0 | v1.0.1 |
| --- | --- | --- |
| `UID STORE 1:* +FLAGS \Seen` | ~34 s | **~7 s** |
| `POST /mail/read-all` | ~30 s | **~14 s** |

## 五、修复清单

**邮件收发与协议**

- 外部 POP3 聚合每轮重复投递：投递去重游标写入了数据库中并不存在的列，写入静默失败导致游标永不推进；现写入 `seen_uid_ls`，且游标写入失败会记日志而不是被丢弃
- 按发件人、主题、大小排序返回空列表：IMAP `SORT` 响应中的 UID 被当作整数读取，而 go-imap 返回的是字符串，收集器把结果全部丢弃
- 正文含超长单行（粘贴的 URL 或日志行）导致发送失败 `mail service error`：提交前按 RFC 5321 行长折行
- 收件人超配额或正文超长只回一个裸 502：现返回 422 及 `recipient_quota_exceeded` / `line_too_long` 与可读文案
- `POST /mail/send` 忽略 `POST /mail/draft` 使用的 `text` 字段，导致按草稿接口调用的客户端发出空邮件：两个字段名现在都被接受
- 删除联系人时，若该联系人属于其他用户，接口返回 204 而非 404

**管理台与 Webmail**

- 部署在 `/admin` 子路径下时，管理台的 logo、品牌资源与 autoconfig XML 仍向站点根请求，三者全部 404
- Webmail 的 Service Worker 从未注册成功（打包的 `sw.js` 中混入了 TypeScript 断言），离线支持与通知点击链路一并失效
- Webmail 向非管理员账号展示管理台入口
- Web 容器运行在 UTC 而非运维所在时区

**部署、运维与依赖**

- 社区版 compose 把引擎邮件端口（25/465/587/110/995/143/993/4190）绑到了 127.0.0.1，全新 `mailezctl up ce` 部署收不到外部邮件；现默认 `${MAILEZ_MAIL_BIND:-0.0.0.0}`，调试端口仍保持仅回环
- 删除用户后 EE 栈的引擎侧邮箱残留：`MAIL_ENGINE_MGMT_ADDR` 缺少 URL scheme，清除请求以 `unsupported protocol scheme "mailezine"` 失败；compose 现已补上 `http://`
- 网关缺少 `/Microsoft-Server-ActiveSync` 路由，手机只能访问回环后端端口、邮件主机名返回 404；现由 `deploy/overrides/nginx/eas.conf` 在 443 上发布（EE，见第三节）
- 新增运维 override compose 文件、宿主机绑定开关与服务密钥透传，便于首次部署加固
- `models.AutoMigrate`（测试与工具使用的全量 schema 助手）缺少 health-snapshot、ban-record、backup-run 三张表；生产迁移不受影响
- 依赖升级：后端 Go modules、前端工作区（Next.js 16.3.5、React 19.3.0）

**权限与安全**

- 登录限速与封禁引擎对**未带转发头**的可信代理请求取到空地址，这类请求共用同一个计数桶；现回退使用 socket 地址
- 域读取开放给域管理员：`GET /domains`、`GET /domains/:name`、DKIM 状态与 DNS 向导返回该管理员持有的域；所有域写操作（创建、修改、删除、生成密钥、管理员、别名域、中继）仍限全局管理员

## 六、已知问题

本版准备期间记录的问题已全部修复并有回归测试覆盖，只剩下面两条容量类事项。均已于 2026-09-18 对照源码复验，不是回归，也不影响升级。

- **IMAP APPEND 摄取约 2 封/秒（容量）**：每个 APPEND 都走与入站投递相同的提交路径，因此大批量导入邮箱很慢（1 万封约 1.3 小时）。要把这条路径批量化属于投递引擎的改造，不是补丁级修复。
- **全文索引同步停滞缺少健康探测（运维）**：索引切到 v2 格式后正文搜索已恢复，但同步一旦停住，健康中心看不到这一状态。

另外两点来自上面的修复：

- **SORT 延迟尚未重新压测**：逐封打开 blob 的路径已去掉（排序键改读投递时缓存的头部块，早于该缓存的历史邮件回退读 blob），但大信箱端到端的数字仍需重新基准。
- **SMTP 503 改写只覆盖明文会话**：TLS 连接在这一层是密文，会原样透传，因此在 TLS 内部乱序发命令的客户端仍会看到库自带的 502。

## 七、升级说明

### 1. 常规升级

镜像部署：编辑 `deploy/mailez.env`，把 `MAILEZ_IMAGE_TAG` 指向 `v1.0.1`，然后重新拉起：

```sh
./deploy/mailezctl.sh up ce
```

控制面在启动时自动执行版本化迁移，`deploy/data/` 不受影响；升级前整体备份该目录即可。

### 2. 本次新增的配置项

以下变量可写入 `deploy/mailez.env`（后端服务通过 `env_file` 读取）：

| 变量 | 默认值 | 说明 |
| --- | --- | --- |
| `MAILEZ_OUTBOUND_PROBE_HOST` | 空 | 出站 25 端口探测的目标主机（留空则拨测 `aspmx.l.google.com`） |
| `MAILEZ_HEALTH_ALERT` | `on` | 变更式健康告警开关 |
| `MAILEZ_HEALTH_ALERT_INTERVAL_MIN` | `60` | 健康巡检间隔（分钟） |
| `MAILEZ_HEALTH_ALERT_WEBHOOK` | 空 | 告警 Webhook 地址（Slack / 钉钉 / 企业微信兼容） |
| `MAILEZ_HEALTH_ALERT_MUTE` | 空 | 静音的检查项，逗号分隔，键格式 `system:<id>` / `domain:<域名>:<id>` |
| `MAILEZ_BAN_MAX_RETRY` | `20` | 触发封禁的失败次数 |
| `MAILEZ_BAN_FINDTIME_SEC` | `600` | 失败计数窗口（秒） |
| `MAILEZ_BAN_WHITELIST` | 空 | 免封禁 CIDR，逗号分隔 |
| `MAILEZ_BACKUP_KEY` | 空 | 备份加密密钥；**未设置则不执行备份** |
| `MAILEZ_BACKUP_TARGET` | 空 | `local:<目录>` 或 `s3` |
| `MAILEZ_BACKUP_KEEP` | `14` | 备份保留份数 |
| `MAILEZ_BACKUP_HOUR` | `3` | 每日执行备份的整点 |
| `MAILEZ_MAX_ATTACHMENT_BYTES` | `20971520`（20 MB） | 服务端单个附件上限；超限返回 422 `attachment_too_large`，设 0 关闭 |

### 3. 需要留意的行为变化

- **社区版邮件端口绑定**：默认由 `127.0.0.1` 改为 `0.0.0.0`（修复新部署收不到外部邮件的问题）。若希望维持仅本机监听，显式设置 `MAILEZ_MAIL_BIND=127.0.0.1`。
- **IP 封禁默认开启**：默认 600 秒内 20 次认证失败即封禁。上线前请确认 `MAILEZ_BAN_WHITELIST` 覆盖了办公出口、监控与探针网段。
- **健康告警默认开启**：默认每小时巡检一次并推送状态变更（首轮只记录基线）。不需要时设 `MAILEZ_HEALTH_ALERT=off`。
- **备份需要密钥**：不配置 `MAILEZ_BACKUP_KEY` 时备份任务不会运行，健康中心会把备份新鲜度评为失败，请在启用前先落好密钥与目标存储。
