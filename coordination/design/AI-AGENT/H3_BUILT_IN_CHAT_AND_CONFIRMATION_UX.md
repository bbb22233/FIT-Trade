# H3: 内置聊天与开仓/加仓确认 UX 设计

## 1. 文档状态与范围

| 属性 | 值 |
| --- | --- |
| 任务 | H3 |
| 状态 | DESIGN_COMPLETE |
| 路径白名单 | `coordination/design/AI-AGENT/H3*` `coordination/evidence/AI-AGENT/H3*` |
| 禁止写入 | `services/trading-core/` `services/hyperliquid-adapter/` `services/hermes-agent/` 凭据/密钥/签名/交易所写 API |
| 授权状态 | `MERGE=false` `DEPLOY=false` `PRODUCTION=false` `LIVE_TRADING=false` |

本文档覆盖：
- 内置聊天界面的信息架构、消息分类和视觉规则
- 模拟数据（mock data）的强制性视觉标记
- 开仓和加仓的结构化确认 UX（确认卡生命周期、字段、按钮文案、超时/取消）
- Stop Market 保护的可见性、验证失败和紧急退出路径
- 幂等与重复提交防护的客户端设计
- `UNKNOWN_REQUIRES_RECONCILIATION` 状态的完整 UX 流程
- 服务端原子确认比较与消费（compare-and-consume）语义：上下文绑定、原子消费、持久唯一性、失败关闭

## 2. 内置聊天 UX 设计

### 2.1 布局与定位

内置聊天是 Hermes 的主要用户交互界面，位于桌面右侧 Hermes 常驻面板（380–440 px）。
在窄屏模式下（≤1279 px）变为可展开/收起的右侧抽屉。

面板结构（自上而下）：

```
┌──────────────────────┐
│ 上下文标题栏          │  ← 当前品种/账户/模式
├──────────────────────┤
│                      │
│  消息流              │  ← 可滚动的聊天记录
│                      │
├──────────────────────┤
│  工具活动指示器      │  ← Hermes 正在获取数据/计算
├──────────────────────┤
│  输入区              │  ← 文本输入 + 快捷指令
└──────────────────────┘
```

### 2.2 消息分类与视觉标记

每条消息必须明确标注其来源和权威级别：

| 消息类型 | 来源 | 视觉标记 | 示例 |
| --- | --- | --- | --- |
| 用户文本 | 用户输入 | 右对齐，用户头像/缩写 | "BTC 现在什么情况" |
| Hermes 分析 | Hermes 模型输出 | 左对齐，Hermes 蓝紫标识 + `分析` 标签 | "BTC 4h 处于上升趋势…" |
| Hermes 建议 | Hermes 模型输出 | 左对齐，Hermes 蓝紫标识 + `建议` 标签 | "建议在 42000 附近轻仓试多" |
| 系统状态 | Go 核心/服务端 | 左对齐，系统灰色标识 + `系统` 标签 | "账户数据已刷新" |
| 交易意图 | Hermes + Go 风控 | 结构化卡片，非纯文本 | 确认卡（参见第3节） |
| 操作结果 | Go 核心/交易所 | 结构化状态，独立组件 | "开仓 BTC 已成交" |
| 错误/告警 | 系统 | 红色/橙色边框 + 图标 | "止损建立失败" |
| 工具状态 | Hermes MCP 工具 | 小字灰色内联 | "正在获取 OKX 行情…" |

**关键规则：**
- Hermes 自由文本不可包含 "已确认" "已下单" "已保护" 等状态声明。
- 交易意图必须使用独立的结构化组件渲染，不得混入聊天 Markdown。
- 系统状态消息不可被 Hermes 文本覆盖或"解释为成功"。

### 2.3 模式标记（Mode Badge）

每条 Hermes 消息和每条交易意图必须携带当前模式的明确标记：

| 模式 | 标记文案 | 颜色 |
| --- | --- | --- |
| `MAINNET` | `主网` | 红色边框 + 实心标签 |
| `SIMULATION` | `模拟` | 橙色边框 + 空心标签 |
| `HYPOTHETICAL` | `假设讨论` | 灰色边框 + 空心标签 |

模式标记显示在 Hermes 消息头部、上下文标题栏和所有结构化确认卡中。
模式切换后，已有消息的标记不会改变。

### 2.4 模拟数据标记（Mock Data Labeling）

在开发和模拟阶段，所有非主网数据必须带有不可忽略的视觉标记：

**数据层面：**
- 模拟订单使用 `SIMULATED_ORDER_` 前缀 Order ID。
- 模拟成交在成交列表中标记 `模拟成交` 并使用灰色斜体。
- 模拟持仓在仓位表的 `类型` 列标注 `模拟`。

