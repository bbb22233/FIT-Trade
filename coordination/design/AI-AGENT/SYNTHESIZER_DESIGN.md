# Synthesizer: Coordinator-Ready AI-Agent Design Result Package

## 状态与边界

- 阶段：设计汇总 + R2 修复汇聚 — 不实现、不部署、不交易
- 工作树：`/Users/guanlan/Documents/FIT-Trade-worktrees/ai-agent-swarm-bootstrap`
- 分支：`codex/ai-agent-swarm-bootstrap`
- 固定 commit：`8fcdb82db970a1e2925b53b61c4245f9b597a2ef`
- 允许写入：仅 `coordination/design/AI-AGENT/SYNTHESIZER*` 与 `coordination/evidence/AI-AGENT/SYNTHESIZER*`
- 禁止：凭据、私钥、token、cookie、signer、交易所写 API、订单、实盘钱包、生产数据库、生产模型、`services/trading-core/`、`services/hyperliquid-adapter/`、`core` auth/trading code
- 本包仅合成 Verifier 已通过的 H1-H4 设计产物及 R2 修复产出；不发明证据、不覆盖 Verifier 判断

**门状态分离：**
- `DESIGN_REVIEW_PASS = true` — 文档/设计审查通过
- `SECURITY_EXECUTION_EVIDENCE_NOT_RUN = true` — 所有安全/P2/运行时测试尚未执行

---

## 1. 任务清单

### 1.1 原始设计任务

| 任务 ID | 角色 | 状态 | 产出文件数 |
| --- | --- | --- | --- |
| `t_8aef1237` | H1 — 多模型与 API Adapter 设计 | 完成（设计阶段） | 2 |
| `t_0252c26e` | H2 — 交易风格学习记忆与离线评估设计 | 完成（设计阶段） | 2 |
| `t_6dcdf7d3` | H3 — 内置聊天与开仓/加仓确认 UX 设计 | 完成（设计阶段） | 2 |
| `t_0cb0931e` | H4 — AI 安全对抗测试计划 | 完成（设计阶段） | 2 |
| `t_3de8d1f2` | Verifier — 独立 AI-agent 设计安全与证据门 | PASS (设计审查) | 1 |

### 1.2 R2 修复任务

| 任务 ID | 角色 | 状态 | 修复内容 |
| --- | --- | --- | --- |
| `t_6dc220db` | R-H1 — 模型能力与失败分类修复 | 完成 | H1 §2/§5/§6 重构 |
| `t_e0b67f06` | R-H2 — 追加型回退与撤销事件修复 | 完成 | H2 §7.5/§7.6 新增 |
| `t_64632ac8` | R-H3 — 原子确认消费修复 | 完成 | H3 §6 新增 |
| `t_ff9c629d` | R-H4 — MCP 工具输出注入测试设计修复 | 完成 | H4 §7 新增 |
| `t_7f24497c` | R2-Synthesizer — 修复汇聚与固定 commit 诚实性 | 进行中 | 本文件 + 3 个证据文件 |

### 1.3 依赖图

```
原始流：
H1 ─┐
H2 ─┼─► Verifier ─► Synthesizer ─► 协调方 (Codex)
H3 ─┤
H4 ─┘

R2 修复流：
R-H1 ─┐
R-H2 ─┼─► R2-Verifier-Update ─► R2-Synthesizer ─► 协调方 (Codex)
R-H3 ─┤
R-H4 ─┘
```

---

## 2. 产物清单

### 2.1 设计文档（含 R2 修复）

| 文件 | 路径 | 状态 | R2 修复 |
| --- | --- | --- | --- |
| H1 多模型适配器设计 | `coordination/design/AI-AGENT/H1_MULTI_MODEL_ADAPTER_DESIGN.md` | ✅ R-H1 已修复 | §2 VERIFIED/UNVERIFIED 矩阵; §5 错误分类; §6 启动发现 |
| H2 学习记忆与评估设计 | `coordination/design/AI-AGENT/H2_LEARNING_AND_EVALUATION.md` | ✅ R-H2 已修复 | §7.5 CANDIDATE_ROLLED_BACK; §7.6 STRATEGY_VERSION_REVOKED |
| H3 聊天与确认 UX 设计 | `coordination/design/AI-AGENT/H3_BUILT_IN_CHAT_AND_CONFIRMATION_UX.md` | ✅ R-H3 已修复 | §6 原子 compare-and-consume 语义 |
| H4 安全测试计划 | `coordination/design/AI-AGENT/H4_SECURITY_TEST_PLAN.md` | ✅ R-H4 已修复 | §7 6 个 MCP 注入用例 |

