# H2: 交易风格学习记忆与离线评估设计

## 1. 设计范围与边界

本设计文件仅定义 FIT-Trade 系统中 Hermes Agent 的学习、记忆与离线评估体系。

**允许写入路径**：`coordination/design/AI-AGENT/H2*`、`coordination/evidence/AI-AGENT/H2*`

**明确禁止**：
- 访问任何凭据、私钥、token、cookie、signer、交易所写 API
- 连接实盘钱包、生产数据库、生产模型
- 修改 `services/trading-core/`、`services/hyperliquid-adapter/`、`infra/`
- 自我修改——学习记忆系统不得从任何 live/paper 执行路径更新自身
- 将回测通过等同于策略/交易授权

## 2. 架构总览

```
┌─────────────────────────────────────────────────────────────┐
│                     用户交互层                               │
│  ┌──────────┐  ┌──────────┐  ┌──────────────┐              │
│  │ 聊天指令  │  │ 确认反馈  │  │ 复盘评分/标注 │              │
│  └────┬─────┘  └────┬─────┘  └──────┬───────┘              │
│       │             │               │                        │
│       ▼             ▼               ▼                        │
│  ┌─────────────────────────────────────────────────────┐     │
│  │              学习数据入口（唯一入口）                  │     │
│  │  仅确认交易 + 明确反馈 + 复盘标注 → 进入学习管线       │     │
│  │  普通聊天 / 未确认操作 / 模拟结果 → 永不进入           │     │
│  └──────────────────────┬──────────────────────────────┘     │
│                         │                                    │
│                         ▼                                    │
│  ┌─────────────────────────────────────────────────────┐     │
│  │           数据谱系 & 冻结窗口层（不可变）              │     │
│  │  ┌─────────┐  ┌──────────┐  ┌────────────────┐      │     │
│  │  │ 谱系ID  │  │ 时间窗口  │  │ 上下文快照哈希  │      │     │
│  │  │ 完整链  │  │ 冻结标记  │  │ (SHA-256)      │      │     │
│  │  └─────────┘  └──────────┘  └────────────────┘      │     │
│  └──────────────────────┬──────────────────────────────┘     │
│                         │                                    │
│              ┌──────────┴──────────┐                         │
│              ▼                     ▼                         │
│  ┌──────────────────┐  ┌──────────────────────┐             │
│  │  研究/回测环境    │  │  策略/交易授权环境    │             │
│  │  (READ-ONLY)      │  │  (READ-ONLY copy)     │             │
│  │  离线评估          │  │  候选版本审批         │             │
│  │  影子运行          │  │  人工变更控制         │             │
│  │  风格复现研究      │  │  ≠ 自动交易授权       │             │
│  └────────┬─────────┘  └──────────┬───────────┘             │
│           │                       │                          │
│           ▼                       ▼                          │
│  ┌─────────────────────────────────────────────────────┐     │
│  │              隔离存储层                              │     │
│  │  研究Profile / 交易Profile 完全分离                   │     │
│  │  研究数据可→交易？必须人工审批 + 版本化 + 证据链      │     │
│  └─────────────────────────────────────────────────────┘     │
└─────────────────────────────────────────────────────────────┘
```

## 3. 数据谱系（Data Lineage）

### 3.1 谱系核心原则

参照决策 D-011、D-028：

1. **仅确认交易和明确反馈进入学习数据**。普通聊天、未确认操作、模拟/回测结果不自动流入学习记忆。
2. **风格复现（research pass）与改进应用（trading pass）是两条独立授权路径**。研究通过不代表策略可以实盘。
3. **每条学习记录携带完整谱系链**，从原始用户交互到最终记忆条目不可截断。

### 3.2 学习事件类型

