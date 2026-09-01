# 升级与版本间迁移

本文回答三类问题:同版本升级怎么做、社区版内控制面切换(MySQL ↔ SQLite)
怎么做、社区版 → 企业版怎么做。**诚实边界写在前面**:跨 KV 后端
(Pebble → TiDB)的邮件数据目前**没有**原地迁移工具;`mailezine migrate`
只覆盖传统栈(Maildir)→ KV。替代路径见下文。

## 1. 同版本升级(拉取新代码)

```sh
./deploy/mailezctl.sh down
git pull          # mailez 与 mailezine 两个仓库都要拉
./deploy/mailezctl.sh up community           # 或 enterprise
```

`mailezctl up` 自带 `--build`,会以正确的 edition 构建参数重建
backend/前端/引擎镜像,是镜像更新的统一入口。注意 `go run
./cmd/build-images` 只覆盖 nginx/rspamd/unbound/macro-scanner 与
**CE 档**引擎(不传 `MAILEZ_EDITION`),不构建 backend 与前端——
企业栈不要用它,否则会拿回 CE 引擎镜像(见 `docs/rollout-runbook.md`
踩坑清单)。

- 控制面:启动时自动执行版本化迁移(`schema_migrations` 表),无需人工干预。
- 邮件数据:`deploy/data/` 不受升级影响;升级前整体备份该目录即可
  (引擎快照见引擎仓库文档)。
- 配置:`deploy/mailez.env` 是部署者自有文件,升级不会覆盖;新增配置项
  以 `mailez.env.example` 与 CHANGELOG 为准。

## 2. 社区版内切换控制面(MySQL → SQLite / 反向)

控制面数据库**不支持原地切换**(GORM 迁移只建 schema 不搬数据)。
标准流程是"配置导出 → 新栈导入",邮件数据目录原样保留:

1. **导出管理配置**:管理控制台 → 配置 → 导出(或
   `GET /api/v1/config/export`,需要管理员会话)。包含:域名、域名别名、
   用户(含密码哈希)、地址别名、中继、外部账号拉取、应用令牌。
   **不包含**用户级数据:联系人、日历、云盘文件、标签、委派关系——
   这些可用 CardDAV/CalDAV/客户端 IMAP 同步自行搬运,或接受重配。
2. **停旧栈,保留数据目录**:
   `./deploy/mailezctl.sh down`(不要删除 `deploy/data/`)。
   建议先完整备份 `deploy/data/`。
3. **起新栈**(默认即 SQLite;反向则设置
   `MAILEZ_DB_DRIVER=mysql` + `MAILEZ_DB_DSN=…` 并加 `--profile mysql`)。
4. **导入配置**:新栈管理控制台 → 配置 → 导入(幂等,可重试)。
5. **验证**:用原账号密码登录(密码哈希随导出迁移),确认邮箱列表、
   别名、拉取配置完整;邮件目录按邮箱地址寻址,控制面出现相同邮箱集
   后邮件随之可见。

> 建议先在数据副本上演练一次,确认账号集与数据目录匹配再切换生产。

## 3. 社区版 → 企业版(CE → EE)

差别有两层,分开处理:

**控制面与授权**:企业栈要求 `deploy/licenses/license.lic`
(`MAILEZ_LICENSE_REQUIRED=true`,自签/采购见 README)。控制面同样走
第 2 节的导出/导入(MySQL → MySQL,无切换问题)。

自签演练的授权签发与全流程实跑(含切换顺序与验证清单)见
`docs/rollout-runbook.md` 阶段二;最短签发命令:

```sh
cd backend
go run ./cmd/license issue --out ../deploy/licenses/license.lic \
  --licensee <名字> --mailboxes 100
go run ./cmd/license inspect -in ../deploy/licenses/license.lic
```

**邮件数据(Pebble/本地 FS → TiDB/MinIO)**:目前**没有** KV→KV 迁移
工具。两条可行路径:

- **IMAP 聚合搬运(推荐,当前可用)**:旧社区栈保持在线,在新企业栈为
  每个账号配置外部账号拉取(设置 → 外部账号,IMAP 指向旧栈),历史邮件
  逐账号拉入新栈;完成后切换 DNS/MX,保留旧栈一个完整邮件周期后下线。
  注意:投递元数据(部分旗标/关键词)在 IMAP 搬运中保真,UID 会重排,
  客户端首次同步会重新拉取。
- **等待官方工具**:KV→KV 迁移在路线图上;订阅 release note。

**回退**:切换前保留旧栈数据目录与配置导出;企业版许可证独立于数据,
回退社区版=停企业栈、起社区栈、导入同一份配置导出。

## 4. 传统栈(Postfix+Dovecot)→ 任意版本

`mailezine migrate`(引擎仓库)支持 Maildir → Pebble/TiDB 单向全量复制,
详见引擎仓库 `cmd/mailezine migrate` 文档。注意它是**全量**复制:
缓冲期内旧栈新到的邮件不会被增量补齐,需确认无增量后再下线旧栈。