**界面层面：**
- `SIMULATION` 模式下：
  - 顶部状态栏环境标记为 `SIMULATION`（橙色）。
  - 所有价格、数量、盈亏数值行添加半透明橙色竖线指示条。
  - 确认卡标题显示 "模拟开仓确认"（非 "主网开仓确认"）。
  - 按钮文案使用 "确认模拟开多 BTC"（非 "确认实盘开多 BTC"）。
- `HYPOTHETICAL` 模式下：
  - 确认卡不生成，仅生成分析摘要。

**开发环境标记：**
- 开发构建在窗口标题和页面底部固定显示 `DEV BUILD — NOT FOR PRODUCTION`。
- 连接非生产 API 时，环境标记旁显示黄色 `DEV-API`。

### 2.5 补问流程（Missing Field Question）

当用户指令缺少必要参数时，Hermes 不得自行假设默认值。必须通过结构化补问收集：

```
用户："BTC 做多"
  ↓
Hermes 识别缺失：杠杆、止损、数量、订单类型
  ↓
Hermes 发送结构化补问卡片（非纯文本）：
  ┌────────────────────────────────────────┐
  │ 🔶 需要补充信息                          │
  │                                        │
  │ 杠杆：[  ]  (1-100)                     │
  │ 止损触发价：[  ] USD                     │
  │ 数量/金额：[  ] BTC / USD               │
  │ 订单类型：○ 市价  ○ 限价                │
  │ 保证金模式：○ 全仓  ○ 逐仓              │
  │ 理由（可选）：[  ]                       │
  │                                        │
  │ [生成交易意图]                           │
  └────────────────────────────────────────┘
```

补问前 Hermes 必须已获取当前市场数据、账户状态和风险配置。

**未指定则不可进入可确认状态的字段：**
- 杠杆 ×
- Stop Market ×
- 数量 or 名义价值 ×
- 非 BTC/ETH/SOL 品种 ×（实盘模式下）

### 2.6 聊天上下文

上下文标题栏显示：
- 当前品种（BTC / ETH / SOL）
- 当前账户
- 数据状态（`LIVE` / `STALE` / `RECONCILING`）
- 模式（`MAINNET` / `SIMULATION` / `HYPOTHETICAL`）

跨页面导航时，聊天面板在指挥中心常驻，在其他页面保持会话但可收起。
切换到不同品种时，聊天上下文自动切换，但保留历史消息。

### 2.7 快捷指令

输入区上方提供常用快捷指令按钮：
- "分析当前行情"
- "查看持仓"
- "查看风险"
- "新建交易意图"

快捷指令仅输入文本，不直接发起交易。

## 3. 开仓/加仓确认 UX 设计

### 3.1 确认卡生命周期

确认卡从 `TradeIntent` 通过 Go 风控计算后生成 `ConfirmationTicket`，经历以下生命周期：

```
DRAFT → AWAITING_CONFIRMATION → EXPIRED（超时未确认）
                              → CONFIRMED → RISK_REVALIDATING → ADMITTED → ...
                                           → REJECTED
```

确认卡在 `AWAITING_CONFIRMATION` 状态时呈现给用户。

### 3.2 确认卡组件结构

确认卡是一个独立的结构化 React 组件，不接受 Hermes Markdown 作为字段输入。
所有字段来自服务端 `ConfirmationTicket` JSON，通过 WebSocket 或 REST 下发。

**字段布局（从上到下）：**

```
┌──────────────────────────────────────────────┐
│ 🔴 MAINNET                    确认卡 #a3f2…b1 │
│                                              │
│ 环境与账户                                    │
│   HYPERLIQUID_MAINNET  │  账户: 0x…abc       │
│                                              │
│ ──────────────────────────────────────────── │
│                                              │
│ 交易动作                                      │
│   BTC-PERP  │  买入  │  开仓                  │
│                                              │
│ 订单                                         │
│   市价 IOC  │  全仓  │  杠杆 3x                │
│   数量: 0.1 BTC                              │
│   名义价值: ≈$4,200                           │
│   预计保证金: ≈$1,400                          │
│                                              │
│ 价格                                         │
│   预计入场: $42,000                           │
│   最差允许: $42,210 (0.5%)                    │
│   快照时间: 2026-07-29T13:30:00Z              │
│                                              │
│ ──────── 保护 ────────                       │
│                                              │
│   🛡 Stop Market 触发: $40,740              │
│      触发方式: 标记价格                        │
│      类型: reduce-only                       │
│   可选止盈: $43,260 (3%)                     │
│   预计强平价: $38,220                         │
│                                              │
│ ──────── 风险 ────────                       │
│                                              │
│   本单预计最大亏损: $126                       │
│   交易前账户总风险: $350 / 上限 $2,000         │
│   交易后账户总风险: $476 / 上限 $2,000         │
│   预计手续费: $4.20                            │
│   预计资金费率: $0.42/8h                       │
│   预计滑点: $8.40                              │
│   风控结果: ✅ 通过                            │
│                                              │
│ ──────── 解释 ────────                       │
│                                              │
│   用户指令: "BTC 42000 附近试多 0.1 BTC"      │
│   Hermes: "4h 趋势向上，建议轻仓试多"          │
│   用户理由: "趋势跟随"                         │
│                                              │
│ ──────────────────────────────────────────── │
│                                              │
│   ⏱ 有效期: 01:28                            │
│   风险版本: risk-v2.3.1                       │
│   确认哈希: a3f2...b1                         │
│                                              │
│ ┌──────────────────────────────────────┐     │
│ │     确认实盘开多 BTC                   │     │
│ └──────────────────────────────────────┘     │
│ [返回修改]                                   │
└──────────────────────────────────────────────┘
```