| 事件类型 | 触发条件 | 数据内容 |
| --- | --- | --- |
| `TRADE_CONFIRMED` | 用户明确确认开仓/加仓 | intent、confirmation ticket、execution result |
| `TRADE_CLOSED_REVIEW` | 仓位平仓后用户反馈 | 复盘评分（1-5）、标签（"符合计划"/"过早"/"过晚"/"错误方向"）、自由文本 |
| `MANUAL_CORRECTION` | 用户主动标注"这单不该做" | 标注类型、原因 |
| `STYLE_ATTRIBUTION` | 用户明确声明某交易归属某风格 | 风格 ID、交易 ID |
| `CANDIDATE_PROMOTED` | 用户批准候选版本→策略 | 候选版本号、批准时间、批准人 |
| `CANDIDATE_ROLLED_BACK` | 人工授权：候选版本回退到明确已批准版本 | rolled_back_version, target_version, authorizer_identity, authorization_time, reason, evidence_hash |
| `STRATEGY_VERSION_REVOKED` | 人工授权：撤销活跃策略版本→已批准版本或 NONE | revoked_version, target_version, authorizer_identity, authorization_time, reason, evidence_hash, affected_instruments |

### 3.3 谱系链结构

每条学习记录必须包含：

```text
LineageEntry {
    lineage_id: UUID              // 唯一谱系ID
    parent_lineage_id: UUID|null  // 前一个谱系条目（链式）
    source_event_id: UUID         // 原始事件ID
    event_type: enum              // 上述事件类型之一
    source: "USER_DIRECTED"       // 固定——仅用户触发
    user_id: UUID
    account_id: UUID
    session_id: UUID
    intent_id: UUID|null          // 关联的交易意图
    confirmation_id: UUID|null    // 关联的确认票据
    frozen_window_id: UUID        // 所属冻结窗口
    created_at: TimestampStr
    content_hash: HashStr         // 内容的 SHA-256 哈希
    schema_version: "fit.trade.v1"
}
```

### 3.4 谱系不可变性要求

- 谱系条目一旦创建不可修改、不可删除。
- 更新路径只允许追加新条目（parent_lineage_id 指向被修正的条目）。
- 从研究到交易的"提升"创建新的 `CANDIDATE_PROMOTED` 事件，包含完整研究谱系引用。

## 4. 记忆隔离（Memory Isolation）

### 4.1 三层隔离

| 层级 | 内容 | 隔离粒度 | 读写权限 |
| --- | --- | --- | --- |
| **会话记忆** | 当前对话上下文、最近工具调用 | 按 session 隔离 | 读写（会话内） |
| **研究记忆** | 回测结果、影子运行统计、风格模式 | 按 research profile 隔离 | 只能追加 |
| **策略记忆** | 已授权的交易风格、用户偏好、风险参数 | 按 trading profile 隔离 | 人工审批后追加 |

### 4.2 跨层数据流

```
会话记忆 ──(用户确认/反馈)──▶ 研究记忆 ──(人工审批)──▶ 策略记忆
   │                              │                        │
   │   永不自动提升               │   必须满足阈值         │    只读给 Hermes
   │   会话结束即归档             │   和人工批准           │    交易推理
```

**严禁**：
- 会话记忆自动提升为研究记忆
- 研究记忆自动提升为策略记忆
- 任何记忆被运行中的 Hermes 执行路径修改（包括 live 和 paper）
- 跨用户/跨账户记忆混合

### 4.3 Hermes Profile 隔离策略

根据 `docs/07_MULTI_AGENT_WORKFLOW.md` 第 6 节：

- 开发 Profile：`~/.hermes/profiles/dev/` — 仅用于开发测试
- 研究 Profile：`~/.hermes/profiles/research/` — 回测与评估
- 交易 Profile：`~/.hermes/profiles/trading/` — 生产级交易推理

三个 Profile 的学习记忆、会话数据、Skill 和记忆条目完全隔离。研究 Profile 的评估结果必须有独立的迁移审批流程才能进入交易 Profile。

## 5. 冻结数据窗口（Frozen Data Windows）

### 5.1 概念

冻结窗口是研究中一个不可变的时间区间快照。在该窗口内运行的所有评估使用完全相同的数据视图，确保结果可复现。

### 5.2 窗口定义

