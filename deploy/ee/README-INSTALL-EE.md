# mailez 企业版离线安装手册

> 版本以包内 `VERSION` 文件为准。本包面向已获交付的客户服务器
> （Linux x86_64，Docker 26+ 与 compose v2 已装）。
> 全程离线：所有镜像都在 `images.tar.gz` 内，无需访问 Docker Hub。

## 1. 服务器要求

| 项 | 要求（500 用户规模） |
|---|---|
| 系统 | Linux x86_64（Ubuntu 22.04+ / Debian 12+ / RHEL 9+） |
| CPU / 内存 | 8 核 / 16GB（最低 4C8G） |
| 磁盘 | 40GB 系统 + 1TB 数据盘（挂到安装目录） |
| 端口 | 25、80、443、465、587、143、993、110、995、4190 对外空闲；8081-8083 仅绑定本机回环、本机空闲即可 |
| 域名 | 一个邮件域名（如 mailez.cn）与主机名（如 mail.mailez.cn）的 A 记录已指向本机 |

## 2. 安装

```sh
tar xzf mailez-ee-<version>.tar.gz
cd mailez-ee-<version>
sudo bash install-ee.sh
```

按提示输入邮件域名、主机名、管理员邮箱与密码（留空自动生成强密码）。
安装器会自动：导入镜像 → 生成密钥 → 启动服务 → 等待健康 → 创建管理员。

完成后：

- 管理后台 `http://<服务器>`（网关 80 端口；8082 仅本机 `127.0.0.1:8082` 调试可达）
- Webmail `http://<服务器>`（同上网关入口；8083 仅本机调试可达）
- 数据目录 `/opt/mailez/data`（重装不删数据；卸载前自行备份）

非交互示例（自动化装机）：

```sh
sudo bash install-ee.sh --domain mailez.cn --hostname mail.mailez.cn \
  --admin-email admin@mailez.cn --admin-password '<强密码>' --yes
```

## 3. 发布 DNS 记录

登录管理后台 → 域名 → DNS 按钮：向导列出全部应发布记录并**实时核验**
是否生效（MX / SPF / DKIM / DMARC / 客户端自动配置 / MTA-STS）。
「健康体检」页面可随时查看整体状态与修复建议。

## 4. 授权（付费后）

交付的 `license.lic`（邮箱数授权）与 `service.lic`（年技术服务）放到
本服务器任意路径后：

```sh
sudo bash install-ee.sh activate /path/license.lic /path/service.lic
```

安装器会安装证书、打开强制校验并重启服务。到管理后台「概览」确认
授权方与邮箱上限；超出上限的建号会被拒绝。

## 5. 生产 TLS（上线前必读）

默认关闭 TLS 便于验收。正式启用请按 `deploy/certs` 流程签发证书后在
`/opt/mailez/mailez.env` 中设置：

```
MAILEZINE_TLS_CERT_FILE=/certs/letsencrypt/live/mailez/nginx-chain.pem
MAILEZINE_TLS_KEY_FILE=/certs/letsencrypt/live/mailez/privkey.pem
```

再执行 `docker compose --env-file mailez.env -p mailez -f ee/docker-compose.ee.yml up -d`。

## 6. 常用运维

```sh
cd /opt/mailez
docker compose --env-file mailez.env -p mailez -f ee/docker-compose.ee.yml ps      # 状态
docker compose --env-file mailez.env -p mailez -f ee/docker-compose.ee.yml logs -f # 日志
docker compose --env-file mailez.env -p mailez -f ee/docker-compose.ee.yml down    # 停止（数据保留）
```

故障排查：25 端口出站被云厂商封禁（发信失败）需向厂商申请解封；
其余见管理后台「健康体检」页。
