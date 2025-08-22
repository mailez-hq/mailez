# mailez Webmail UI 设计规格

> 目标读者：mailez 前端开发者（Trae / Codex）。本文档是 webmail 界面重构的设计基准，实现时以本文为准，与 `branding/` 品牌体系保持一致。
> 相关代码现状：`frontend/apps/webmail/`（Next.js 16 + React 19 + Tailwind v4 + shadcn/ui + TipTap + next-intl）。

## 1. 设计理念

**FastMail 的皮 + mainstream webmail 的脑 + SnappyMail 的手感。**

- **皮（视觉）**：冷静克制、信息密度高但不乱、阅读区永远不抢邮件内容的风头。取 FastMail。
- **脑（智能）**：会话线程、智能分类、强大搜索、AI 摘要/写作。取 mainstream webmail，但做成透明、可开关、可自托管。
- **手（交互）**：秒开、桌面客户端手感、快捷键、右键/拖拽、离线状态。取 SnappyMail。

**差异化（别人抄不走的三张牌）：**

1. **只此青绿品牌色**：邮件客户端几乎没人用青绿。克制使用，形成一眼辨识。
2. **AI 原生 + 透明**：AI 摘要/分类/写作深度集成，但每条 AI 输出都带标识、可开关、隐私默认优先。
3. **一体化的自托管体验**：webmail 与 admin 共用设计语言与登录态；写邮件时直接看到发件身份与 DKIM 健康度。

## 2. 设计 Token

现有 `app/globals.css` 是 shadcn 默认 neutral 模板。**中性色从冷灰 zinc 换成暖灰 stone**（呼应 logo 的纸张色 #FCF9EF），主色取品牌青绿。

### 2.1 双色系统

| Token | 色值 | 用途 |
|---|---|---|
| `--brand` 青绿 | `#2F8E6C`（亮）/ `#3BA77F`（暗） | 主操作、选中态、链接、未读点 |
| `--brand-deep` | `#175A44` | hover/active、品牌文字 |
| `--brand-tint` | `#E5F2EC`（亮）/ `#1F3D33`（暗） | 选中行背景、徽章底 |
| `--ai` 石青 | `#2E6E8E`（亮）/ `#4D8FAC`（暗） | **AI 专属色**：AI 摘要卡、AI 草稿、智能分类 |

> 设计决策：**青绿管"人操作"，石青管"机器操作"**。用户一眼能区分哪些是 AI 产物。这是 mailez 的特色识别。

### 2.2 亮色主题（替换 globals.css `:root`）

| 变量 | 值 | | 变量 | 值 |
|---|---|---|---|---|
| `--background` | `#FAFAF7` | | `--primary` | `#2F8E6C` |
| `--foreground` | `#1C1917` | | `--primary-foreground` | `#FFFFFF` |
| `--card` / `--popover` | `#FFFFFF` | | `--secondary` | `#F2F0EA` |
| `--muted` | `#F2F0EA` | | `--secondary-foreground` | `#1C1917` |
| `--muted-foreground` | `#79716B` | | `--accent` | `#E5F2EC` |
| `--border` / `--input` | `#E7E4DD` | | `--accent-foreground` | `#175A44` |
| `--ring` | `#2F8E6C` | | `--destructive` | `#C94B4B` |
| `--sidebar` | `#F4F2EC` | | `--sidebar-accent` | `#E5F2EC` |
| `--sidebar-foreground` | `#44403C` | | `--sidebar-border` | `#E7E4DD` |
| `--ai` | `#2E6E8E` | | `--ai-foreground` | `#FFFFFF` |

### 2.3 暗色主题（替换 `:root.dark`）

| 变量 | 值 | | 变量 | 值 |
|---|---|---|---|---|
| `--background` | `#141613` | | `--primary` | `#3BA77F` |
| `--foreground` | `#EDEAE4` | | `--primary-foreground` | `#0B231B` |
| `--card` / `--popover` | `#1C1E1B` | | `--secondary` | `#262823` |
| `--muted` | `#262823` | | `--secondary-foreground` | `#EDEAE4` |
| `--muted-foreground` | `#A3A09A` | | `--accent` | `#1F3D33` |
| `--border` | `rgba(255,255,255,0.08)` | | `--accent-foreground` | `#8FD9BE` |
| `--input` | `rgba(255,255,255,0.12)` | | `--destructive` | `#E56B6B` |
| `--ring` | `#3BA77F` | | `--sidebar` | `#181A17` |
| `--sidebar-accent` | `#1F3D33` | | `--sidebar-foreground` | `#A3A09A` |
| `--ai` | `#4D8FAC` | | `--ai-foreground` | `#0E1D26` |

### 2.4 字体与字号

- 字体：沿用 Geist Sans / Geist Mono（`layout.tsx` 已引入）。
- UI 字号基线 14px；邮件正文 15px / 行高 1.7；列表行 13–14px。
- 阅读区正文居中限宽：`max-w-3xl`（约 72ch）。

### 2.5 密度三档