```text
FrozenWindow {
    window_id: UUID
    label: string                 // 如 "2026Q3-Candidate-1"
    instrument_ids: [string]      // BTC-PERP, ETH-PERP, SOL-PERP
    interval: string              // "1H", "4H"
    start_utc: TimestampStr
    end_utc: TimestampStr
    frozen_at: TimestampStr       // 冻结时间（此后数据不再更新）
    events_root_hash: HashStr     // 窗口内所有事件的 Merkle root
    bar_count: int
    schema_version: "fit.trade.v1"
    is_candidate: bool            // true=候选版本评估窗口
}
```

### 5.3 冻结流程

1. **确定窗口**：选择足够历史数据（最少 100 bars，参照 D-042）
2. **快照**：从 `MarketSnapshot` 和 `OHLC` 事件构建完整不可变快照
3. **Merkle 树哈希**：所有事件按时间排序后构建 Merkle 树，root hash 作为窗口指纹
4. **冻结**：窗口被冻结后用于所有评估，无法再修改
5. **验证**：任何评估可以独立验证窗口完整性（重新计算 Merkle root）

### 5.4 窗口生命周期

```
定义窗口 → 构建快照 → 计算 Merkle Root → 冻结
                                            │
                                            ▼
                              所有评估引用此窗口（不可变）
                                            │
                                            ▼
                              候选通过审批 → 升级为"黄金标准窗口"
                              候选未通过   → 归档（不变）
```

## 6. 离线评估（Offline Evaluation）

### 6.1 评估管线

```
┌──────────┐    ┌────────────┐    ┌──────────────┐    ┌────────────┐
│ 冻结窗口  │──▶│ 风格复现    │──▶│ 影子模拟      │──▶│ 指标计算    │
│ (数据源)  │    │ (模拟交易)  │    │ (并行对比)    │    │ (风险/收益) │
└──────────┘    └────────────┘    └──────────────┘    └─────┬──────┘
                                                           │
                                    ┌──────────────────────┘
                                    ▼
┌──────────┐    ┌────────────┐    ┌──────────────┐
│ 评估报告  │◀───│ 风险指标    │◀───│ 统计检验    │
│ (人工审批)│    │ (回撤/夏普) │    │ (稳定性检验) │
└──────────┘    └────────────┘    └──────────────┘
```

### 6.2 评估指标

| 指标 | 定义 | 最小阈值 |
| --- | --- | --- |
| 总交易次数 | 窗口内的信号生成次数 | ≥ 30 |
| 胜率 | 盈利交易 / 总交易 | 不设阈值（信息性） |
| 盈亏比 | 平均盈利 / 平均亏损 | ≥ 1.2 |
| 最大回撤 | 峰值到谷底的最大资金回撤 | ≤ 用户配置 |
| 夏普比率 | (年化收益 - 无风险利率) / 年化波动率 | ≥ 0.5 |
| Calmar 比率 | 年化收益 / 最大回撤 | ≥ 0.3 |
| 单笔最大亏损 | 最差单笔交易的资金损失 | ≤ 用户单笔风险上限 |
| 连续亏损次数 | 连续亏损的最大次数 | ≤ 5 |

### 6.3 影子运行（Shadow Running）

候选版本在实盘旁并行运行，不产生实际订单：

1. 候选版本使用与实盘相同的市场数据
2. 生成信号但不执行——记录"影子信号"和时间戳
3. 将影子信号与真实交易（如果有）进行对比
4. 汇总影子运行指标，供人工审批参考

影子运行的信号记录格式：

```text
ShadowSignal {
    signal_id: UUID
    candidate_version: string    // 候选版本号
    window_id: UUID              // 冻结窗口引用
    created_at: TimestampStr
    symbol: Symbol
    side: Side
    position_effect: PositionEffect
    quantity: DecimalStr
    entry_price: DecimalStr
    stop_loss: DecimalStr
    take_profit_plan: [...]
    confidence: FractionStr      // 模型置信度 [0, 1]
    reasoning: string            // 推理摘要
    executed: bool               // 是否实际执行（影子=false）
}
```

