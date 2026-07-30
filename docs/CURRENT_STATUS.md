# 当前项目进度

更新日期：2026-07-29
证据截止时间（as-of）：`2026-07-29T21:24:11-07:00`
状态快照：`codex/round-4-progress-docs`（父提交
`codex/round-3-progress-docs@e63704675b11d045be317339b4e544a246ccbc42`）

## 1. 结论

本轮 P1-003 auth core 固定对象
`a3108988463e9b373b8263fb3f060f8908c54c82`（parent
`19b778785565d31dc095996ce4d4b0f7d58bebdf`）已获 reviewer-7 独立 fixed-commit review
`P0/P1/P2 = 0/0/0`、`PASS`。`origin` 的 `codex/p1-003-auth-core` 已推送并在本快照以
远端 ref 核验到同一 40 位对象；未合并。该接受结论满足 P1-004 的 P1-003 接受前置，
但 P1-004 R6 截至本快照为 `DEPENDENCY_BLOCKED/NOT_STARTED`：目标分支
`codex/p1-004-postgres-runtime-r6`、工作树
`/root/fit-trade-dev/worktrees/p1-004-postgres-runtime-r6` 均 `ABSENT`，writer 未启动，
旧 `a41960ff98a8b3be7b82e31f06c89ccb085c2593` 未触碰。决定性 blocker 是已接受 P1-003
尚缺 OwnerMutation input/result 和 NATS consumer business-effect shared interfaces，architect
禁止 persistence 自行创造；第二 blocker 是仅 Docker 可用、disposable PostgreSQL 16 尚未实证。
root 正在安排共享接口补充与 Docker PostgreSQL 16 preflight；在可核验固定对象前均为 `PENDING`。
P1-005 仍等待已接受的 P1-004 receipt gate。AI Agent / Hermes 分支的已接受、已推送状态保持不变。

这不是 Phase 退出、合并、部署、生产、钱包连接或真实交易授权。

## 2. 固定对象登记

| 工作项 | 固定对象与分支 | 独立审查结论 | Git 状态 | 当前判定 |
| --- | --- | --- | --- | --- |
| P1-001 platform contracts | `9b4b113a4032f7f0eee88297781e3b2d069b1f18`；`codex/p1-platform-contracts` | `P0/P1/P2 = 0/0/0`，`PASS` | 已推送，未合并 | Phase 1 共享契约与失败语义；不是运行时 |
| P1-002 Go contract client | `19b778785565d31dc095996ce4d4b0f7d58bebdf`；`codex/p1-go-contract-client` | `P0/P1/P2 = 0/0/0`，`PASS` | 已推送，未合并 | 只读、严格的 Phase 1 契约消费端；不是 API、DB、NATS 或交易运行时 |
| P1 interface capsule | `f61474ba4f96624f34ff478bd0047c28e72becab`；`codex/p1-interface-capsule` | security 与 DBMSG 两次独立审查均 `PASS` | 已推送，未合并 | 实现接口和依赖顺序已冻结；不产生运行时能力 |
| P1-003 auth 设计 | `12d698181ae88cadbfaf42a9dce6107585c432b7`；`codex/p1-003-design-r2` | `PASS` | 已推送，未合并 | 认证、会话、设备和 owner scope 的实现前设计 |
| P1-004 PostgreSQL 设计 | `264d4e943565a74039d0b74583bb0404b695d378`；`codex/p1-004-postgres-design-r2` | `PASS` | 已推送，未合并 | 持久化、事务、Inbox/Outbox 的实现前设计 |
| P1-005 JetStream 设计 | `4589bed3f7f9287751a5c57db311a1373597a67f`；`codex/p1-005-jetstream-design` | `PASS` | 已推送，未合并 | 事件投递设计；不是已运行的 NATS/JetStream |
| AI Agent / Hermes | `367b86f94f855ddc7673997b0b455f5b0908f04f`；`codex/ai-agent-swarm-bootstrap` | 外部 fixed-commit review：`P0/P1/P2 = 0/0/0`，`PASS` | 已推送，未合并 | 已关闭该固定对象的 provenance review；仍不等于 Phase 3 runtime/security exit |
| P1-003 auth core 运行时 | `a3108988463e9b373b8263fb3f060f8908c54c82`；parent `19b778785565d31dc095996ce4d4b0f7d58bebdf`；`codex/p1-003-auth-core` | reviewer-7：`P0/P1/P2 = 0/0/0`，`PASS` | 已推送并以 `origin` 远端 ref 核验；未合并 | 已接受的 P1-003 固定对象；不授权 P1-004 acceptance、main merge 或发布 |
| P1-004 PostgreSQL runtime | R6 `DEPENDENCY_BLOCKED/NOT_STARTED`；`codex/p1-004-postgres-runtime-r6` 与 `/root/fit-trade-dev/worktrees/p1-004-postgres-runtime-r6` 均 `ABSENT` | 缺 OwnerMutation input/result、NATS consumer business-effect shared interfaces；architect 禁止 persistence 自造；且仅 Docker 可用、disposable PostgreSQL 16 未实证 | writer 未启动；旧 `a41960ff98a8b3be7b82e31f06c89ccb085c2593` 未触碰；无 R6 fixed object | 等待共享接口补充和 Docker PostgreSQL 16 preflight 的可核验证据；不得以 P1-003 `PASS` 代替 P1-004 acceptance |
| P1-005 JetStream runtime | 无运行时提交 | 未启动独立 runtime 审查 | 等待已接受的 P1-004 receipt gate | 不得越过持久 Outbox/receipt 依赖 |