### 2.2 证据文档（含 R2 修复）

| 文件 | 路径 | 状态 | R2 修复 |
| --- | --- | --- | --- |
| H1 设计证据 | `coordination/evidence/AI-AGENT/H1_DESIGN_EVIDENCE.md` | ✅ R-H1 已修复 | 覆盖验证、接口依赖、风险同步 |
| H2 证据 | `coordination/evidence/AI-AGENT/H2_EVIDENCE.md` | ✅ R-H2 已修复 | 基线更新、coverage/integrity 检查 |
| H3 证据 | `coordination/evidence/AI-AGENT/H3_EVIDENCE.md` | ✅ R-H3 已修复 | 覆盖、接口依赖、风险、基线提交 |
| H4 安全证据 | `coordination/evidence/AI-AGENT/H4_SECURITY_EVIDENCE.md` | ✅ R-H4 已修复 | 新增 6 用例证据模板 |
| Verifier 报告 | `coordination/evidence/AI-AGENT/VERIFIER_REPORT.md` | ✅ R2 更新 | gate 分离、固定 commit 诚实性 |
| Synthesizer 设计 | `coordination/design/AI-AGENT/SYNTHESIZER_DESIGN.md` | ✅ R2 更新 | 本文件 |
| Synthesizer 证据 | `coordination/evidence/AI-AGENT/SYNTHESIZER_EVIDENCE.md` | ✅ R2 更新 | R2 修复证据 |
| CLEANUP-R1 验证报告 | `coordination/evidence/AI-AGENT/CLEANUP_R1_VERIFIER_REPORT.md` | ✅ R2 更新 | 固定 commit 诚实性限定 |

### 2.3 基线文档（参考）

| 文件 | 路径 | 用途 |
| --- | --- | --- |
| SWARM_CHARTER | `coordination/design/AI-AGENT/SWARM_CHARTER.md` | 设计契约与完成标准 |
| BOOTSTRAP_EVIDENCE | `coordination/evidence/AI-AGENT/BOOTSTRAP_EVIDENCE.md` | 标准结果包模板与安全声明 |

### 2.4 旧命名文件状态

以下五个命名文件在固定 commit `8fcdb82` 中缺席，且当前文件系统中也不存在：

| 文件名 | 固定 commit 中存在？ | 当前存在？ |
| --- | --- | --- |
| `coordination/evidence/AI-AGENT/H1_EVIDENCE.md` | 否 | 否 |
| `coordination/design/AI-AGENT/H1_MULTI_MODEL_ADAPTER.md` | 否 | 否 |
| `coordination/design/AI-AGENT/H2_LEARNING_MEMORY_EVALUATION_DESIGN.md` | 否 | 否 |
| `coordination/evidence/AI-AGENT/H4_EVIDENCE.md` | 否 | 否 |
| `coordination/design/AI-AGENT/H4_SAFETY_TEST_PLAN.md` | 否 | 否 |

**NOT_VERIFIABLE_FROM_FIXED_COMMIT**：这些文件的删除行为（谁删除、何时删除、删除前内容、删除前行数/哈希值计数、删除前扫描结果）在固定 commit `8fcdb82` 中不可独立证明。从该固定 commit 可证明的事实限于：这五个文件在该 commit 中缺席。

---

## 3. 验证门结果

**DESIGN_REVIEW_PASS = true** — 设计/文档审查通过。

**SECURITY_EXECUTION_EVIDENCE_NOT_RUN = true** — 安全执行层面（P2 对抗测试、运行时注入测试、Go 核心集成测试、生产环境安全测试）尚未执行。

Verifier（`t_3de8d1f2`，R2 更新）完成设计级独立检查：