### 6.4 评估报告

每次评估生成标准报告：

```text
EvaluationReport {
    report_id: UUID
    candidate_version: string
    window_id: UUID
    evaluated_at: TimestampStr
    metrics: { metric_name: value }
    shadow_signals_count: int
    shadow_vs_live_diff: [...]     // 影子与实盘差异
    passed_thresholds: bool
    unpassed_thresholds: [string]  // 未达标的指标列表
    requires_human_review: bool    // 必须有未达标时=true
    reviewer_id: UUID|null
    approved_at: TimestampStr|null
    decision: "PENDING"|"APPROVED"|"REJECTED"
    evidence_receipt: HashStr      // Finverse evidence receipt
}
```

## 7. 人工审核变更控制（Human-Reviewed Change Control）

### 7.1 变更类型与审批路径

| 变更类型 | 触发者 | 审批者 | 安全要求 |
| --- | --- | --- | --- |
| 学习事件追加 | 用户操作 | 自动（谱系记录） | 仅追加，不可变 |
| 风格模式更新 | 系统建议 | 用户确认 | 需要至少 1 个冻结窗口评估通过 |
| 候选版本 → 策略 | 用户批准 | 用户本人 | 满足 D-042 的样本/影子/风险指标 |
| 风险参数变更（放宽） | 用户 | 设备密码验证 | D-010、D-016 |
| 策略记忆删除 | 用户 | 设备密码验证 | 软删除，保留审计记录 |
| 候选版本回退 | 人工授权 | 用户本人 | 必须提供 evidence_hash、回退原因、目标版本 |
| 策略版本撤销 | 人工授权 | 用户本人 | 必须提供 evidence_hash、撤销原因、目标版本；target_version 可为 NONE |

### 7.2 候选版本审批流程

```
候选版本满足全部指标 ──▶ 生成评估报告
                              │
                              ▼
                    人工审批（用户查看报告）
                         ╱          ╲
                    批准             拒绝/暂缓
                     │                 │
                     ▼                 ▼
              创建 CANDIDATE_      记录拒绝原因
              PROMOTED 事件        返回研究队列
                     │
                     ▼
              策略记忆更新
              （追加，不可变）
```

### 7.3 审批证据链

每次审批必须产出完整的证据链：

1. **输入**：冻结窗口 ID、窗口内数据根哈希
2. **处理**：候选版本号、使用的指标集、回测执行时间
3. **输出**：所有指标的实际值、影子运行对比、通过/未通过列表
4. **决策**：批准/拒绝、批准人、时间、理由
5. **加密证明**：Finverse `evidence_receipt` 哈希，包含上述全部摘要

### 7.4 变更拒绝后的处理

- 拒绝原因记录在研究记忆（软删除由策略记忆，保留审计记录）
- 被拒绝的候选版本可以修改后重新提交
- 重新提交需要新的冻结窗口（不得使用同一窗口重新评估以制造虚假通过）

### 7.5 候选版本回退（Candidate Rollback）

`CANDIDATE_ROLLED_BACK` 是追加型生命周期事件，用于将已提升为策略的候选版本安全回退到先前明确批准的版本。回退不删除或重写历史记录，仅标记版本的禁用状态。

**触发条件**：
- 已提升的候选版本在实盘并行验证（影子运行）中发现缺陷
- 影子运行指标显著偏离预期阈值
- 用户或安全审查发现候选版本存在风险

**事件结构**：

```text
CANDIDATE_ROLLED_BACK {
    event_id: UUID                  // 唯一事件ID
    event_type: "CANDIDATE_ROLLED_BACK"
    lineage_id: UUID                // 谱系条目ID（追加，不可变）
    parent_lineage_id: UUID         // 指向被回退的 CANDIDATE_PROMOTED 事件谱系
    authorizer_identity: string     // 人工授权人身份/角色（必填）
    authorization_time: TimestampStr  // 授权时间（必填）
    reason: string                  // 回退原因（必填，不可为空）
    evidence_hash: HashStr          // 支撑回退决策的证据哈希（必填）
    rolled_back_version: string     // 被回退的候选版本号
    target_version: string          // 回退到的目标版本号（必须是明确批准过的版本）
    previous_versions: [string]     // 先前链上版本列表
    disabled_at: TimestampStr       // 被回退版本的立即禁用时间
    schema_version: "fit.trade.v1"
}
```