`PASS` 只描述表中那个固定对象的独立审查范围。它不授权 merge、deploy、
production、wallet/signer、exchange write API、live order 或 automatic trading。

## 3. 进度口径：`PROGRESS_UNITS_V1`

本表替换上一版的“半个单元”加权估算。`PROGRESS_UNITS_V1` 是完整产品计划的
25 个等权、可审计退出单元：每行只能在满足本行“计入分子条件”后成为
`COMPLETED`，否则不计分。分子不因分支已推送、设计存在、绿测、工作树运行中或
口头状态增加；未合并的固定对象可以满足其自身的 design/contract 单元，但绝不
自动满足 runtime 或阶段退出单元。

来源映射：P0 行来自 `coordination/tasks/P0-001`—`P0-004` 与 `main` 固定历史；
P1 行来自 `codex/p1-platform-contracts` 中的 `coordination/tasks/P1-001`—`P1-007`
及第 2 节固定对象；P2—P9 行逐项来自 `docs/04_DEVELOPMENT_PLAN.md` 对应 Phase 的
“产物”与“退出条件”。Phase 9 原计划没有退出条件，故 V1 显式保留两个
`NOT_AUTHORIZED` 单元，且永不因文档存在而计分。

| 稳定 ID | 名称 | Phase | 当前状态 | 计入分子条件 | 固定证据或 pending 理由 |
| --- | --- | --- | --- | --- | --- |
| P0-U01 | 共享契约与安全语义 | P0 | COMPLETED | `P0-001` 的固定验收对象已在 `main`，且契约/状态/确认/最小 MCP 边界已冻结 | `e67430dc4b837ceb2d714a0b2d964eee29622de3` 是 `main@60850b6` 祖先；任务包 `P0-001` |
| P0-U02 | 跨语言 conformance 与 P0 exit review | P0 | PENDING | `P0-002`、`P0-003` 固定对象及 `P0-004` 的独立 `0/0/0 PASS` exit review 均存在 | `a93cd1b`、`0c6ec07` 已在 `main`，但没有可登记的 `P0-004` 固定 exit-review 证据；任务包 `P0-002`—`P0-004` |
| P1-U01 | 平台契约与失败语义 | P1 | COMPLETED | `P1-001` 固定对象独立审查 `P0/P1/P2=0/0/0 PASS` | `9b4b113a4032f7f0eee88297781e3b2d069b1f18`；第 2 节与 `P1-001` |
| P1-U02 | Go platform-contract consumer | P1 | COMPLETED | `P1-002` 固定对象独立审查 `P0/P1/P2=0/0/0 PASS` | `19b778785565d31dc095996ce4d4b0f7d58bebdf`；第 2 节与 `P1-002` |
| P1-U03 | 接口 capsule 与实现前设计包 | P1 | COMPLETED | interface capsule security+DBMSG 双 `PASS`，且 P1-003/004/005 设计对象各有 `PASS` | `f61474ba4f96624f34ff478bd0047c28e72becab`、`12d698181ae88cadbfaf42a9dce6107585c432b7`、`264d4e943565a74039d0b74583bb0404b695d378`、`4589bed3f7f9287751a5c57db311a1373597a67f` |
| P1-U04 | API、identity、session、ownership runtime | P1 | PENDING | `P1-003` 固定实现候选通过其 task-packet acceptance 并独立审查 | `a3108988463e9b373b8263fb3f060f8908c54c82` 获 reviewer-7 `0/0/0 PASS` 且已推送、未合并；本快照没有其 task-packet acceptance 完整证据，故不计分 |
| P1-U05 | PostgreSQL 原子持久化 runtime | P1 | PENDING | `P1-004` 在真实 disposable PostgreSQL 上通过 task-packet acceptance 并独立审查 | R6 为 `DEPENDENCY_BLOCKED/NOT_STARTED`：shared interfaces 缺失且 Docker PostgreSQL 16 未实证；无 fixed object、acceptance 或 review，故不计分 |
| P1-U06 | NATS/JetStream、Inbox/Outbox、observability runtime | P1 | PENDING | `P1-005` 固定候选通过投递、重放、去重、故障 acceptance 并独立审查 | 等待已接受的 P1-004 receipt gate；未启动运行时验收或审查 |
| P1-U07 | default-disabled recovery infrastructure 与 Phase 1 exit | P1 | PENDING | `P1-006` recovery acceptance 完成，且 `P1-007` 线性固定对象有独立 exit `PASS` | 无 P1-006/P1-007 候选；`P1-006`、`P1-007` |
| P2-U01 | Hyperliquid 只读 Connector 与 Go 投影 | P2 | PENDING | Phase 2 所列只读市场/账户同步、重连和投影有固定验收对象 | `docs/04_DEVELOPMENT_PLAN.md` §5.2；没有固定对象 |
| P2-U02 | Hyperliquid 只读一致性 exit | P2 | PENDING | Phase 2 五项退出条件均有固定验收与独立 review | `docs/04_DEVELOPMENT_PLAN.md` §5.3；没有固定对象 |
| P3-U01 | Hermes 与网页/桌面只读实现 | P3 | PENDING | Phase 3 产物有固定实现和 acceptance；AI 安全资料必须通过 provenance review | AI Agent `367b86f` 已获 provenance fixed-review `0/0/0 PASS`，但 Phase 3 的 Hermes 与网页/桌面完整只读产物/acceptance 尚未齐备；`docs/04_DEVELOPMENT_PLAN.md` §6.2 |
| P3-U02 | Hermes/客户端安全与权威状态 exit | P3 | PENDING | Phase 3 五项退出条件均有固定执行证据与独立 review | `docs/04_DEVELOPMENT_PLAN.md` §6.3；AI security execution 未完成 |
| P4-U01 | Fake Hyperliquid 交易、确认、风控、保护仿真 | P4 | PENDING | Phase 4 产物及列出的故障路径有固定验收对象 | `docs/04_DEVELOPMENT_PLAN.md` §7.2—§7.3；没有固定对象 |
| P4-U02 | 仿真交易安全 exit | P4 | PENDING | Phase 4 六项退出条件均有固定执行证据与独立 review | `docs/04_DEVELOPMENT_PLAN.md` §7.4；没有固定对象 |
| P5-U01 | 小额主网指挥交易的一次性前置授权与执行顺序 | P5 | NOT_AUTHORIZED | 用户显式给出 Phase 5 的全部前置授权，且执行顺序有固定受控证据 | `docs/04_DEVELOPMENT_PLAN.md` §8.2—§8.3；当前无授权 |
| P5-U02 | 小额主网指挥交易 exit | P5 | NOT_AUTHORIZED | Phase 5 六项退出条件有固定证据，扩大资金另获授权 | `docs/04_DEVELOPMENT_PLAN.md` §8.4；当前无授权/证据 |
| P6-U01 | 复盘、学习与候选版本闭环 | P6 | PENDING | Phase 6 产物有固定实现和 acceptance | `docs/04_DEVELOPMENT_PLAN.md` §9.2；没有固定对象 |
| P6-U02 | 学习数据隔离与候选版本 exit | P6 | PENDING | Phase 6 六项退出条件均有固定执行证据与独立 review | `docs/04_DEVELOPMENT_PLAN.md` §9.3；没有固定对象 |
| P7-U01 | 原生 iOS 客户端 | P7 | PENDING | Phase 7 产物有固定实现和 acceptance | `docs/04_DEVELOPMENT_PLAN.md` §10.2；没有固定对象 |
| P7-U02 | iOS 会话、确认与状态一致性 exit | P7 | PENDING | Phase 7 五项退出条件均有固定执行证据与独立 review | `docs/04_DEVELOPMENT_PLAN.md` §10.3；没有固定对象 |
| P8-U01 | 受限自动交易实现 | P8 | NOT_AUTHORIZED | Phase 8 前置条件及产物有固定 acceptance，并获用户对具体模型/策略/风险的授权 | `docs/04_DEVELOPMENT_PLAN.md` §11.2—§11.3；当前无授权 |
| P8-U02 | 受限自动交易 safety exit | P8 | NOT_AUTHORIZED | Phase 8 六项退出条件有固定执行证据与独立 review | `docs/04_DEVELOPMENT_PLAN.md` §11.4；当前无授权/证据 |
| P9-U01 | 多用户身份、租户与执行隔离重新评估 | P9 | NOT_AUTHORIZED | Phase 9 进入前重新评估通过，且身份/租户/API Wallet 隔离有固定 acceptance | `docs/04_DEVELOPMENT_PLAN.md` §12；当前阶段未授权，原计划未定义 exit 条件 |
| P9-U02 | 多用户运维、合规与高可用重新评估 | P9 | NOT_AUTHORIZED | Phase 9 进入前重新评估通过，且配额、隐私、合规、HA、支持有固定 acceptance | `docs/04_DEVELOPMENT_PLAN.md` §12；当前阶段未授权，原计划未定义 exit 条件 |

