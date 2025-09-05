# 发布检查清单（Release Checklist）

打任何公开 tag（社区版 CE 或私有 EE 发布）之前逐项核对。卡住的项必须
解决或显式豁免并在 release notes 说明，不允许带病发布。

## 占位符与敏感信息

- [ ] `SECURITY.md` 指向真实上报渠道（`security@mailez.net`），无
      `mailez.invalid` / `replace before release` / `TBD` 残留：
      `git grep -n -E 'mailez\.invalid|replace before|TBD|@example\.(com|org)' -- ':!backend/internal' ':!frontend/**/*_test*'`
      结果仅允许测试夹具 / 演示默认值 / RFC 保留域用法。
- [ ] 公开文档不含未标注的演示默认口令；`admin@example.com /
      MailezDemo2026!` 仅作为 seed 默认并在文档中显式提示首登修改。
- [ ] 无私有仓库路径、内部 workflow 名、内网地址出现在将被公开的文件里。

## 版本与变更

- [ ] `CHANGELOG.md` 已按 Keep a Changelog 补充，版本号符合 SemVer；
      tag 与 `CHANGELOG`/`internal/version`（引擎）一致。
- [ ] 版本发布说明包含升级注意事项（配置变更、存储迁移、破坏性变更）。

## CE 导出（mailez 与 mailezine 两仓，缺一不可）

- [ ] 两仓的公开源树导出脚本干跑通过（各自输出 “all post-conditions
      passed”，不推送）。
- [ ] 导出内容门禁无命中：EE 词汇 / 私有路径 / 中文术语残留
      （词表已含多字节关键词，见各仓导出脚本）。
- [ ] CE 导出提交信息含源 commit SHA（两仓可相互溯源对账）。
- [ ] 双仓公开 tag 成对、版本一致（引擎 ↔ 控制面依赖约定）。

## 构建与测试

- [ ] 私仓 CI 全绿：Linux（含 `-race`）/ Windows / rspamd 集成；
      私有 purity 门禁与导出校验步骤通过。
- [ ] 前端模块集为 EE 时构建通过（webmail + admin，见私有 CI 的
      frontend-ee job）。
- [ ] 协议一致性回归（conformance / `cmd/e2e`）对发布镜像跑过。
- [ ] license 声明与 `go.mod` 一致（`NOTICE`/`THIRD_PARTY_NOTICES`
      无已移除依赖，vendored 目录均带 LICENSE）。

## 发布冒烟

- [ ] 社区栈从发布镜像 boot 冒烟通过（health + seed + SSO login，
      见 release workflow 的 smoke job）。
- [ ] 扩展栈从发布 `-ee` 镜像 boot 冒烟通过（dev-license 模式，
      health + seed + SSO login，见 smoke-ee job）。

## 安全

- [ ] 本轮变更无新增的高危项（越权、注入、明文密钥路径）。
- [ ] 若有安全修复，按 SECURITY.md 的 90 天策略确认披露窗口。

## 收尾

- [ ] `README`/`README.zh-CN.md` 与实现同步（无死路径、无过期命令）。
- [ ] 联系人 / 邮件列表 / 社区渠道可用。