| # | 检查项 | 结果（设计审查） | 运行时执行 |
| --- | --- | --- | --- |
| 1 | 路径白名单合规 — 8/8 文件在允许路径内 | ✅ | — |
| 2 | 禁止系统访问 — 32 项逐一确认 | ✅ | — |
| 3 | 授权标志 — 16 标志全部 false | ✅ | — |
| 4 | SWARM_CHARTER 覆盖 — H1 7/7, H2 6/6, H3 6/6, H4 31 子用例 | ✅ | — |
| 5 | Fail-closed 语义 — 设计级验证完成（含 R2 修复新增） | ✅ (设计) | NOT_RUN |
| 6 | 跨设计一致性 — 6+1 交叉面全通过 | ✅ | — |
| 7 | 标准结果包完整性 — 4 worker × 13 字段全部存在 | ✅ | — |
| 8 | 证据充分性 — 4 worker 均有足够设计级证据 | ✅ (设计) | NOT_RUN |
| 9 | 决策注册表对齐 — 已验证存在且对齐 | ✅ | — |
| 10 | AGENTS.md 边界遵守 — 所有 worker 遵守产品边界 | ✅ | — |
| 11 | R2 修复交叉一致性 — 4 修复之间无矛盾 | ✅ (设计) | NOT_RUN |
| 12 | 固定 commit 诚实性 — NOT_VERIFIABLE_FROM_FIXED_COMMIT 已标记 | ✅ | — |

**非阻断性发现（5 项）：**
1. 旧文件状态：五个命名文件在固定 commit `8fcdb82` 中缺席（NOT_VERIFIABLE_FROM_FIXED_COMMIT：删除行为不可证明）
2. H4 §1.4 间接注入为前瞻测试（D-039 外部数据未接入），标记为 FORWARD-LOOKING
3. H4 Phase 2-4 安全测试尚未实现（NOT_YET_IMPLEMENTED，设计特征）
4. H2 ↔ H4 自我修改测试覆盖为间接验证（多层防护缓解）
5. R2 所有新增项均标记为 NOT_YET_IMPLEMENTED/NOT_EXECUTED（设计特征）

---

## 4. 模型路由

| 属性 | 值 |
| --- | --- |
| 主路由 | `deepseek-v4-pro`（DeepSeek，通过 OpenRouter） |
| 回退层 1 | `meta/llama-4-maverick`（Meta，通过 OpenRouter） |
| 回退层 2 | `mistral/mistral-large`（Mistral，通过 OpenRouter） |
| 回退耗尽 | NO_TRADE + 用户通知（fail-closed） |
| 已验证 Profile | `default` |
| 未验证路径 | `gpt-5.6-sol`、`gpt-5.6-terra`（OpenAI 直连，与 Hermes 环境不共享凭证） |
| 不可用路径 | Anthropic、Google（OpenRouter Provider Allowlist 403） |
| 发现协议 | Fail-closed：默认 UNVERIFIED → 实际 API 调用验证 → VERIFIED/DEGRADED/UNAVAILABLE |
| 错误分类 [R-H1] | 7 类：AUTH_FAILED/PROVIDER_DENIED/RATE_LIMITED/TRANSIENT_TRANSPORT/TIMEOUT/INVALID_RESPONSE/UNKNOWN |
| 缓存 TTL | 300s |

---

## 5. 安全边界汇总

以下边界在所有 H1-H4 设计（含 R2 修复）中保持一致，无设计层面的冲突或绕过：

| 边界 | 约束 | 覆盖的 Worker |
| --- | --- | --- |
| B1 文本隔离 | Hermes 文本不能直接签名、提交或授权订单 | H1, H3, H4 |
| B2 Schema 封堵 | MCP 工具 input_schema 拒绝 SERVER_SCOPE_FIELDS | H4 |
| B3 字段白名单 | Pydantic `extra='forbid'` | H4 |
| B4 Enum 锁死 | Symbol 仅 BTC/ETH/SOL-PERP | H4 |
| B5 Decimal 约束 | 规范化 decimal 字符串 | H4 |
| B6 Timestamp 约束 | 仅 UTC Z/z 终端 | H4 |
| B7 Stop 强制 | OPEN/INCREASE 必须带 stop（reduce-only STOP_MARKET） | H3, H4 |
| B8 确认绑定 | RFC 8785 JCS + SHA-256 confirmation_hash | H1, H3, H4 |
| B9 确认范围 | ConfirmationTicket 仅 OPEN/INCREASE | H3, H4 |
| B10 保护失败 | ProtectionFailed → 仅 reduce-only 紧急平仓 | H3, H4 |
| B11 未知结果 | UNKNOWN_REQUIRES_RECONCILIATION 不能推断为成功 | H3, H4 |
| B12 MCP 网关 | Hermes 仅通过受控 MCP 工具网关访问 Go 核心 | H1, H4 |
| B13 路由锁定 | Profile `default`，模型 `deepseek-v4-pro` | H1 |
| B14 Authorization | 仅 USER_CONFIRMATION / AUTOMATION_GRANT / RISK_REDUCTION | H4 |