`PROGRESS_SUBGROUPS_V1` 是 `PROGRESS_UNITS_V1` 的稳定、机器可解析分组映射。每个
规则以稳定 ID 正则匹配，不依赖行序、中文名称或工作树状态；`ALL`、`P0`、`P1`、
`P1_DESIGN_CONTRACT`、`P1_RUNTIME_RECOVERY_EXIT` 的分子都只接受匹配行中
`当前状态 = COMPLETED` 的行。

| subgroup ID | 稳定 ID 正则 | 预期分母 |
| --- | --- | ---: |
| ALL | `^P[0-9]-U[0-9][0-9]$` | 25 |
| P0 | `^P0-U[0-9][0-9]$` | 2 |
| P1 | `^P1-U[0-9][0-9]$` | 7 |
| P1_DESIGN_CONTRACT | `^P1-U0[1-3]$` | 3 |
| P1_RUNTIME_RECOVERY_EXIT | `^P1-U0[4-7]$` | 4 |

统一公式（所有单元等权）：`完成率 = COMPLETED 行数 / 全部行数 × 100`，显示 1 位
小数。由上表机械得出：

| 维度 | 计数 | 进度 | 解释 |
| --- | ---: | ---: | --- |
| 整体产品 | `4/25` | `16.0%` | 仅 `P0-U01` 与 `P1-U01`—`P1-U03` 计分；AI `367b86f` 的 review PASS 只关闭 provenance，未满足 P3-U01 的完整 Phase 3 产物/acceptance 条件。 |
| Phase 0 | `1/2` | `50.0%` | `P0-U02` 缺 `P0-004` 固定 exit-review 证据，不计分。 |
| Phase 1 | `3/7` | `42.9%` | 三个 design/contract 单元已满足固定审查条件；四个 runtime/recovery-exit 单元均未完成。 |
| Phase 1 design / contract | `3/3` | `100.0%` | 仅描述 `P1-U01`—`P1-U03`，不表示 Phase 1 runtime 或阶段退出。 |
| Phase 1 runtime / recovery exit | `0/4` | `0.0%` | P1-003 已通过其 fixed-review，但 `P1-U04` 的 task-packet acceptance 未在本快照完整核验；P1-004 R6 为 `DEPENDENCY_BLOCKED/NOT_STARTED`，故 `P1-U04`—`P1-U07` 均不计分。 |