### 3.3 视觉层级

| 层级 | 内容 | 字号/权重 |
| --- | --- | --- |
| **第1层** | `MAINNET`、动作（开多/开空）、品种（BTC/ETH/SOL）、方向 | 最大/最重 |
| **第2层** | 最大亏损、交易后总风险、Stop Market 触发价、强平价 | 次大/中重 |
| **第3层** | 数量、杠杆、价格、费用 | 正常 |
| **第4层** | 解释、版本、哈希、有效期 | 最小/最轻 |

**规则：**
- 潜在盈利不作为确认卡的主要视觉焦点。
- 最大亏损、总风险和止损的视觉权重高于入场价和名义价值。
- 保护信息（Stop Market）用独立分区和🛡图标突出。

### 3.4 确认按钮规范

**主按钮文案必须包含：**
- 动作词：`确认`
- 环境：`实盘` 或 `模拟`
- 动作方向：`开多` `开空` `加多` `加空`
- 品种：`BTC` `ETH` `SOL`

示例：`确认实盘开多 BTC` `确认模拟加空 ETH`

**辅助按钮：**
- `返回修改` — 回到聊天，废弃当前确认对象
- 无 `取消` 按钮（确认卡有有效期，用户不操作自然过期）

**按钮行为：**
- 点击后立即锁定（disabled + loading 状态），文案变为 "正在提交确认…"
- 窗口切换、页面刷新或客户端重连不会恢复已锁定的按钮
- 收到服务端拒绝/失效后，不恢复旧确认按钮，显示"确认已失效，需要重新生成"
- 确认过期、风险版本变化、市场快照过期或账户变化后，按钮变为 `重新生成确认卡` 并跳回补问流程

### 3.5 确认后的执行进度

确认提交后，聊天面板显示结构化 Operation 进度跟踪器：

```
Operation #op-uuid
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 ✅ 已提交确认                         00:00
 ⏳ 正在重新验证风险                    00:02
 ⬜ 正在派发                            —
 ⬜ 等待交易所结果                       —
 ⬜ 正在核对订单与成交                    —
 ⬜ 正在建立保护                         —
 ⬜ 持仓已保护                           —
```

每步完成后填充 ✅。任一步失败后显示 ❌ 并展示失败原因和后续操作。

### 3.6 超时与取消

**确认卡有效期：**
- 由服务端 `ConfirmationTicket.expires_at` 决定。
- 前端显示实时倒计时。
- 倒计时 < 30 秒时，倒计时数字变为橙色警示。
- 倒计时 < 10 秒时，变为红色闪烁（每 2 秒一次，符合 `prefers-reduced-motion` 时使用静态红色）。
- 到期后按钮立即禁用，文案变为 "确认已过期 — 市场快照为 2026-07-29T13:30:00Z"。

**过期处理：**
- 确认卡保留显示但所有操作禁用。
- 提供 `重新生成确认卡` 按钮，触发完整的重新获取市场、账户和风险流程。
- 不得复用过期确认卡的任何字段。

**取消（隐含）：**
- 用户在有效期内不操作，确认自动过期。
- 用户在聊天中输入新指令（如"取消"或开始新的交易讨论）时：
  - 当前确认卡显示为灰色已过期状态。
  - 但聊天记录中保留确认卡的历史记录，标记 `已过期未确认`。

### 3.7 风险版本和快照变化时的失效

以下变化导致当前有效确认卡立即失效：
- 用户的风险配置版本号变化
- 市场快照 ID 变化（行情更新）
- 账户状态变化（余额、已有仓位变动）

失效后：
- 确认卡状态变为 `确认已失效 — <原因>`。
- 按钮变为 `重新生成确认卡`。
- 原因需明确说明，如 "风险配置已更新至 risk-v2.3.2" 或 "市场快照已过期"。

## 4. Stop Market 保护可见性

### 4.1 在确认卡中

