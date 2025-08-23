# 同类引擎评估

> 状态：**评估完成**（2026-08-23）· 结论：同类引擎定位为「第二引擎 / 参考实现」，
> 不作为 mailez 的默认邮件引擎。

## 1. 背景

mailez 的数据平面当前是经典容器栈（nginx / legacy MTA / legacy IMAP / rspamd /
unbound），控制平面（用户/域/API/筛选/AI）是 Go 后端。用户考虑"全 Go"路线时，
评估同类引擎（`D:\code\peer engine`，完整 Go 邮件服务器）是否可以作为替代引擎。

## 2. peer engine 事实

- **是什么**：一个完整的 Go 邮件服务器（不是代理），SMTP 收/发/投递、IMAP4、
  webmail、SPF/DKIM/DMARC、内置声誉跟踪 + 贝叶斯反垃圾、ACME 自动证书、
  DANE/MTA-STS、国际化地址（EAI）都在一个二进制里。
- **版本**：v0.0.17（2026-08-19 最近提交，作者活跃）；模块 `github.com/mjl-/peer engine`。
- **维护**：单维护者（mjl-），bus factor = 1；0.x 版本，API/存储格式未冻结。
- **许可**：MIT 或 MPL-2.0 双许可（用户任选）。
- **定位**：低维护的自托管个人/小规模邮件。README 明确不计划支持：POP3、
  可插拔投递机制、投递到 OS 系统用户。

## 3. 与 mailez 需求的关键差距

| 需求 | 经典栈（现状） | peer engine |
|---|---|---|
| Sieve 过滤 + ManageSieve（4190） | legacy IMAP pigeonhole ✓ | ✗ 过滤用自研 Rulesets，Sieve 在计划中 |
| IMAP Sieve 扩展 | 支持 | 计划中 |
| POP3（110/995） | ✓ | ✗ 明确不支持 |
| Milter 接口（挂 rspamd） | ✓ | 计划中 |
| 多租户 / 虚拟域管理 | 成熟（完整的域 / 别名 / 管理员模型） | 面向个人域，管理模型不同 |
| 反垃圾生态 | rspamd 社区规则集 + 贝叶斯 | 内置贝叶斯 + 声誉（个人级） |
| 存储格式 | maildir（标准、可迁移） | 自定义格式，迁入即锁定 |
| 维护风险 | 多项目 / 社区久经考验 | 单维护者 + 0.x |

## 4. 为什么不做默认引擎

1. **Sieve 缺失 = 产品功能倒退**：mailez webmail 已有筛选编辑器，后端已实现
   ManageSieve REST（`/sieve*`），切到 peer engine 这块直接归零，只能退回 peer engine 自研
   Rulesets，且无 4190 端口协议。
2. **POP3 缺失**：现有端口/客户端兼容性断裂（110/995）。
3. **Milter 缺失**：无法挂 rspamd；而 peer engine 内置反垃圾面向单用户，达不到
   mailez 的多租户服务形态要求。
4. **存储格式私有**：邮件一旦进入 peer engine 存储，迁移回 maildir/标准格式成本高。
5. **单维护者 + 0.x**：作为产品核心依赖的风险过高。

## 5. peer engine 的价值

- **活教材**：`store` / `dkim` / `dmarc` / `smtpserver` / `imapserver` 都是
  独立高质量 Go 包，是 Go 邮件实现的参考标杆。
- **第二引擎候选**：若 peer engine 落地 Sieve（过滤脚本）+ Milter + 1.0，可作为
  `mailez-light`（单二进制发行版，面向个人/单域）的引擎。
- **架构参照**：为"控制平面 + 可插拔引擎"的中期抽象提供第二个实现样例。

## 6. 建议

- **主线不变**：经典容器栈 + Go 控制平面，继续自研化（本地镜像、
  Go 重写配置代理）。
- **中期**：把数据平面边界抽象为接口（认证、邮箱路径/配额、筛选、反垃圾
  报告、DKIM），让经典栈与 peer engine 都能挂载。
- **观察信号**：peer engine 的 Sieve、Milter、1.0 三个里程碑；每季度小 PoC 验证
  （见下）。2026 年内不切换默认引擎。

## 7. PoC 计划（若要验证）

- 最小环境：单域、约 10 用户、每日约 100 封真实流量。
- 验证项：SMTP 入站/出站、IMAP 客户端（搜索/线程/同步）、Rulesets 过滤、
  声誉垃圾学习、ACME 证书续期。
- 记录：存储占用、CPU/内存、`maildir → peer engine store` 迁移工具是否可行、与
  现有 e2e 的差距。