**核心原则**：

1. **历史记录不可变**：`CANDIDATE_ROLLED_BACK` 不删除或重写原有的 `CANDIDATE_PROMOTED` 事件。被回退版本的 PROMOTED 事件在审计追踪中永久保留，仅被 ROLLED_BACK 事件标注为已禁用。
2. **只能回退到明确批准的版本**：`target_version` 必须指向一个曾经通过 `CANDIDATE_PROMOTED` 审批且当前未被撤销的版本。不得回退到未批准或已被撤销的版本。
3. **立即禁用**：被回退的候选版本在 `CANDIDATE_ROLLED_BACK` 事件创建时立即禁用，系统不得再使用该版本生成任何交易建议。
4. **不可自我回退**：任何自动化路径（包括 Hermes 推理、回测引擎、影子运行）不得自行触发回退。回退只能由人工授权人发起。
5. **回退不等于删除**：回退是版本切换操作，不是证据抹除操作。所有与被回退版本相关的历史数据（影子运行记录、评估报告、审批记录）保持不变且可审计。
6. **研究证据与策略授权的分离保持**：回退一个候选版本不影响该版本对应的研究证据。研究证据是独立、不可变的记录；回退仅影响策略授权状态。

**审批证据链**：

每次回退必须产出完整的证据链：
1. **输入**：被回退版本号、目标版本号、回退原因
2. **证据**：触发回退的评估报告或异常检测证据的哈希
3. **决策**：授权人身份、时间、理由
4. **加密证明**：Finverse `evidence_receipt` 哈希，包含上述全部摘要

### 7.6 策略版本撤销（Strategy Version Revocation）

`STRATEGY_VERSION_REVOKED` 是追加型生命周期事件，用于在发现安全缺陷或策略不再适用时，将活跃策略版本安全撤销并回退到明确指定的已批准版本。

**与 `CANDIDATE_ROLLED_BACK` 的区别**：
- `CANDIDATE_ROLLED_BACK` 针对**候选版本**（Candidate Version）被提升后短期发现问题的场景
- `STRATEGY_VERSION_REVOKED` 针对已**长期运行的活跃策略版本**（Active Strategy Version）因安全缺陷、市场环境根本变化或合规原因需要撤销的场景

**触发条件**：
- 已授权策略版本发现安全缺陷或逻辑错误
- 市场条件发生根本性变化使策略不再适用
- 合规或风险审查要求撤销

**事件结构**：

```text
STRATEGY_VERSION_REVOKED {
    event_id: UUID                       // 唯一事件ID
    event_type: "STRATEGY_VERSION_REVOKED"
    lineage_id: UUID                     // 谱系条目ID（追加，不可变）
    parent_lineage_id: UUID              // 指向被撤销版本的最后一个 PROMOTED 事件谱系
    authorizer_identity: string          // 人工授权人身份/角色（必填）
    authorization_time: TimestampStr     // 授权时间（必填）
    reason: string                       // 撤销原因（必填，不可为空）
    evidence_hash: HashStr               // 支撑撤销决策的证据哈希（必填）
    revoked_version: string              // 被撤销的策略版本号
    target_version: string               // 撤销后回退到的目标版本号（必须是明确批准过的版本，或 "NONE" 表示无替代版本）
    affected_instruments: [string]       // 受影响的交易对
    disabled_at: TimestampStr            // 被撤销版本的立即禁用时间
    schema_version: "fit.trade.v1"
}
```

**核心原则**：

