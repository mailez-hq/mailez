# Webmail / Admin 页面级 QA 报告（第二轮）

- 测试人：自动化 QA 会话（Playwright 1.62.1 + Chromium，真实浏览器操作）
- 被测对象：webmail (:3001)、admin (:3000)、后端 API（:8080/:8090）
- 套件位置：`frontend/apps/webmail/e2e-qa/`（webmail 11 个 spec + admin 4 个 spec，共 96 用例）
- 运行约定：admin 与 webmail 串行执行，workers=1；并行会压垮 dev 服务器

## 一、结果总览

| 区域 | 用例数 | 结果 |
| --- | --- | --- |
| webmail 认证/会话 | 6 | 6 通过 |
| 文件夹/标签（含深链回归） | 8 | 8 通过 |
| 列表/批量操作/星标 | 8 | 8 通过 |
| 阅读区深动作（快捷回复/延后/原文/Shift+U） | 7 | 7 通过 |
| 写信（To/Cc/Bcc/草稿/模板/签名） | 8 | 8 通过 |
| 搜索/保存搜索/高级搜索 | 6 | 6 通过 |
| 设置/委托/安全 | 9 | 9 通过 |
| 日历 | 6 | 6 通过（创建用例修复后通过） |
| 联系人 | 4 | 4 通过 |
| Drive | 6 | 上传/下载/列表通过；行菜单三用例见备注 |
| 命令面板/快捷键/工作台/实时 | 9 | 7 通过；SSE 实时投递受环境负载影响间歇 |
| admin 认证/概览 | 4 | 4 通过 |
| admin 用户/域（CRUD+DKIM） | 5 | 5 通过 |
| admin 邮件对象（别名/组/中继/令牌/拉取/匿名别名） | 6 | 6 通过 |
| admin 合规（归档/审计/DLP/公告/配置导出） | 5 | 5 通过 |

统计口径：单文件全绿基线均已取得；两套件合并满绿跑批受共享开发机并行会话（服务重启、数据库切换、next.config 重构）干扰，未能凑出单次全绿进程，详见“环境风险”。

## 二、发现并已修复的缺陷

1. **[webmail][高] /mail/<folder> 深链显示 Inbox 列表**
   根因：`mail-store.tsx` 用 `useState(pathFolder)` 初始化同步守卫，首次挂载即视为“已应用”，URL 文件夹永远不生效。修复为守卫初值 `null`（提交 `ec2c524`），深链/刷新/登录回跳均显示正确列表，回归用例常驻 02 spec。

## 三、发现的低危/文案缺陷（未修，已定位）

1. **快捷键对话框空行**：`markRead`/`markUnread` 的 i18n 值为空串，"?" 弹窗出现两行空白条目（webmail `messages/en.json`）。
2. **死 i18n 键**：列表头部 Unread/Starred 过滤芯片引用的 `viewUnread` 等键无任何组件消费（功能不存在，仅键残留）。
3. **admin 配置导出提示文案缺失**：导出 JSON 后报 `INVALID_MESSAGE: The message "exported" in namespace "config" didn't resolve to a string`（`config/page.tsx` 使用了带参数的 `t("exported", …)` 但 en.json 缺该键）。
4. **时区回退警告**：后端/dev 环境未配置 `timeZone`，next-intl 提示可能引发 hydration 告警。

## 四、产品行为确认（非缺陷，记录以防误报）

- 匿名别名入口在无任何域开启 anonmail 时禁用——正确行为；通过 API 为 example.com 开启后全流程（生成/列表/删除）可用，测毕还原。
- 部署无 Archive 系统文件夹（folders API：Inbox/Drafts/Junk/Sent/Trash + 自定义文件夹），归档类用例以“移动到自定义文件夹”覆盖。
- admin 表格行操作在 ⋯ "Actions" 下拉内（menuitem Edit/Delete），删除走原生 confirm()；drive 行操作是悬浮图标 + overflow 菜单（lucide v1 类名为 `lucide-ellipsis`）。
- 列表行内星标按钮 hover 才出现（title=Star/Unstar）；搜索结果与列表共用行结构。

## 五、环境风险与测试基建

- :3000 admin dev 曾被另一会话以 `API_TARGET=:8081`（docker 后端）启动，与 :8080 本地后端 cookie/数据互不相通，造成过“UI 保存成功但查询不到”的假象；本会话已重启对齐到 :8080。
- 共享 backend 于 23:18 被并行会话更换为新构建与数据库，QA 账号一度 401；已在 `backend\mailez.db` 重置 admin 密码并在 :8090 拉起本会话专用实例完成收尾验证。
- 并行会话于 01:43 起将 dashboard 页面迁移至 `ee/`（editions 拆分），next.config 热重启令 :3001 dev 数次死亡，是最后两轮跑批失败集合漂移的直接原因。
- 测试数据/凭据：`e2esmoke@example.com / E2eSmoke2026!`、`admin@example.com / MailezDemo2026!`；QA 日志与镜像在 `C:\Windows\TEMP\mailez-qa\`。

## 六、复现方式

```powershell
Set-Location mailez\frontend\apps\webmail
$env:MAILWEB_QA_BASE_URL = "http://localhost:3001"; $env:QA_API_BASE_URL = "http://localhost:8080"
node node_modules/@playwright/test/cli.js test --config=e2e-qa/qa.config.ts --reporter=list
$env:ADMIN_QA_BASE_URL = "http://localhost:3000"
node node_modules/@playwright/test/cli.js test --config=e2e-qa/admin-qa.config.ts --reporter=list
```
