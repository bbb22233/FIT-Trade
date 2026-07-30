# H2: 交易风格学习记忆与离线评估 —— 证据文件

## Fixed inputs

- Repository base commit: `8fcdb82`
- Isolated branch: `codex/ai-agent-swarm-bootstrap`
- Isolated worktree: `/Users/guanlan/Documents/FIT-Trade-worktrees/ai-agent-swarm-bootstrap`
- Hermes CLI: `Hermes Agent v0.19.0`
- Verified installed profile/model: `default` / `deepseek-v4-pro`

## Design scope verification

### 路径白名单遵守

| 文件 | 路径 | 符合白名单 |
| --- | --- | --- |
| H2_LEARNING_AND_EVALUATION.md | `coordination/design/AI-AGENT/H2_LEARNING_AND_EVALUATION.md` | ✅ |
| H2_EVIDENCE.md | `coordination/evidence/AI-AGENT/H2_EVIDENCE.md` | ✅ |

未写入任何白名单外路径。

### 禁止系统验证

| 禁止项 | 是否访问 | 证据 |
| --- | --- | --- |
| 凭据/私钥/token/cookie | 否 | 未读写任何 .env、key、secret 文件 |
| 交易所写 API | 否 | 未调用任何交易所端点 |
| 签名/signer | 否 | 未导入或使用任何签名库 |
| 实盘钱包 | 否 | 未访问任何钱包服务 |
| 生产数据库 | 否 | 未连接任何数据库 |
| production model | 否 | 仅使用默认 deepseek-v4-pro |
| services/trading-core/ | 否 | 只读上下文，未修改 |
| services/hyperliquid-adapter/ | 否 | 未读写 |
| infra/ | 否 | 未读写 |

未写入任何白名单外文件。

## Design coverage checklist

对照 SWARM_CHARTER 中 H2 的必须覆盖项：

| 要求 | 设计文档章节 | 状态 |
| --- | --- | --- |
| 审计化数据谱系 | 第3节：数据谱系（3.1-3.4） | ✅ |
| 记忆隔离 | 第4节：记忆隔离（4.1-4.3） | ✅ |
| 冻结数据窗口 | 第5节：冻结数据窗口（5.1-5.4） | ✅ |
| 离线评估 | 第6节：离线评估（6.1-6.4） | ✅ |
| 人工审核变更控制（含回退与撤销） | 第7节：人工审核变更控制（7.1-7.6） | ✅ |
| 禁止从实盘自我修改 | 第8节：自我修改禁止（8.1-8.2） | ✅ |
| 研究路径 vs 策略/交易授权区分 | 第9节：研究路径 vs 策略/交易授权（9.1-9.2） | ✅ |
| 不把回测通过写成策略/交易授权 | 第7.2、7.3 节 | ✅ |
| 追加型回退与撤销生命周期事件 | 第3.2、7.5、7.6 节 | ✅ |
| 返回标准结果包（所有授权 flag=false） | 附录A | ✅ |

## Decision register alignment

与 `docs/05_DECISION_REGISTER.md` 中相关决策的一致性检查：

| 决策 ID | 决策内容 | H2 设计中的体现 |
| --- | --- | --- |
| D-011 | 只有确认交易和明确反馈进入学习数据 | 第3.1节：学习数据入口仅限确认交易+明确反馈 |
| D-027 | 平仓后自动复盘并询问反馈 | 第3.2节：TRADE_CLOSED_REVIEW 事件 |
| D-028 | 风格复现与改进研究分离 | 第9节：研究路径 vs 策略授权完全分离 |
| D-029 | 自动交易预授权杠杆 | 第8节：Hermes 不得修改风险参数 |
| D-042 | 候选版本满足样本/影子/风险指标后用户批准 | 第7.2节：候选版本审批流程 |
| D-043 | 交易模型提出 + 独立模型审核 | 第6.3节：影子运行 + 第7节：人工审批 |

所有相关决策在设计中有明确对应的实现方案。

## Design integrity checks

### 谱系链完整性

- LineageEntry 采用 parent_lineage_id 链式结构 → 单向不可逆
- 所有学习事件必须来自 USER_DIRECTED → Hermes 不能生成学习事件
- content_hash (SHA-256) 确保内容不可篡改

### 隔离完整性