1. **历史记录不可变**：撤销不能重写或抹除先前的证据、交易记录或评估报告。被撤销版本的所有历史数据在审计追踪中永久保留。
2. **只能回退到明确批准的版本**：`target_version` 必须指向一个经过独立审批的有效版本。如果没有任何已批准版本可用，`target_version` 设为 `"NONE"`，系统进入仅允许 `reduce-only` 操作的安全模式。
3. **立即禁用**：被撤销的策略版本在 `STRATEGY_VERSION_REVOKED` 事件创建时立即禁用，所有使用该版本的活跃建议和待确认操作被标记为无效。
4. **不可自我撤销**：任何自动化路径（包括 Hermes 推理、风控引擎、合规扫描）不得自行触发撤销。撤销只能由人工授权人发起。
5. **撤销保留完整审计链**：撤销后的版本在历史记录中标记为 `"REVOKED-at-{time}-by-{authorizer}"`，不删除任何相关联的数据。
6. **研究证据与策略授权的分离保持**：撤销一个策略版本不影响该版本对应的研究证据。研究证据是独立的、不可变的记录；撤销仅影响策略授权状态。

**撤销后的处理**：
- 被撤销版本的所有研究记忆（评估报告、影子运行记录）保持完整且可访问
- 撤销后系统自动切换到 `target_version`，或进入 `NONE` 安全模式
- 被撤销的版本可以基于新的研究和独立证据重新提交审批
- 重新审批必须使用新的冻结窗口和独立评估，不得复用撤销前的评估结果

**撤销流程**：

```text
活跃策略版本 ──▶ 发现安全缺陷/合规问题
                     │
                     ▼
              收集证据（异常指标/审查报告）
                     │
                     ▼
              人工授权撤销 ──▶ 创建 STRATEGY_VERSION_REVOKED 事件
                     │
                     ▼
              立即禁用被撤销版本
                     │
                     ▼
              切换至 target_version 或 NONE 模式
              （追加，不可变，历史记录完整保留）
```

## 8. 自我修改禁止（Self-Modification Prohibition）

### 8.1 禁止路径

以下路径被**明确禁止**，在设计层面不可绕过：

| 路径 | 说明 |
| --- | --- |
| Hermes → 研究记忆写入 | Hermes 可以读取研究记忆进行推理，但不得修改 |
| Hermes → 策略记忆写入 | Hermes 可以读取策略记忆进行交易风格推断，但不得修改 |
| Hermes → 候选版本提升 | Hermes 不得自行批准候选版本 |
| Hermes → 风险参数放宽 | Hermes 不得自行放宽任何风险参数 |
| 执行路径 → 自我学习 | 任何 live/paper 执行结果不得自动流入学习记忆 |
| 回测结果 → 自动授权 | 回测通过不等于策略授权，必须有独立人工审批 |
| Hermes → 候选版本回退 | Hermes 不得自行触发 CANDIDATE_ROLLED_BACK 事件 |
| Hermes → 策略版本撤销 | Hermes 不得自行触发 STRATEGY_VERSION_REVOKED 事件 |
| 自动化路径 → 自我回退/撤销 | 回测引擎、影子运行、风控引擎不得自动触发版本回退或撤销 |

### 8.2 实现防护

- Hermes 的 prompt 中嵌入不可变的系统指令层
- 学习记忆写入 API 需要独立的用户认证（设备密码或等效），不经过 Hermes tool gateway
- Go 交易核心在 `AutomationAuthorization` 中维护独立的状态机，不受 Hermes 文本输出影响
- 符合 `contracts.py` 中 `ConfirmationTicket` 和 `GrantState` 的设计语义

## 9. 研究路径 vs 策略/交易授权

### 9.1 两条路径的明确区分