| 档位 | 列表行高 | padding | 适用 |
|---|---|---|---|
| compact | 36px | py-1 | 重度用户、宽屏 |
| cozy（默认） | 44px | py-2 | 大多数用户 |
| relaxed | 52px+ | py-2.5 | 触屏、低视力 |

密度设置存 localStorage（或用户偏好接口），与主题一起放 `MailSettings`。

## 3. 布局规格

### 3.1 三栏结构

```
┌─────────────┬──────────────────────┬─────────────────────────────┐
│ Sidebar     │ MessageList          │ ReadingPane                │
│ 240px       │ 320–420px（可拖宽）   │ 自适应                     │
│ ├ brand     │ ├ SearchBar (/ )     │ ├ MessageHeader            │
│ ├ Compose   │ ├ Toolbar(选中时)    │ │  ├ 头像/发件人/时间       │
│ ├ 文件夹    │ ├ MessageList(虚拟)  │ │  └ 操作: 回复/全部回复/转发│
│ │  ├ 收件箱 │ │  └ 行: 未读点/发件人│ ├ AiSummaryCard(石青)      │
│ │  ├ 已发送 │ │     主题/预览/时间  │ ├ MessageBody              │
│ │  ├ 草稿   │ └ LoadMore/无限滚动  │ │  └ 引用折叠/远程图片策略  │
│ │  └ 已删除 │                      │ └ AttachmentList(卡片+预览) │
│ └ 身份/配额 │                      │                             │
└─────────────┴──────────────────────┴─────────────────────────────┘
```

### 3.2 断点行为

| 视口 | 布局 |
|---|---|
| ≥1280px | 三栏并排 |
| 900–1279px | 两栏（列表 + 阅读），文件夹收成抽屉（汉堡/图标 rail 56px） |
| <900px | 单栏：列表 ⇄ 阅读切换，文件夹抽屉 |

### 3.3 组件树（建议拆分目标）

现 `MailView`（`components/mail-view.tsx`）是一个近 500 行的大组件，重构时拆成：

```
MailShell
├── FolderNav        ← 现 mail-view 中 aside 部分
├── MessageListPanel
│   ├── SearchBar    ← 现顶部 form；支持 / 聚焦
│   ├── ListToolbar  ← 现选中后工具条（删除/已读/未读 + 后续 归档/移动/垃圾）
│   └── MessageList  ← 虚拟滚动行组件
├── ReadingPane
│   ├── MessageHeader
│   ├── AiSummaryCard   （新建，石青）
│   ├── MessageBody     ← 现 article；加引用折叠/远程图片策略
│   └── AttachmentList  （新建，卡片化 + 内嵌预览）
├── Composer         ← 现 Dialog；加发件身份选择 + AI 草稿
├── CommandPalette   （新建，⌘K）
├── ShortcutsDialog  （新建，?）
└── Settings / Contacts 对话框
```

## 4. 交互细节

### 4.1 邮件列表行

- 未读：发件人/主题加粗 + 6px 青绿圆点（**只有这一个地方常驻用青绿**）。
- 选中：`brand-tint` 背景 + 左侧 3px 青绿条。
- hover：仅浅色背景变化，行内右侧浮现操作（删除/归档/星标），不遮挡时间。
- 时间：今天显示 HH:mm，今年显示 M月d日，更早显示 yyyy/M/d。
- 键盘光标行：与 hover 同款样式 + `outline`。

### 4.2 阅读区

- 引用折叠：正文中的 `>` 引用块默认折叠为一行"引用 N 行，点击展开"。
- 远程图片：默认不加载，顶部显示"此邮件含远程图片"横幅，按钮"加载图片"（一次/总是，总是则记住发件人）。
- HTML 正文渲染保持现有 sanitize 思路；附件改为卡片：文件类型图标 + 文件名 + 大小 + 下载；图片/PDF 点击内嵌预览。

### 4.3 动效

- 只做 150–200ms 的 opacity/transform 微动效（行 hover、选中、弹窗）。
- 不做页面级转场动画；列表更新用乐观更新，失败回滚 + toast。

## 5. 键盘快捷键（完整表）

现有实现：`j/k` 移动、`Enter` 打开、`r` 回复、`#` 删除。扩展为：

| 按键 | 动作 | | 按键 | 动作 |
|---|---|---|---|---|
| `j` / `k` | 下一条 / 上一条 | | `r` | 回复 |
| `Enter` / `o` | 打开 | | `a` | 全部回复 |
| `x` | 多选 | | `f` | 转发 |
| `e` | 归档（P1） | | `#` | 删除 |
| `!` | 标记垃圾（P1） | | `s` | 星标/旗标 |
| `Shift+I` | 标为已读 | | `Shift+U` | 标为未读 |
| `/` | 聚焦搜索 | | `Esc` | 清搜索/关弹窗/回列表 |
| `?` | 快捷键面板 | | `⌘K` | 命令面板 |
| `n` | 新邮件 | | `u` | 回到列表 |
| `g i` | 收件箱 | | `g s` | 已发送 |

> 实现要求：输入框/编辑器聚焦时全部忽略；快捷键面板用 `?` 打开且可搜索。

## 6. AI 特色设计（差异化核心）