Stop Market 在确认卡中以独立分区显示，包含：
- 🛡 保护图标
- 触发价（标记价格类型）
- reduce-only 标记
- 预计强平价（用于对照）
- 如果 Stop Market 未通过验证，确认卡不得生成

### 4.2 在持仓详情中

每个持仓显示保护覆盖状态：

```
BTC-PERP 仓位 #pos-uuid
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
数量: 0.1 BTC  │  方向: 多
入场价: $42,000  │  标记价: $41,800

🛡 保护状态: PROTECTED
   Stop Market #sm-uuid-1
   触发价: $40,740 (标记价格)
   覆盖数量: 0.1 BTC ← 必须与持仓数量一致
```

### 4.3 保护状态与 UX 映射

| 保护状态 | 视觉表现 | 用户可操作 |
| --- | --- | --- |
| `NO_POSITION` | 无 | 无 |
| `ENTRY_PENDING` | 黄色 "等待入场" | 可撤销入场订单 |
| `PARTIALLY_FILLED` | 黄色 "部分成交 0.05/0.1 BTC"，突出已成交量 | 可撤销未成交部分 |
| `FILLED` | 蓝色 "已成交，正在建立保护" | 等待 |
| `PROTECTION_PENDING` | **红色闪烁边框 + "止损尚未核对"** | 不可开新仓 |
| `PROTECTED` | 绿色 🛡 "已保护" + Stop Market 详情 | 可管理仓位 |
| `ADJUSTING_PROTECTION` | 黄色 "正在调整保护" + 保留旧 Stop Market 显示 | 不可开新仓 |
| `PROTECTION_FAILED` | **红色最高告警 + 不可静默** | 仅可紧急退出 |
| `EMERGENCY_CLOSING` | 红色 "系统正在降低风险" | 不要求确认 |
| `MANUAL_RECONCILIATION` | 红色 "需人工核对" + Operation ID | 查看指引 |
| `CLOSED` | 灰色 "已平仓" | 无 |

**关键规则：**
- 只有服务端确认 `PROTECTED` 后，UI 才显示 "持仓已保护"。
- `PROTECTION_FAILED` 触发不可静默的 Critical 告警。
- 止损调整中（`ADJUSTING_PROTECTION`），保留旧止损显示直到新止损核对完成。
- 如果新止损建立失败且旧止损仍然有效，恢复显示旧止损并标记 `PROTECTED`。
- 如果新止损建立失败且旧止损也已失效 → `PROTECTION_FAILED` → 紧急退出。

### 4.4 Stop Market 与 `reduce-only` 紧急退出

当 `PROTECTION_FAILED` 发生：
1. 全局 Critical 告警启动，不可静默。
2. 告警文案："持仓保护失败 — 系统正在执行 reduce-only 紧急退出。请勿手动操作。"
3. 系统自动下发 `reduce-only` 市价单。
4. 不要求用户确认（这是安全降级路径，不是风险增加动作）。
5. 退出完成后进入 `MANUAL_RECONCILIATION` 或 `CLOSED`。

前端不提供 "全部平仓" 按钮。紧急退出路径是系统自动触发的唯一风险出口。

## 5. 幂等与重复提交防护

### 5.1 客户端防护

**提交时锁定：**
- 确认按钮点击后立即进入 `submitting` 状态：
  - 按钮禁用。
  - 显示 loading 指示器。
  - 文案变为 "正在提交确认…"。
- 页面刷新、窗口切换或 WebSocket 重连不恢复按钮状态。

**客户端去重：**
- 每个确认对象携带唯一的 `confirmation_nonce`（32–128 字符）。
- 客户端在提交确认时携带此 nonce。
- 服务端确认级去重依赖计划中的原子确认消费（compare-and-consume，详见第 6 节），以 `confirmation_id`（UUID）为键。
  - **当前状态：NOT_IMPLEMENTED / NOT_EXECUTED**。现有 `IdempotencyStore`（`services/trading-core/domain/idempotency.go`）仅提供基于 `client_order_id` 的订单级幂等去重，不包含 confirmation_id 键，也未实现 compare-and-consume 语义。
  - 客户端 UI 不得假定服务端已实现确认级去重；在该路径实现并验证前，客户端只能展示确认状态和提交请求，执行保持 fail-closed。
- 计划语义（NOT_IMPLEMENTED）：当原子确认消费实现后，如果客户端因网络问题重发，服务端返回 `EXISTING_RESULT` 而非创建新 Operation。

**乐观更新禁令：**
- 客户端不得在收到服务端确认前显示 "已成交"、创建假订单或显示假保护状态。
- 所有状态必须来自服务端 WebSocket 推送或 REST 轮询结果。

### 5.2 重复确认防护