---

## 6. 跨设计接口矩阵

| 接口 | 生产者 | 消费者 | 状态 |
| --- | --- | --- | --- |
| 模型路由 + fallback 链 | H1 | H2, H3, H4 | ✅ 一致（H4 B13 锁定同样路由） |
| 回退模型能力差异风险 | H1 | H2, H3 | ✅ 已标注，不影响设计 |
| 信任链：模型输出→contracts→确认→风控 | H1 | H3, H4 | ✅ 三层文档描述一致 |
| TradeIntent/ConfirmationTicket 结构 | Go 核心 types.go | H3, H4 | ✅ H3/H4 引用同一 source |
| 候选版本审批 UX | H2 | H3 | ⚠️ H3 未直接覆盖，但 H2 标注为 H3 依赖 |
| 确认卡流程 | H3 | H4 | ✅ H4 以 H3 确认卡为攻击目标 |
| 幂等防护 (含 compare-and-consume) [R-H3] | H3 | H4 | ✅ H4 以 H3 防护机制为测试目标 |
| UNKNOWN UX | H3 | H4 | ✅ 状态转换和 UX 约束一致 |
| 自我修改禁止 | H2 | H4 | ✅ 间接验证，多层防护缓解 |
| 学习记忆 → 策略记忆提升 | H2 | H3 | ✅ 审批流程需要 H3 UX 支持 |
| 错误分类 → 确认消费 [R-H1↔R-H3] | R-H1 | R-H3 | ✅ AUTH_FAILED 下确认不可消费 |
| 回退/撤销 → 路由行为 [R-H2↔R-H1] | R-H2 | R-H1 | ✅ 策略版本不可用时路由不变 |
| 确认伪造 → MCP 注入覆盖 [R-H3↔R-H4] | R-H3 | R-H4 | ✅ fail-closed 断言覆盖 |

---

## 7. 未解决风险汇总

| 风险 | 来源 | 严重性 |
| --- | --- | --- |
| 生产环境中 OpenAI 直连路径（Codex OAuth）可否共享给 Hermes Agent 路由 | H1 | 中 |
| 回退模型能力差异（无 REASONING）可能导致交易意图质量下降 | H1 | 中 |
| OpenRouter Provider Allowlist 未来变化可能影响回退可用性 | H1 | 低 |
| 冻结窗口最小样本量（30 笔）是建议阈值，需用户配置 | H2 | 低 |
| 影子运行与时序差异的容忍延迟窗口需在实现时确定 | H2 | 低 |
| 候选版本审批 UX 流程的交互细节由 H3 定义（H2↔H3 gap） | H2 | 低 |
| H4 Phase 2-4 安全测试尚未实现（NOT_YET_IMPLEMENTED，设计特征） | H4 | 低 |
| Go 核心集成测试不在 Python 范围，需跨团队协调 | H4 | 低 |
| 间接注入为前瞻测试（D-039 外部数据未接入） | H4 | 低 |
| R-H1 运行时能力发现未实现（NOT_IMPLEMENTED） | R-H1 | 中 |
| R-H2 append-only 事件持久化未实现（NOT_IMPLEMENTED） | R-H2 | 中 |
| R-H3 compare-and-consume 未实现（NOT_IMPLEMENTED） | R-H3 | 中 |
| R-H4 6 个 MCP 注入用例未执行（NOT_EXECUTED） | R-H4 | 中 |

---

## 8. 集成依赖