现有 `aiStatus/aiSummarize/aiDraft` 接口已通。UI 层规则：

1. **AI 摘要卡**：阅读区顶部，石青底色徽章"AI"；内容可折叠；底部"有帮助/不准确"反馈 + "重新生成"。摘要基于当前邮件，明确标注"AI 生成，仅供参考"。
2. **AI 草稿**：写邮件对话框内，"AI 起草"支持语气选择（正式/简洁/友好）；生成后可"重新生成"。
3. **智能分类（P2）**：文件夹列表顶部"优先"入口，由 AI 打分排序，可关。
4. **语义搜索（P2）**：命令面板里输入自然语言（"上个月的合同附件"），后端语义检索。
5. **透明性**：设置页提供 AI 开关总控 + 每个功能独立开关；注明"默认本地处理，可配置外部模型"。

## 7. 一体化特色

- **发件身份选择**：Composer 顶部身份下拉，列出主地址 + 别名 + 托管域名；每个身份右侧显示 DKIM 状态点（绿=健康，黄=未配置，灰=未知），hover 显示说明。数据来自 admin 现有 DKIM API。
- **登录态打通**：webmail 与 admin 共用会话 cookie；Sidebar 底部提供"管理控制台"入口。
- **设计语言统一**：两 app 的 globals.css token 保持一致（本文第 2 节），修改时两边同步。

## 8. 任务清单

### P0（立住"好看"）

- [x] `globals.css` 应用品牌 token（第 2 节全量替换，亮/暗两套）
- [x] `MailView` 拆分为第 3.3 节组件树
- [x] 三栏响应式断点（3.2）+ 列表列宽可拖拽（+/-）
- [x] 密度三档切换，持久化
- [x] 深浅主题（跟随系统 + 手动覆盖），持久化
- [x] 邮件列表虚拟滚动（自研固定行高虚拟列表，替换 `ScrollArea` 内 map；环境无 npm 故未用 `@tanstack/react-virtual`）
- [x] 快捷键扩展 + `?` 面板（第 5 节）
- [x] `/` 聚焦搜索、`Esc` 清除
- [x] 列表行视觉统一（4.1）：未读点、选中条、hover 操作
- [x] 阅读区排版：限宽、15px/1.7、引用折叠
- [x] 附件卡片化 + 图片内嵌预览
- [x] 远程图片策略横幅
- [x] 空状态/加载骨架屏
- [x] **i18n 补全**：消灭硬编码英文（`AI Summary`、`Drafting...`、`Clear search`、`Select`、`Delete (#)` 等已全部进 `messages/en|zh.json`）

**P0 验收标准**：`next build` 通过；亮/暗主题和密度重启后保持；10,000 条样本列表滚动不掉帧；zh locale 下无英文硬编码；快捷键面板可查可搜。

### P1（好用）

- [x] 归档/移动文件夹 + 删除撤销 toast（后端 `POST /mail/move` 支持批量 UID MOVE，目标文件夹自动创建；删除=移入 Trash，撤销即移回）
- [x] 命令面板 `⌘K`（打开文件夹/搜索/写邮件/设置）
- [x] 发件身份选择 + DKIM 状态点（后端 `GET /mail/identities`，send 支持 From 且校验属于本人/别名）
- [x] 垃圾邮件举报 `!` 与列表批量移动（优先复用已存在的 Spam/Junk 文件夹）
- [x] 连接状态 / 断线重连提示
- [x] 会话线程视图（后端按规范化主题聚合最近 300 封，`thread_id/thread_count` 注入列表，`GET /mail/thread` 返回会话内全部邮件；阅读区会话切换条）
- [x] 高级搜索语法：`from:` / `to:` / `subject:` / `has:attachment` / `before:` / `after:`（含引号值），输入框语法提示 chips
- [x] AI 摘要卡打磨（折叠、有帮助/没帮助反馈、重新生成）
- [x] PWA + Service Worker 离线基础（manifest + SW：静态资源 cache-first、导航 network-first 回退 app shell、API 永不缓存）

### P2（有特色）

- [ ] AI 优先收件箱（智能排序/分类标签）
- [ ] 语义搜索
- [ ] 联系人视图：按发件人聚合往来、头像、联系人卡片
- [ ] Sieve 规则管理界面
- [ ] 内置 PGP（加密/签名）
- [ ] 日历/待办（可选扩展）

## 9. 性能预算（技术约束）

- 邮箱主界面首屏 JS < 200KB gzip；阅读窗格按需加载。
- 列表滚动 ≥60fps；键盘响应 < 50ms。
- 数据层用 React Query/SWR 做 IMAP 缓存 + 乐观更新；错误 toast 可撤销。
- 组件库沿用 shadcn 风格自建，不引入重型 UI 框架。

## 10. 反目标（不要做）

- 不做 mainstream webmail 式拥挤工具栏（图标全堆一排）。
- 不做满屏渐变/彩色装饰——青绿只出现在第 2.1 节列出的位置。
- 不做大转场动画、不做 3D/拟物。
- 不牺牲键盘工作流换视觉效果。
- 不在无把握时改动后端契约；本文涉及的接口若不存在，先与后端确认再开发。