用户可能通过以下路径尝试重复确认：
1. 在同一确认卡上多次点击按钮 → **已被按钮锁定阻止**
2. 在多个设备/标签页打开同一确认卡 → **计划通过服务端 confirmation_id compare-and-consume 幂等去重实现（NOT_IMPLEMENTED / NOT_EXECUTED）**。当前客户端不得依赖此服务端去重；现有 `IdempotencyStore` 仅基于 client_order_id 去重。
3. 网络断开后重连，UI 恢复时重新点击 → **按钮状态由服务端 state 驱动，不依赖客户端内存（依赖 compare-and-consume 持久化状态，NOT_IMPLEMENTED / NOT_EXECUTED）**。

### 5.3 重复订单防护

- 每个 `client_order_id` (cloid) 在 `IdempotencyStore` 中以 `ClientOrderID` 类型存储。
- 如果 cloid 已存在且对应不同结果，返回 `ErrIdentifierConflict` → Operation 进入 `MANUAL_RECONCILIATION`。
- 如果 cloid 已存在且结果相同，返回 `EXISTING_RESULT` → Operation 正常结束。

### 5.4 客户端断线重连

1. WebSocket 断开时：
   - 所有数据标记为 `STALE`。
   - 交易确认入口禁用。
   - 聊天框顶部显示 "连接已断开 — 数据可能已过期"。
2. 重连后：
   - 进入 `RECONCILING` 状态。
   - 等待服务端推送完整一致性检查结果。
   - 只有服务端确认一致性通过后恢复 `LIVE`。
   - 在此期间禁止任何增加风险的操作。

## 6. 服务端原子确认比较并消费（Compare-and-Consume）语义

### 6.1 确认绑定上下文

每次确认消费必须在服务端原子绑定并比较以下上下文维度：

| 绑定维度 | 字段 | 验证方式 |
| --- | --- | --- |
| 认证用户 | `authenticated_user` | 比较当前认证用户 ID 与确认票创建时绑定的用户 ID |
| 账户 | `account` | 比较当前交易所账户标识与确认票绑定的账户 |
| 设备 | `device` | 比较当前客户端设备指纹与确认票创建时绑定的设备 ID |
| 会话 | `session` | 比较当前服务端会话 ID 与确认票创建时绑定的会话 ID |
| 目的 | `purpose` | 比较确认意图类型（`OPEN` / `ADD` / `REDUCE`） |
| intent_hash | `intent_hash` | 比较当前提交的 intent_hash 与确认票创建时的原始 intent_hash |

**绑定规则：**
- **任一项不匹配** → 消费失败，返回 `CONFIRMATION_CONTEXT_MISMATCH`，失败原因需明确指出不匹配的具体维度（如 `device_mismatch`）。
- **上下文比较必须在消费写入之前完成** — 在同一个原子事务或 compare-and-swap 中执行比较和写入。
- **任一维度无法获取时** — 使用显式 `UNKNOWN` 值参与比较。仅当创建时和消费时双方都为 `UNKNOWN` 时，该维度视为匹配。

### 6.2 原子比较并消费操作（Compare-and-Consume）

确认消费是严格的一次性操作。服务端必须实现以下原子语义：

```
COMPARE-AND-CONSUME(confirmation_id, nonce, context):
  1. 开始原子事务或 CAS 锁
  2. 查询 confirmation_id 在持久存储中的状态
     a. 不存在 → 返回 CONFIRMATION_NOT_FOUND
     b. 已消费 → 返回 EXISTING_RESULT，携带最终结果引用和首次消费元数据
     c. 已过期 → 返回 CONFIRMATION_EXPIRED，携带过期时间戳
  3. 比较 context 全部 6 个绑定维度（见 6.1）
     a. 任一项不匹配 → 中止事务，返回 CONFIRMATION_CONTEXT_MISMATCH
  4. 原子写入消费记录：
     a. 状态：AWAITING_CONFIRMATION → CONSUMED
     b. 消费时间戳：记录服务端当前时间
     c. 消费设备/会话：记录本次消费的设备与会话
     d. nonce 绑定：将 nonce 写入 confirmation_nonces 表（唯一约束）
     e. 最终结果引用：若已有下游结果（如 Operation ID），写入引用
  5. 提交事务
  6. 返回 SUCCESS + 最终结果引用（如可用）
```

**关键约束：**
- **第 3 步（上下文比较）与第 4 步（原子写入）必须在同一个原子事务中完成。** 不允许比较通过后事务提交失败而留下部分状态。
- **如果第 4-5 步执行但存储层返回不确定状态**（如事务提交超时、写入确认丢失）→ 进入 `UNKNOWN_REQUIRES_RECONCILIATION`（详见第 7 节）。
- **第 2b 步的 `EXISTING_RESULT`** 必须携带消费时持久化的最终结果，不得基于实时状态重新推断。

