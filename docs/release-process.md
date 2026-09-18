# 发布流程：打 tag → 镜像 → tarball

本文是一次版本发布的固定动作清单。**核心约束只有一条：先给 mailezine 打
tag，再给 mailez 打 tag。**

## 1. 为什么必须是这个顺序

- mailez 的 release 工作流（私有树 `.github/workflows/release-private.yml`，
  公开树 `release.yml`）只构建 `ghcr.io/mailez-hq/mailez-*`；
  `ghcr.io/mailez-hq/mailez-mailezine[-ee]:<tag>` 由 **mailezine 仓库自己的
  release 工作流**产出，两个仓库靠同名 tag 锁步。
- mailez 的 smoke 作业从 ghcr 拉 `mailez-mailezine:<tag>` 起来跑
  health/seed/登录冒烟。镜像还没发布时它会重试（10 次 × 30 秒）。
- 顺序颠倒的代价不是"等一会儿"，而是**半发布**：mailez 的镜像已经推上
  ghcr，smoke 却卡在一个可能永远不会出现的引擎 tag 上，Release 资产也不会
  生成。
- v1.0.1 尤其踩这条线：本版"大信箱批量操作提速"（`UID STORE 1:* +FLAGS
  \Seen` 300 封 ~34s → ~7s）依赖引擎仓库的 `6594327 perf(mailstore): batch
  flag writes in one transaction`，只有 mailezine 的 v1.0.1 镜像里有它。

为此 mailez 的两个 release 工作流都加了 `preflight` 作业：tag 推上来后先
等引擎镜像出现（最多 10 分钟）才开始构建；等不到就直接失败，并在日志里
写明"先给 mailezine 打同名 tag"，避免半发布。

## 2. 发布前检查清单

- [ ] `CHANGELOG.md`：`[Unreleased]` 已切成 `## [vX.Y.Z] - YYYY-MM-DD`，
      并在顶部新开一个空的 `[Unreleased]`
- [ ] Release notes 就绪：`docs/release-notes-vX.Y.Z.md`（GitHub 用英文正文）
      与 `docs/release-notes-vX.Y.Z.zh-CN.md`（Gitee / OSChina 等中文渠道）
- [ ] 本次新增的环境变量已写进 `deploy/mailez.env.example`
- [ ] 涉及引擎的条目，对应提交都在 mailezine 默认分支上（逐条对照 CHANGELOG）
- [ ] 需要迁移或行为变化的说明已补进 `docs/upgrades.md`
- [ ] 两个仓库工作树干净，`make verify` / CI 绿，默认分支已同步远端

## 3. 打 tag（严格按顺序）

版本号在两个仓库必须完全一致，tag 名即镜像 tag。

```sh
# 1) 引擎仓库先打——它的 release 工作流负责产出引擎镜像
cd ../mailezine
git checkout main && git pull
git tag -a v1.0.1 -m "v1.0.1"
git push origin v1.0.1        # GitHub：触发 release.yml → ghcr 引擎镜像
git push gitee  v1.0.1        # 源码镜像，可选

# 2) 等引擎镜像出现在 ghcr（或直接进入下一步，由 preflight 兜底）
#    docker manifest inspect ghcr.io/mailez-hq/mailez-mailezine:v1.0.1

# 3) 控制面仓库
cd ../mailez
git tag -a v1.0.1 -m "v1.0.1"
git push origin v1.0.1
git push gitee  v1.0.1
```

## 4. tag 之后依次发生什么

| 作业 | 产出 / 动作 |
| --- | --- |
| `preflight` | 等待 `mailez-mailezine[-ee]:<tag>` 出现在 ghcr，超时即失败 |
| `build-images` | `ghcr.io/mailez-hq/mailez-*:<tag>`（私有树还含 `-ee` 组） |
| `smoke` / `smoke-ee` | 用刚发布的镜像起 CE / EE 栈，跑 health + seed + SSO 登录 |
| `release` | `dist/mailez-<tag>.tar.gz` + `checksums.txt` 挂到 GitHub Release |

## 5. 发布后

- 把 `docs/release-notes-vX.Y.Z.md` 的正文贴进 GitHub Release；中文版贴
  Gitee Release 与 OSChina 等渠道。
- 抽查镜像与产物：
  `docker manifest inspect ghcr.io/mailez-hq/mailez-mailezine:vX.Y.Z`、
  下载 tarball 用 `checksums.txt` 校验。
- 部署侧升级：`deploy/mailez.env` 里 `MAILEZ_IMAGE_TAG=vX.Y.Z`，然后
  `./deploy/mailezctl.sh up ce`（引擎可用 `MAILEZINE_IMAGE_TAG` 单独钉）。

## 5.1 镜像可见性：社区安装的前提

GHCR 上的 `mailez-*` 镜像必须**对匿名客户端可拉取**。包默认跟随发布它的仓库
可见性，从私有仓库发出来的包默认是私有的，私有包对组织外的人一律回 `denied`
——社区用户 `docker compose pull` 就是死在这一步（不是 tag 不存在，是没权限）。

发布工作流里有两道闸：

- `public-check`：镜像推完后用匿名 token 接口逐个验证社区镜像和当前 tag 是否
  可取。任何一个拿不到就判定发布失败，并打印要改的包名；同时会检查四个
  `-ee` 镜像没有被公开（命中只告警，不拦发布）。
- `publish-latest`：稳定 tag（名字里不带 `-`）会把 `latest` 指到本次发布，
  预发布（如 `v1.0.1-rc.1`）不动 `latest`。`latest` 是社区快速开始的默认
  tag，不推的话新 clone 的仓库会拉到不存在的镜像。

包可见性只能在 GitHub 上改，CI 无权代劳：组织 → Packages → 选包 →
Package settings → Change visibility → Public（需要公开的按上文的镜像清单，
四个 `-ee` 保持私有）。新包第一次发布一定是私有的，`public-check` 就是为了
在用户撞上之前把这件事暴露出来。

引擎镜像 `mailez-mailezine` 由引擎仓库自己的工作流发布，同一套检查在那边也有
一份。

## 6. 失败与重跑

- 只是某个作业失败：在 Actions 页面 **Re-run failed jobs**。buildx 推同一
  tag 会覆盖，重跑安全。
- 失败原因是"引擎镜像不存在"：说明 mailezine 还没打 tag（或它的工作流失败
  了）。先把引擎侧补上，再重跑 mailez 的失败作业——**不要**改 mailez 的
  tag 名来绕开，两个仓库 tag 不一致会让后续所有升级都拿错引擎镜像。
- 已经推出去的 tag 需要废弃：删远端 tag（`git push origin :refs/tags/vX.Y.Z`）
  后重新打同名 tag，并同步删除 gitee 侧；已发布的镜像 tag 会被下一次推送
  覆盖，但已经 `pull` 过的部署不会自动回退，必要时在发布说明里写明。