| 属性 | 研究路径（Research Pass） | 策略授权（Trading Authorization） |
| --- | --- | --- |
| 目的 | 发现、回测、影子验证交易风格 | 实盘交易中的风格辅助 |
| 数据源 | 冻结历史窗口 | 实时市场数据（通过 Go 风控） |
| 输出 | 评估报告、统计指标、建议 | 结构化 TradeIntent（需确认） |
| 写入权限 | 追加研究记忆 | 无写入权限（只读策略记忆） |
| 能否发单 | 否 | 否（仅生成建议，Go 确认） |
| 谁来批准 | 用户审阅评估报告 | 用户确认每个 TradeIntent |
| 记忆影响 | 可写入研究 Profile | 不可写入任何记忆 |
| 模型 | 可实验不同模型/参数 | 固定已批准的模型和版本 |

### 9.2 授权传递链

```
用户意图 ──▶ 研究路径（发现风格）
                │
                ▼
           离线评估（冻结窗口回测）
                │
                ▼
           影子运行（并行验证）
                │
                ▼
           人工审批（评估报告审阅）
                │
                ▼
           策略记忆更新（候选→已授权）
                │
                ▼
           交易路径（实盘推理，需 Go 确认）
```

## 10. 与现有系统集成

### 10.1 与 contracts.py 的关系

本设计新增以下概念，需要未来在 contracts 层实现：

- `LineageEntry` — 谱系条目
- `FrozenWindow` — 冻结数据窗口
- `ShadowSignal` — 影子信号
- `EvaluationReport` — 评估报告
- `StyleDefinition` — 风格定义
- `CandidateVersion` — 候选版本

这些不改变现有 `TradeIntent`、`ConfirmationTicket`、`Operation` 等核心契约，而是新增独立模块。

### 10.2 与 Finverse MCP 的关系

- `evidence_receipt` — 用于审批证据链的加密证明
- `backtest_summary` — 可辅助离线评估计算
- `frozen thresholds` — 可在风格分类中使用

### 10.3 与 Go 交易核心的关系

- Go 核心的 `AutomationAuthorization` 必须独立维护授权状态
- 策略记忆中的风格定义以版本化的只读方式提供给 Go 核心
- Hermes 的推理输出（建议）始终通过 Go 核心的确定性校验

## 11. 实现优先级

| 阶段 | 内容 | 依赖 |
| --- | --- | --- |
| Phase 1 | 数据谱系基础：LineageEntry 结构、不可变追加 | 无 |
| Phase 2 | 冻结窗口：FrozenWindow 定义、Merkle 树、快照 | Phase 1 |
| Phase 3 | 离线评估管线：回测引擎、指标计算、报告生成 | Phase 2 |
| Phase 4 | 影子运行：ShadowSignal、并行对比 | Phase 2 |
| Phase 5 | 候选版本全生命周期审批：CANDIDATE_PROMOTED/CANDIDATE_ROLLED_BACK/STRATEGY_VERSION_REVOKED 事件 | Phase 3+4 |
| Phase 6 | 策略记忆隔离与交易路径只读访问 | Phase 5 |

## 12. 风险与未决问题

| 风险 | 缓解措施 |
| --- | --- |
| 冻结窗口数据不足以支撑统计显著性 | 设置最少 30 笔交易阈值，不足则自动标记"数据不足" |
| 用户可能跳过审批直接使用候选版本 | 系统设计中审批是必经路径，Go 核心检查授权状态 |
| 影子运行与实盘的时序差异导致对比失真 | 记录精确时间戳，对比时容忍合理延迟窗口 |
| 多个候选版本间的交叉污染 | 每个候选版本使用独立冻结窗口评估 |
| 研究 Profile 被误用为交易 Profile | Profile 级别的配置隔离 + 启动时检查 |
| 回退到过时版本的风险 | 目标版本必须经过当前市场条件的重新验证；回退后应尽快安排新的候选评估 |
| 撤销后无可用已批准版本（NONE 模式） | NONE 模式限制为 reduce-only 操作，确保系统安全退出 |

---

## A. 授权状态

```
MERGE=false
DEPLOY=false
PRODUCTION=false
LIVE_TRADING=false
```

本设计文件仅定义了学习记忆与离线评估的架构蓝图，不授权任何代码实现、部署、合并或实盘交易。