### 6.3 confirmation_id 与 nonce 的持久唯一性

| 属性 | confirmation_id | nonce |
| --- | --- | --- |
| 生成者 | 服务端（在创建确认票时） | 客户端（前端随机生成） |
| 格式 | UUID v4 | 32-128 字符随机字符串 |
| 唯一性约束 | 主键（`confirmation_records` 表） | 唯一索引（`confirmation_nonces` 表） |
| 生命周期 | 创建后永久保留，不可删除 | 消费后永久绑定到 confirmation_id |
| 可重用性 | 不可重用 — 已消费/已过期后不可重新激活 | 不可重用 — 重放立即被检测并拒绝 |
| 跨设备验证 | 通过设备绑定比较（见 6.1） | 不独立验证（通过 confirmation_id 间接绑定） |

**nonce 重放检测：**
- nonce 在 `confirmation_nonces` 表中具有唯一约束。
- 客户端提交已存在的 nonce → 服务端返回 `NONCE_REPLAY_DETECTED`。
- 检测到 nonce 重放时，不执行任何上下文比较或消费写入。
- nonce 一旦被消费，永久绑定到其 confirmation_id 和最终结果，不可覆盖。

**confirmation_id 生命周期：**
- 状态转换：`AWAITING_CONFIRMATION` → `EXPIRED`（超时）或 `CONSUMED`（用户确认）。
- 已 `EXPIRED` 的 confirmation_id 不可被消费。
- 已 `CONSUMED` 的 confirmation_id 不可重新消费。
- 所有状态转换记录在持久存储中，包含时间戳和原因。

### 6.4 失败关闭场景

所有以下场景必须 fail closed（拒绝消费并记录）：

| 场景 | 检测条件 | 返回错误码 | 是否触发 UNKNOWN |
| --- | --- | --- | --- |
| 跨设备使用 | 消费 device ≠ 创建 device | `CONFIRMATION_CONTEXT_MISMATCH` (device) | 否 |
| 确认已过期 | `expires_at < now()` | `CONFIRMATION_EXPIRED` | 否 |
| nonce 重放 | nonce 已存在于 `confirmation_nonces` | `NONCE_REPLAY_DETECTED` | 否 |
| 身份不匹配 | 消费 user ≠ 创建 user | `CONFIRMATION_CONTEXT_MISMATCH` (user) | 否 |
| 账户不匹配 | 消费 account ≠ 创建 account | `CONFIRMATION_CONTEXT_MISMATCH` (account) | 否 |
| 会话不匹配 | 消费 session ≠ 创建 session | `CONFIRMATION_CONTEXT_MISMATCH` (session) | 否 |
| 目的不匹配 | 消费 purpose ≠ 创建 purpose | `CONFIRMATION_CONTEXT_MISMATCH` (purpose) | 否 |
| intent_hash 不匹配 | 消费 intent_hash ≠ 创建 intent_hash | `CONFIRMATION_CONTEXT_MISMATCH` (intent) | 否 |
| confirmation_id 不存在 | 查询返回空行 | `CONFIRMATION_NOT_FOUND` | 否 |
| 并发竞争消费 | CAS 冲突（另一消费先完成） | `EXISTING_RESULT`（携带首次消费结果） | 否 |
| 持久化结果未知 | 存储写入后返回不确定性 | `CONFIRMATION_PERSISTENCE_UNKNOWN` | 是 → 第 7 节 |

**并发消费冲突处理：**
- 使用 CAS（compare-and-swap）或数据库行级锁确保同一 confirmation_id 仅被消费一次。
- 第二个并发消费收到 `EXISTING_RESULT`，携带首次消费的 nonce、时间戳和最终结果。
- 第二个请求的 nonce 不被写入（不重复插入确认记录），客户端可安全重试。

**持久化结果未知处理：**
- 当存储层在写入后返回不确定状态（网络超时但数据可能已写入、数据库故障转移）时：
  - 服务端进入 `UNKNOWN` 状态。
  - **不得假定消费成功。**
  - **不得假定消费失败并建议重试。**
  - 必须触发 `UNKNOWN_REQUIRES_RECONCILIATION` 流程（第 7 节）。
  - 后续查询必须从持久存储中查找实际记录，不得从内存缓存或会话状态推断。

### 6.5 与现有 IdempotencyStore 的关系

**重要声明——如实记录：**

- 现有 `IdempotencyStore`（`services/trading-core/domain/idempotency.go`）仅提供基于 `client_order_id`（cloid）的**订单级**幂等去重。
- 该存储**不包含** confirmation_id 相关的键或数据结构。
- 该存储**未实现** compare-and-consume（比较并消费）语义。
- 本节描述的原子确认消费语义需要以下独立数据结构和方法：