| 依赖 | 类型 | 状态 |
| --- | --- | --- |
| `services/hermes-agent/` contracts.py / validator.py / mcp_tools.py / confirmation.py | 内部（已存在） | ✅ |
| Go 交易核心：types.go / state.go / idempotency.go / scenarios.go | 内部（已存在） | ✅ |
| Finverse MCP：evidence_receipt / backtest_summary / frozen thresholds | 外部 MCP 服务 | ✅ |
| OpenRouter API | 外部 API | ✅ |
| `docs/05_DECISION_REGISTER.md` | 内部文档 | ✅ |
| `docs/06_FRONTEND_UX_SPEC.md` | 内部文档 | ✅ |
| `docs/07_MULTI_AGENT_WORKFLOW.md` | 内部文档 | ✅ |
| Profile 隔离：dev / research / trading | 基础设施依赖 | ⏳ 待创建 |
| R-H1 错误分类体系 Go 风控核心对齐 | 跨团队实现依赖 | ⏳ 待实现 |
| R-H2 append-only 事件存储 | 实现依赖 | ⏳ 待实现 |
| R-H3 compare-and-consume IdempotencyStore 扩展 | 实现依赖 | ⏳ 待实现 |
| R-H4 MCP 注入测试基础设施 | 实现依赖 | ⏳ 待实现 |

---

## 9. 设计覆盖度总表

| 设计域 | 需求数 | 覆盖数 | 覆盖率（设计审查） |
| --- | --- | --- | --- |
| H1 — 模型与 Adapter（含 R-H1） | 7 + 3 修复项 | 7 + 3 | 设计 100% |
| H2 — 学习记忆与评估（含 R-H2） | 6 + 2 修复项 | 6 + 2 | 设计 100% |
| H3 — 聊天与确认 UX（含 R-H3） | 6 + 1 修复项 | 6 + 1 | 设计 100% |
| H4 — 安全测试计划（含 R-H4） | 31 子用例（25 + 6） | 31 | 设计 100% |

R2 修复使安全测试子用例从 25 增至 31，fail-closed 语义从 23 点增至 31 点（+R-H1: 2, +R-H2: 2, +R-H3: 2, +R-H4: 1 类别×6 断言）。

---

## 10. 实现优先级（跨 Worker + R2 修复）

| 阶段 | 内容 | 依赖 Worker |
| --- | --- | --- |
| Phase 0 | Adapter 接口实现（ABC + 能力发现 + 启动发现） | H1 + R-H1 |
| Phase 0 | 合约层扩展（LineageEntry, FrozenWindow, append-only events） | H2 + R-H2 |
| Phase 1 | 回退链 + 健康检查 + 错误分类运行时发现 | H1 + R-H1 |
| Phase 1 | IdempotencyStore compare-and-consume 扩展 | H3 + R-H3 |
| Phase 1 | P0 安全测试（已有覆盖，仅需运行） | H4 |
| Phase 2 | 冻结窗口 + 离线评估管线 | H2 |
| Phase 2 | P1 安全测试（扩展确认 hash、Unicode 测试） | H4 |
| Phase 3 | 确认卡 UX 组件实现 | H3 |
| Phase 3 | 影子运行 + 候选版本审批 + CANDIDATE_ROLLED_BACK/STRATEGY_VERSION_REVOKED 持久化 | H2 + R-H2 |
| Phase 3 | P2 安全测试（prompt injection e2e、log sanitizer、MCP 注入 6 用例） | H4 + R-H4 |
| Phase 4 | Profile 隔离部署 | H2 |
| Phase 4 | P3 集成测试（Go 核心） | H4 |

---

## 11. R2 修复汇聚摘要

以下为 R-H1、R-H2、R-H3、R-H4 四个修复的结构化汇总。

### 11.1 R-H1：模型能力与失败分类修复

- **修复目标**：H1 §2 模型矩阵、§3.1 错误枚举、§5 错误分类体系、§6 启动发现规范
- **新增类**：ErrorClass 枚举（7 类）、VerifiedEntry dataclass（7 必需字段）
- **行为变更**：AUTH_FAILED/PROVIDER_DENIED/UNKNOWN → 不回退、不重试、立即 NO_TRADE
- **新增决策**：无新的 D-H1 编号（现有 5 条决策扩充语义）
- **NOT_IMPLEMENTED**：运行时能力发现、ErrorClass 对齐 Go 风控核心
- **文件**：H1_MULTI_MODEL_ADAPTER_DESIGN.md（470→~810 行）、H1_DESIGN_EVIDENCE.md（117→~160 行）