可复制的检查（从仓库根目录运行；它按 `PROGRESS_SUBGROUPS_V1` 的稳定 ID 正则，
只计 `COMPLETED`）：

```sh
awk -F '|' '
  $2 ~ /^ P[0-9]-U[0-9][0-9] $/ {
    id = $2; sub(/^ /, "", id); sub(/ $/, "", id);
    completed = ($5 == " COMPLETED ");
    add("ALL", completed);
    if (id ~ /^P0-U[0-9][0-9]$/) add("P0", completed);
    if (id ~ /^P1-U[0-9][0-9]$/) add("P1", completed);
    if (id ~ /^P1-U0[1-3]$/) add("P1_DESIGN_CONTRACT", completed);
    if (id ~ /^P1-U0[4-7]$/) add("P1_RUNTIME_RECOVERY_EXIT", completed);
  }
  function add(group, is_completed) { total[group]++; if (is_completed) done[group]++ }
  function report(group) {
    printf "%s=%d/%d=%.1f%%\n", group, done[group] + 0, total[group],
      100 * (done[group] + 0) / total[group];
  }
  END {
    report("ALL"); report("P0"); report("P1");
    report("P1_DESIGN_CONTRACT"); report("P1_RUNTIME_RECOVERY_EXIT");
  }
' docs/CURRENT_STATUS.md
```