| 组件 | 说明 | 状态 |
| --- | --- | --- |
| `ConfirmationRecord` | 持久化记录：confirmation_id, user, account, device, session, purpose, intent_hash, status, nonce, consumed_at, result_reference | **NOT_IMPLEMENTED** |
| `ConfirmationNonce` | nonce 唯一索引表 | **NOT_IMPLEMENTED** |
| `CompareAndConsume()` | 事务方法：实现 6.2 中描述的原子比较并消费 | **NOT_IMPLEMENTED** |
| 确认消费存储接口 | 持久化层的抽象接口 | **NOT_IMPLEMENTED** |

**运行时状态：**
```
NOT_IMPLEMENTED — 服务端 compare-and-consume 路径尚未实现
NOT_EXECUTED   — 未在任何环境中运行或测试
```

当前确认流程仅为 UX 设计规范（本文档第 3 节）。服务端原子确认消费的实现属于后续开发阶段，不在本次交付范围内。

### 6.6 安全边界保持

本原子确认消费语义的增加**不改变**以下安全边界：

| 安全边界 | 保持状态 |
| --- | --- |
| 人工确认强制性 | ✅ 保持 — 原子消费在用户点击确认按钮后触发，不可被 Hermes 文本自动触发 |
| Stop Market 保护 | ✅ 保持 — 确认消费不替代或绕过止损保护要求 |
| Hermes 无直接授权 | ✅ 保持 — 确认消费是服务端操作，Hermes 仅作为 UX 通道 |
| 实盘白名单（BTC/ETH/SOL） | ✅ 保持 — 非白名单品种不生成确认票 |
| 禁止乐观更新 | ✅ 保持 — 所有状态来自服务端持久存储 |

## 7. UNKNOWN_REQUIRES_RECONCILIATION UX 设计

### 6.1 触发场景

`UNKNOWN_REQUIRES_RECONCILIATION` 出现在以下情况（详见 `services/trading-core/domain/scenarios.go`）：
- `dispatch_timeout_after_send` — 派发请求已发出但未收到响应
- `executor_crash_after_exchange_success` — 执行器确认交易所成功后崩溃
- `executor_crash_after_submit_before_record` — 提交后、记录前崩溃
- 任何无法确定交易所是否收到、处理或拒绝的请求

### 6.2 UX 表现

**全局层面：**
- 顶部状态栏永久显示红色告警指示器。
- Critical 告警横幅：`⚠️ 订单结果未知 — 正在核对真实状态`
- 通知推送：Email + iOS Push。

**操作详情页：**
```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
⚠️ 订单结果未知 — 正在查找真实订单
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Operation: #op-uuid
意图: BTC-PERP 开多 0.1 BTC @ $42,000
cloid: 0xa1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6

最近核对: 2026-07-29T13:30:15Z
核对状态: PENDING

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
我们正在查询交易所的真实订单状态。
这个过程可能需要几分钟。

请勿：
❌ 重新下单
❌ 手动开仓
❌ 关闭应用或断开连接

您可以：
✅ 查看事件时间线
✅ 等待自动核对完成
```

**强制约束：**
- **不显示 "失败，请重试" 文案。** 文案必须使用 "结果未知，正在查询真实订单"。
- **不提供 "再次下单" 按钮或快捷操作。** 重复尝试可能导致重复成交。
- 显示 Operation ID 和 cloid/oid（如果存在）。
- 显示最近核对时间和核对状态。

### 6.3 UNKNOWN → RESOLVED 转换

核对完成后，状态转为以下之一：

| 核对结果 | 新状态 | UX 表现 |
| --- | --- | --- |
| 交易所确认订单存在且状态明确 | `ACKNOWLEDGED` → `FINAL` | 正常显示成交/拒绝结果 |
| 交易所确认订单被拒绝 | `REJECTED` → `FINAL` | 显示拒绝原因 |
| 交易所找不到订单 | `MANUAL_RECONCILIATION` | 显示处理指引 |
| 系统恢复后查到已有结果 | `ACKNOWLEDGED` → `FINAL` | 从持久存储恢复 |
| 核对过程本身超时 | 保持 `UNKNOWN`，增加核对计数 | 更新核对时间，等待下次核对 |

### 6.4 MANUAL_RECONCILIATION 状态

当自动核对无法解决时：

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
🔴 需要人工核对
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Operation: #op-uuid
意图: BTC-PERP 开多 0.1 BTC @ $42,000
cloid: 0xa1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6

自动核对未能在交易所找到对应订单。

处理指引：
1. 打开 Hyperliquid 官网查看您的活动订单和成交记录。
2. 查找 cloid: 0xa1b2c3…c5d6
3. 在下方选择核对结果：