### 11.2 R-H2：追加型回退与撤销事件修复

- **修复目标**：H2 §7.5 CANDIDATE_ROLLED_BACK、§7.6 STRATEGY_VERSION_REVOKED
- **新增事件**：2 个 append-only 事件类型，各含 authorizer_identity、authorization_time、reason、evidence_hash、previous/target strategy versions
- **行为变更**：立即禁用、仅可切换到明确批准版本、NONE 触发 reduce-only 安全模式
- **同步更新**：事件类型表（3.2）、变更类型表（7.1）、自我修改禁止表（8.1+3 条）、优先级表（11）、风险表（12+2 条）
- **NOT_IMPLEMENTED**：事件持久化、append-only 存储实现
- **文件**：H2_LEARNING_AND_EVALUATION.md（437→~561 行）、H2_EVIDENCE.md（127→~183 行）

### 11.3 R-H3：原子确认消费修复

- **修复目标**：H3 §6 服务端原子确认比较并消费（Compare-and-Consume）语义
- **新增定义**：6 维度上下文绑定（用户/账户/设备/会话/目的/intent_hash）、原子 compare-and-consume 算法、confirmation_id/nonce 持久唯一性
- **新增场景**：11 种失败关闭场景（含持久化结果未知→UNKNOWN_REQUIRES_RECONCILIATION 桥接）
- **新增决策**：6 条（H3-D-011~H3-D-016）
- **NOT_IMPLEMENTED**：现有 IdempotencyStore 不包含 compare-and-consume 实现
- **文件**：H3_BUILT_IN_CHAT_AND_CONFIRMATION_UX.md（570→~727 行）、H3_EVIDENCE.md（73→~99 行）

### 11.4 R-H4：MCP 工具输出注入测试设计修复

- **修复目标**：H4 §7 新增 6 个 MCP/tool-output 注入用例
- **新增用例**：伪造系统指令、伪造确认/执行结果、嵌套 JSON 走私、类秘密载荷、时间线伪造、批量事件走私
- **FAIL-CLOSED 断言**（每用例 1 条）：工具输出=不可信数据，不可改变安全策略/执行工具/授权交易意图/证明执行/泄露秘密/绕过协调
- **新增标记**：全部 NOT_YET_IMPLEMENTED/NOT_EXECUTED、P2 执行要求、证据模板
- **文件**：H4_SECURITY_TEST_PLAN.md（356→~441 行）、H4_SECURITY_EVIDENCE.md（167→~313 行）

### 11.5 R2 交叉一致性验证

| 修复对 | 一致性检查 | 结果 |
| --- | --- | --- |
| R-H1 ↔ R-H2 | 错误分类不改变事件生命周期语义 | ✅ |
| R-H1 ↔ R-H3 | AUTH_FAILED 下确认不可消费（fail-closed 一致） | ✅ |
| R-H1 ↔ R-H4 | MCP 输出标准化→验证，不可绕过适配器路由 | ✅ |
| R-H2 ↔ R-H3 | 策略版本不可用时 compare-and-consume 不可通过 | ✅ |
| R-H2 ↔ R-H4 | CANDIDATE_ROLLED_BACK/REVOKED 事件不可通过工具输出注入伪造 | ✅ |
| R-H3 ↔ R-H4 | 伪造确认结果被 R-H4 fail-closed 断言覆盖 | ✅ |

---

## 12. 授权状态

```
MERGE=false
DEPLOY=false
PRODUCTION=false
LIVE_TRADING=false
DESIGN_REVIEW_PASS=true           (设计审查通过)
SECURITY_EXECUTION_EVIDENCE_NOT_RUN=true  (安全/P2/运行时测试未执行)
```

本合成包汇总 H1-H4 设计产物及 R2 四个修复产出，不授权任何代码实现、部署、合并或实盘交易。所有安全边界（B1-B14）、NOT_YET_IMPLEMENTED/NOT_EXECUTED 标记、授权标志和门状态分离均保留。