预期输出为 `ALL=4/25=16.0%`、`P0=1/2=50.0%`、`P1=3/7=42.9%`、
`P1_DESIGN_CONTRACT=3/3=100.0%`、`P1_RUNTIME_RECOVERY_EXIT=0/4=0.0%`。这套
口径不把“已推送”“设计审查 PASS”“`go test -race` 绿测”视为 runtime、Phase exit
或产品就绪；AI `367b86f` 的独立 PASS 也不自动补齐 P3-U01 的其他产物和 acceptance。

## 4. 证据边界

- 官方 npm 证据仅可证明某次从官方 registry 解析到的包版本、integrity 和时间；
  它不证明代码安全、可运行、已接入或已部署。任何 npm 依赖结论必须在固定对象
  中保留 registry、版本、integrity、命令输出和时间，不能以本机缓存或口头结果替代。
- `go test -race` 仅能证明该固定候选在该 Go 进程/测试覆盖下未检测到 race；它
  不能证明 PostgreSQL 事务、NATS/JetStream 投递、跨进程竞争、真实网络或 AI 安全
  执行。P1-004 R6 为 `DEPENDENCY_BLOCKED/NOT_STARTED`，目标 branch/worktree `ABSENT`，
  shared interfaces 与 disposable PostgreSQL 16 preflight 均未就绪；P1-003 `PASS` 不会将其
  升级为 runtime `PASS`。
- 上表只引用固定 commit 或绑定到 as-of 的明确证据状态。移动工作树、运行中 review、
  远端分支存在和设计文档都不得替代固定候选审查。

## 5. 当前安全与发布状态

| 项目 | 状态 |
| --- | --- |
| `main` 集成 | `NO_MAIN_MERGE`；本表所有 P1/AI Agent 分支均未合并 |
| 服务器部署 | `NOT_DEPLOYED` |
| 生产启用 | `PRODUCTION_DISABLED` |
| Hyperliquid 连接 | `NOT_CONNECTED` |
| 钱包/签名器 | `NOT_IMPLEMENTED` |
| 真实下单 | `NOT_AUTHORIZED` |
| 自动交易 | `NOT_AUTHORIZED` |
| 本文档候选授权 | `MERGE=false`、`DEPLOY=false`、`PRODUCTION=false`、`LIVE_TRADING=false` |

## 6. 下一门槛与已知风险

1. P1-003 `a3108988463e9b373b8263fb3f060f8908c54c82` 已获 reviewer-7 `0/0/0 PASS` 并
   推送；它满足 P1-004 的 P1-003 接受前置，但不补齐 P1-004 所需 shared interfaces，也不构成
   P1-003 task-packet acceptance 或 Phase 1 runtime 计分证据。
2. 补齐 Operation、ExecutionAttempt、Order 的交易 domain subjects；目前不能把通用
   owner audit event 当成交易执行 JetStream subject。
3. P1-004 R6 为 `DEPENDENCY_BLOCKED/NOT_STARTED`：先由 architect/root 补齐 OwnerMutation
   input/result 与 NATS consumer business-effect shared interfaces，并完成 Docker PostgreSQL 16
   preflight；persistence 不得自造接口。目标 branch/worktree 仍 `ABSENT`，收到可核验证据前
   不创建或登记 R6 fixed object，也不能提前标为接受或 runtime `PASS`。
4. P1-005 先等待已接受的 P1-004 receipt gate；随后才可在真实 NATS/JetStream 上验证
   投递、重放、去重、顺序和恢复。
5. AI Agent `367b86f` 的 provenance fixed-review 已关闭；仍须完成 Phase 3 其余只读
   产物与真实 AI security execution，模型声明不能代替注入、工具越权、确认绕过和未知
   结果的执行证据。

在上述门槛完成并经过独立固定提交审查前，项目保持开发/模拟阶段。
