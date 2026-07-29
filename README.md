# Hyperliquid Hermes 个人交易系统

这是一个面向个人使用、但保留未来多用户扩展边界的加密货币永续合约交易系统。

系统以 Hyperliquid 为第一家交易所，以 Hermes 为交易 Agent。用户通过网页、桌面软件和后续的 iOS App 内置聊天指挥 Hermes；所有增加风险的交易必须经过结构化确认、确定性风控和独立签名服务，Hermes 不接触交易私钥。

## 当前状态

- 产品需求讨论已形成第一版基线。
- 技术架构已选定；机构风控台方向的网页/桌面模拟原型已通过固定提交独立审查并推送功能分支。
- Phase 0 主契约已经进入 `main`；Phase 1 平台安全契约已通过独立审查并推送功能分支，尚未合并。
- 交易核心运行时、真实交易所接入、Hermes 运行时、钱包签名和生产服务尚未开始实现。
- 没有部署、生产启用或自动实盘授权。
- 第一阶段目标是个人版运行、多用户数据模型。

## 文档

- [文档索引](docs/00_INDEX.md)
- [完整产品需求](docs/01_PRODUCT_REQUIREMENTS.md)
- [系统架构](docs/02_SYSTEM_ARCHITECTURE.md)
- [功能与验收标准](docs/03_FUNCTIONAL_ACCEPTANCE.md)
- [开发阶段计划](docs/04_DEVELOPMENT_PLAN.md)
- [决策记录](docs/05_DECISION_REGISTER.md)
- [前端 UX 设计规范](docs/06_FRONTEND_UX_SPEC.md)
- [多代理开发工作流](docs/07_MULTI_AGENT_WORKFLOW.md)
- [项目代码地图](docs/CODE_MAP.md)
- [当前项目进度](docs/CURRENT_STATUS.md)
- [桌面高保真原型](prototype/web-desktop/)

## 固定范围

- 交易所：Hyperliquid
- 产品：永续合约
- 实盘白名单：BTC、ETH、SOL
- Agent：Hermes
- 主要周期：1h、4h
- 客户端顺序：网页/桌面优先，iOS 后续
- 初始部署：单台高质量 Linux 服务器与远程备份

## 重要边界

- 主钱包不连接系统；系统只连接资金受限的独立交易钱包。
- Hermes、客户端、Go 交易核心和数据库均不能读取签名私钥。
- 开仓、加仓和其他增加风险的动作必须确认。
- 止损是所有仓位的强制保护；保护失败时允许自动 `reduce-only` 紧急平仓。
- 自动交易必须绑定已批准的模型版本、风险策略和授权范围。
- 文档完成不代表允许部署、生产启用或自动实盘。