┌─────────────────────────────────────┐
│ ○ 订单已成交 — 确认持仓             │
│ ○ 订单未成交 — 标记为未执行         │
│ ○ 我无法确定 — 保持告警             │
└─────────────────────────────────────┘
[提交核对结果]
```

保留完整事件时间线，不覆盖原始异常事实。

## 8. 开发阶段模拟确认 UX

在开发阶段，确认 UX 使用模拟模式，但必须与实盘模式的交互流程完全一致：

- 确认卡布局和字段与实盘一致。
- 按钮文案使用 "确认模拟开多 BTC"。
- 模拟成交使用模拟 Order ID（`SIMULATED_ORDER_` 前缀）。
- 模拟保护使用与服务端相同状态机的模拟保护状态。
- 模拟 `UNKNOWN_REQUIRES_RECONCILIATION` 场景可通过 `dispatch_timeout_after_send` 场景触发。

**模拟数据可见标记：**
- 确认卡顶部环境标记为橙色 `SIMULATION`。
- 所有价格/数量数值旁有橙色竖线指示条。
- 持仓表格中 `类型` 列显示 `模拟`。

## 9. 设计决策记录

| ID | 决策 | 依据 |
| --- | --- | --- |
| H3-D-001 | 确认卡为独立结构化组件，不从 Hermes Markdown 生成 | D-009/D-010/D-011，安全边界 |
| H3-D-002 | Stop Market 在确认卡中使用独立分区 | D-020，保护可见性 |
| H3-D-003 | 确认按钮文案包含动作、环境和品种 | 防误操作，UX-ACC-008 |
| H3-D-004 | UNKNOWN 状态不提供 "重试" 按钮 | D-021，防止重复成交 |
| H3-D-005 | 模拟数据使用橙色指示条而非仅文字标记 | 视觉安全，防止将模拟误认为实盘 |
| H3-D-006 | 补问卡片为结构化表单，Hermes 不自行假设 | D-014/D-025/D-026，安全与用户体验 |
| H3-D-007 | 确认过期后不可复用，必须重新生成完整流程 | UX-ACC-008，市场数据时效性 |
| H3-D-008 | 客户端乐观更新被禁止，所有状态来自服务端 | 安全关键规则，服务端权威 |
| H3-D-009 | Kill Switch 不提供 "全部平仓"，紧急退出路径是系统触发的 reduce-only | D-056，降低风险的唯一出口 |
| H3-D-010 | 非 BTC/ETH/SOL 确认卡在实盘模式不生成 | D-004，实盘白名单 |
| H3-D-011 | 确认消费绑定 6 个上下文维度（用户、账户、设备、会话、目的、intent_hash） | 防跨设备/跨身份误操作，安全关键 |
| H3-D-012 | 确认消费为严格一次性原子操作（compare-and-consume） | 防止并发消费、重复确认 |
| H3-D-013 | confirmation_id 和 nonce 全局持久唯一，永久绑定最终结果 | 防重放攻击，审计完整性 |
| H3-D-014 | 所有确认消费失败关闭（fail closed） | D-021/D-022，系统安全原则 |
| H3-D-015 | 持久化结果未知时进入 UNKNOWN_REQUIRES_RECONCILIATION | D-021，防止错误假设 |
| H3-D-016 | 现有 IdempotencyStore 不包含 compare-and-consume 实现，需独立开发 | 如实声明，避免错误假设已被实现 |

## 10. 与其他设计的接口

| 依赖 | 引用 |
| --- | --- |
| 确认卡字段定义 | `services/trading-core/domain/types.go:ConfirmationTicket` |
| Operation 状态机 | `services/trading-core/domain/state.go:OperationState` |
| 保护状态机 | `services/trading-core/domain/state.go:ProtectionState` |
| 幂等存储 | `services/trading-core/domain/idempotency.go:IdempotencyStore` |
| 故障场景 | `services/trading-core/domain/scenarios.go:frozenScenarioResults` |
| 前端 UX 规范 | `docs/06_FRONTEND_UX_SPEC.md` |
| 多代理工作流 | `docs/07_MULTI_AGENT_WORKFLOW.md` |
| 决策记录 | `docs/05_DECISION_REGISTER.md` |
| Hermes 确认哈希 | `services/hermes-agent/src/hermes_agent/confirmation.py` |
| Hermes 合约模型 | `services/hermes-agent/src/hermes_agent/contracts.py` |
| 确认消费记录（未实现） | `services/trading-core/domain/confirmation_consume.go`（新文件，NOT_IMPLEMENTED） |
| 确认 nonce 索引（未实现） | `services/trading-core/domain/confirmation_nonce.go`（新文件，NOT_IMPLEMENTED） |

## 11. 授权状态

```
MERGE=false
DEPLOY=false
PRODUCTION=false
LIVE_TRADING=false
```

本设计文档仅定义 UX 交互规范，不授权任何交易执行、部署或生产操作。