- 三层隔离：会话 / 研究 / 策略 — 层间需要独立的人工审批
- Profile 级别隔离：dev / research / trading — 完全独立的文件系统路径
- 跨层数据流是单向且需审批的

### 自我修改禁止

- 第 8.1 节列出了 6 条被明确禁止的修改路径
- 第 8.2 节提供了 3 层防护（prompt 层、API 层、Go 核心层）
- 与 D-031（安全暂停后需设备密码恢复）一致

### 授权分离

- 研究通过 ≠ 策略授权，需要 `CANDIDATE_PROMOTED` 事件作为显式过渡
- 候选版本拒绝后又重新提交必须使用新的冻结窗口
- 所有审批产出的 evidence_receipt 由 Finverse 加密证明

### 回退与撤销完整性

- `CANDIDATE_ROLLED_BACK` 和 `STRATEGY_VERSION_REVOKED` 均为追加型、不可变事件
- 回退/撤销不删除或重写历史 PROMOTED 事件——仅通过新事件标注版本禁用状态
- 每个回退/撤销事件必须包含：人工授权人身份/角色、授权时间、原因、evidence_hash、previous 和 target strategy versions
- 回退/撤销后立即禁用被回退/撤销的版本，仅可切换到明确批准的版本（或 NONE）
- 撤销到 NONE 时系统进入 reduce-only 安全模式
- Hermes 或任何自动化路径不得自行触发回退或撤销
- 研究证据与策略授权完全分离——回退/撤销不影响研究记录

## Standard worker result package

```text
TASK_ID:            t_e0b67f06
ROLE:               R-H2 — 追加型回退与撤销生命周期事件修复
STATUS:             COMPLETED
ALLOWED_PATHS:      coordination/design/AI-AGENT/H2*
                    coordination/evidence/AI-AGENT/H2*
FILES_WRITTEN:
  coordination/design/AI-AGENT/H2_LEARNING_AND_EVALUATION.md  (modified: +117 lines)
  coordination/evidence/AI-AGENT/H2_EVIDENCE.md               (modified: +10 lines)
CHECKS_RUN:
  - 路径白名单验证 ✅
  - 禁止系统访问验证 ✅
  - 事件类型完整性（+CANDIDATE_ROLLED_BACK +STRATEGY_VERSION_REVOKED） ✅
  - 人工授权字段完整性（identity/role, authorization_time, reason, evidence_hash） ✅
  - 版本切换约束（target_version 必须明确批准或 NONE） ✅
  - 历史不可变性声明 ✅
  - 自我修改禁止扩展（+回退/撤销路径） ✅
  - 研究证据/策略授权分离保持 ✅
ROUTING_PROFILE_AND_MODEL: default / deepseek-v4-pro
FINDINGS_ADDRESSED:
  - 追加 CANDIDATE_ROLLED_BACK 事件（第3.2、7.5节）：候选人版本回退到明确已批准版本
  - 追加 STRATEGY_VERSION_REVOKED 事件（第3.2、7.6节）：活跃策略版本撤销→已批准版本或 NONE
  - 每个事件结构包含：authorizer_identity, authorization_time, reason, evidence_hash, previous/target versions
  - 声明确保：立即禁用被回退/撤销版本、仅可切换到明确批准版本、历史记录不可变
  - 自我修改禁止表扩展：3 条新禁行路径（Hermes 回退、Hermes 撤销、自动路径自我回退/撤销）
  - 风险表扩展：回退到过时版本、撤销后 NONE 模式
RESIDUAL_RISKS:
  - 目标版本当前市场适用性——回退后需尽快安排独立重新评估
  - NONE 模式下所有仓位变成 reduce-only 的 UX 流程（需 H3 补充）
  - 回退/撤销的 event 触发和验证机制需在实现阶段由 Go 交易核心强制执行
INTEGRATION_DEPENDENCIES:
  - Go 交易核心：AutomationAuthorization 状态机需支持回退/撤销状态转换
  - H3（UX 设计）：回退确认 UX、撤销确认 UX、NONE 模式下的 reduce-only 提示
  - H4（安全测试计划）：回退/撤销路径的安全测试用例
  - Finverse MCP：evidence_receipt 用于回退/撤销的证据链
MERGE=false
DEPLOY=false
PRODUCTION=false
LIVE_TRADING=false
```
