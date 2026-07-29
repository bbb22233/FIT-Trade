# 多代理开发工作流

## 1. 目标

使用当前 Codex、本机 Hermes 和服务器 Codex 并行开发，同时保持共享接口、代码所有权、安全边界和验收结果可追溯。

速度来自互不重叠的并行工作，不来自多个 Agent 同时修改同一文件。

## 2. 角色与所有权

| 角色 | 主要职责 | 固定所有权 |
| --- | --- | --- |
| 当前 Codex | 架构、任务拆分、共享契约、前端、集成和验收 | `contracts/`、`apps/web/`、`docs/`、根目录共享文件 |
| 本机 Hermes 开发 Profile | Agent、模型路由、提示词、意图翻译和评估场景 | `services/hermes-agent/` |
| 服务器 Codex 开发账户 | Go 核心、Hyperliquid 适配器、PostgreSQL、NATS 和 Linux 验证 | `services/trading-core/`、`services/hyperliquid-adapter/`、`infra/` |

共享契约和根目录高耦合文件只有当前 Codex 可以修改。其他 Agent 只能在交付说明中提出修改建议。

## 3. 并行开发循环

1. 当前 Codex 冻结任务使用的接口版本和基线 commit。
2. 为每个 Agent 创建独立 branch、worktree 和任务包。
3. 两个执行 Agent 在互不重叠的目录并行开发。
4. 每个 Agent 返回固定 commit、测试结果、已知限制和依赖关系。
5. 当前 Codex 先做机械范围检查，再做契约、安全和故障路径审查。
6. 按依赖顺序集成，通过完整验证后才进入下一批任务。

## 4. 首批共享契约

正式并行编码前先冻结：

- `TradeIntent`
- `RiskDecision`
- `ConfirmationTicket`
- `ExecutionCommand`
- `ExecutionResult`
- `PositionSnapshot`
- `ProtectionStatus`
- `ReconciliationStatus`
- `AgentFeedback`
- `AutomationAuthorization`

这些契约必须包含稳定 ID、版本、生成时间、过期时间、账户、交易环境和幂等键。

## 5. 任务交付标准

每个任务必须提交：

- 唯一 Task ID
- 基线和交付 commit
- 允许修改的文件范围
- 实际修改文件
- 单元、契约、集成和故障路径测试
- 未解决风险
- 集成依赖与顺序
- 明确的 `MERGE/DEPLOY/PRODUCTION AUTHORIZED = NO`

任务模板位于 [`coordination/TASK_TEMPLATE.md`](../coordination/TASK_TEMPLATE.md)。

## 6. 安全隔离

- Hermes 使用专门的开发 Profile，不与未来实盘 Profile 共用学习数据、会话或权限。
- 服务器 Codex 使用专门的开发目录和账户权限，不直接操作未来生产服务。
- 密钥只保存在各自主机的受限 Secret 文件中，不进入 Git、任务提示或聊天记录。
- 开发阶段禁止交易所写操作、签名和自动交易。
- 合并、部署和实盘启用是三个独立授权边界。

## 7. 第一阶段执行顺序

1. 建立 Git 基线、远程仓库和三套 worktree。
2. 建立 monorepo 目录与共享契约包。
3. 当前 Codex完成前端壳和契约客户端。
4. Hermes 完成受控 Agent 服务骨架。
5. 服务器 Codex 完成 Go 核心、只读 Hyperliquid 适配器和基础设施骨架。
6. 完成端到端模拟链路：聊天指令 → 结构化意图 → 风控 → 确认票据 → 模拟执行 → 核对 → 保护状态。
7. 通过故障注入和独立审查后，再讨论任何小额主网验证。
