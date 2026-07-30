# 当前项目进度

更新日期：2026-07-30
证据截止时间（as-of）：`2026-07-30T06:58:45-07:00`
状态快照：`codex/round-5-progress-docs`（精确 base
`codex/round-4-progress-docs@7f741da4ecb3a1a309a5c884973cb38f7b40b100`）
协调状态：`DEVELOPMENT_PAUSED_BY_USER`；active writers=`0`。除本轮两份状态文档的
固定提交与独立只读复审外，不再下载依赖、修复、启动 runtime、合并或部署。

## 1. 结论

P1-003 auth core 固定对象
`a3108988463e9b373b8263fb3f060f8908c54c82`（parent
`19b778785565d31dc095996ce4d4b0f7d58bebdf`）已获 reviewer-7 独立 fixed-commit review
`P0/P1/P2 = 0/0/0`、`PASS`；`origin/codex/p1-003-auth-core` 仍精确指向该对象且未合并。

P1-003B shared ports 的原 dirty source `859786af5b45946af0c930633e934c0d275006b2`
已被拒绝；clean fixed object 是
`958195ad2fde646c3c956ef42c734b1175d6f154`（direct parent
`a3108988463e9b373b8263fb3f060f8908c54c82`、tree
`cc87858aadfdd053fec587a13de8e779b44fd4f7`）。其 workflowport-only 代码、isolated Go、race、
negative-compile 与 accepted-platform-object gate 均有 `SERVER_EXTERNAL_EVIDENCE`（见 §4）；但候选直接 `npm test`
仍因 P1-001 fixed-base `test:platform` wrapper 而失败，且官方 R7 intake/build 两次均
`FAIL_CLOSED_NO_ARTIFACT`、`P0/P1/P2=0/1/0`、cleanup/residue=0。因此 `958195…` 尚未
accepted 或 pushed。cross-phase T
`72cf5f5189c9632dbadd08f551592bb1b2a31a90`（同为
`a3108988463e9b373b8263fb3f060f8908c54c82` 的 sibling、tree
`ba854f0c442d5ff461d899c163563fdfaf941ef1`）自身/完整 fixed tests 有
`SERVER_EXTERNAL_EVIDENCE`，但实际
`958195…` gate 暴露 offline module artifact 缺口；T 为 `CHANGES_REQUIRED`，未推送，不得写作
代码失败或 T `PASS`。R7 第三次官方下载网络窗口未获授权且已因用户暂停而取消；此前
两次尝试均 fail-closed。

P1-004 R6 仍为 `NOT_STARTED`，只等待 accepted P1-003B shared ports；旧
`a41960ff98a8b3be7b82e31f06c89ccb085c2593` 未触碰。disposable PostgreSQL 16 预检与
P1-004 runtime 是两项不同证据，且预检 R2—R5 总体仍为 `OVERALL=UNVERIFIED`：
R2 dynamic-port resolution 失败；R3 容器 internal network 阻断 host port；R4 ordinary
bridge 已连通但 Bash `set -u` local-variable 失败；R5 固定文件只证明 harness preflight
`PASS`、真实运行 `FAIL_CLOSED_RC=2 / CHANGES_REQUIRED`，且 receipt/manifest 不存在。
R5 的具体触发点、version/rollback/CAS/recreate 执行状态以及运行后 cleanup/residue
没有绑定到当前登记证据，统一为 `UNVERIFIED`。四轮均为 `CHANGES_REQUIRED`。这不是
P1-004 代码失败，亦绝不外推为 R6 runtime `PASS` 或计分；用户暂停后不再启动 R6。

P1-005 存在本地 fixed candidate
`codex/p1-005-jetstream-runtime@d9dcf3ef2f82cc001023642aa974be23931c445a`（parent
`19b778785565d31dc095996ce4d4b0f7d58bebdf`、15 paths），状态为
`NOT_ACCEPTED/NOT_PUSHED/P1-004_INTEGRATION_BLOCKED`；其绿测/包证据不计 runtime。JetStream
environment preflight R6 的 `SERVER_EXTERNAL_EVIDENCE` 声明 `PASS`：`nats:2.11.8-alpine` digest
`sha256:71092f77d707a4a81b12aca5096d6b2d2e07ad16aa57c84066940a17af74f61a`、server
`v2.11.8`、`nats.go@v1.39.1` 的 loopback/auth/dedupe/ack/redelivery/persistence/restart/cleanup
均通过；R2 hash verifier 两次 exit 0。该外部预检仍不等于候选 acceptance/runtime。
AI Agent fixed R3 `5f6273d5b7e1661460e2aa8c29cb73eeeb3d623e`（parent `60850…`）为 15 个 add-only
文档，且 origin branch 精确已推送；这些 Git 事实为 `VERIFIED`。绑定该对象的独立 review
没有 durable receipt，故 `EXTERNAL_FIXED_REVIEW_RECEIPT_UNAVAILABLE / UNVERIFIED`；不增加
P3-U01 分子。

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
| AI Agent / Hermes R3 | `5f6273d5b7e1661460e2aa8c29cb73eeeb3d623e`；parent `60850b69dd06c46bbe2a9cfab19d27cdb2ef0412`；`codex/ai-agent-fixed-r3` | Git object/parent/15 add-only docs `VERIFIED`；`EXTERNAL_FIXED_REVIEW_RECEIPT_UNAVAILABLE / UNVERIFIED` | `origin` 精确已推送，未合并 | 不声称 auditable `0/0/0 PASS`；仍不等于 Phase 3 runtime/security exit，亦不增加 P3-U01 |
| P1-003 auth core 运行时 | `a3108988463e9b373b8263fb3f060f8908c54c82`；parent `19b778785565d31dc095996ce4d4b0f7d58bebdf`；`codex/p1-003-auth-core` | reviewer-7：`P0/P1/P2 = 0/0/0`，`PASS` | 已推送并以 `origin` 远端 ref 核验；未合并 | 已接受的 P1-003 固定对象；不授权 P1-004 acceptance、main merge 或发布 |
| P1-003B shared ports | `958195ad2fde646c3c956ef42c734b1175d6f154`；parent `a3108988463e9b373b8263fb3f060f8908c54c82`；tree `cc87858aadfdd053fec587a13de8e779b44fd4f7`；server-only `codex/p1-003b-shared-ports-clean-r1` | `SERVER_EXTERNAL_EVIDENCE`：code/isolated gates mostly `PASS`；official R7 `P0/P1/P2=0/1/0 CHANGES_REQUIRED` | 未 accepted、未 pushed；R7 artifact=0、residue=0 | exact paths/SHA 见 §4；third network attempt 未授权并已取消 |
| P1-GATE cross-phase T | `72cf5f5189c9632dbadd08f551592bb1b2a31a90`；parent `a3108988463e9b373b8263fb3f060f8908c54c82`；tree `ba854f0c442d5ff461d899c163563fdfaf941ef1`；server-only `codex/p1-gate-crossphase-r1` | `SERVER_EXTERNAL_EVIDENCE`：self/full fixed tests support；overall actual-`958195…` gate `P0/P1/P2=0/1/0 CHANGES_REQUIRED` | 未 pushed | offline module artifact 缺口；不是 shared-ports code failure，不能写 T `PASS` |
| P1-004 PostgreSQL runtime | R6 `NOT_STARTED`；无 R6 fixed object | P1-003B 未 accepted；旧 `a41960ff98a8b3be7b82e31f06c89ccb085c2593` 未触碰 | active writer=`0`；用户已暂停 | R2—R5 均 `CHANGES_REQUIRED`；preflight `OVERALL=UNVERIFIED`，不得代替 R6 acceptance |
| P1-005 JetStream runtime | `codex/p1-005-jetstream-runtime@d9dcf3ef2f82cc001023642aa974be23931c445a`；parent `19b778785565d31dc095996ce4d4b0f7d58bebdf`；15 paths | 本地候选 `NOT_ACCEPTED/NOT_PUSHED/P1-004_INTEGRATION_BLOCKED`；`SERVER_EXTERNAL_EVIDENCE` preflight receipt 声明 `PASS` | origin 无该分支；等待 accepted P1-004 receipt gate | 绿测/包/preflight 不得算作 runtime acceptance |

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
| P1-U05 | PostgreSQL 原子持久化 runtime | P1 | PENDING | `P1-004` 在真实 disposable PostgreSQL 上通过 task-packet acceptance 并独立审查 | P1-004 R6 未启动，等待 accepted P1-003B；PG16 preflight `OVERALL=UNVERIFIED` 且不等于 R6 fixed object/acceptance/review，故不计分 |
| P1-U06 | NATS/JetStream、Inbox/Outbox、observability runtime | P1 | PENDING | `P1-005` 固定候选通过投递、重放、去重、故障 acceptance 并独立审查 | 本地 `d9dcf3e` 为 `NOT_ACCEPTED/NOT_PUSHED/P1-004_INTEGRATION_BLOCKED`；external preflight/绿测/包证据不等于 runtime acceptance，故不计分 |
| P1-U07 | default-disabled recovery infrastructure 与 Phase 1 exit | P1 | PENDING | `P1-006` recovery acceptance 完成，且 `P1-007` 线性固定对象有独立 exit `PASS` | 无 P1-006/P1-007 候选；`P1-006`、`P1-007` |
| P2-U01 | Hyperliquid 只读 Connector 与 Go 投影 | P2 | PENDING | Phase 2 所列只读市场/账户同步、重连和投影有固定验收对象 | `docs/04_DEVELOPMENT_PLAN.md` §5.2；没有固定对象 |
| P2-U02 | Hyperliquid 只读一致性 exit | P2 | PENDING | Phase 2 五项退出条件均有固定验收与独立 review | `docs/04_DEVELOPMENT_PLAN.md` §5.3；没有固定对象 |
| P3-U01 | Hermes 与网页/桌面只读实现 | P3 | PENDING | Phase 3 产物有固定实现和 acceptance；AI 安全资料必须通过 provenance review | AI R3 `5f6273d` 的 Git object/push 已核验，但独立 review receipt unavailable/`UNVERIFIED`；Phase 3 完整只读产物/acceptance 亦未齐备，故不计分；`docs/04_DEVELOPMENT_PLAN.md` §6.2 |
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
| 整体产品 | `4/25` | `16.0%` | 仅 `P0-U01` 与 `P1-U01`—`P1-U03` 计分；AI R3 `5f6273d` review receipt unavailable/`UNVERIFIED`，且未满足 P3-U01 完整产物/acceptance。 |
| Phase 0 | `1/2` | `50.0%` | `P0-U02` 缺 `P0-004` 固定 exit-review 证据，不计分。 |
| Phase 1 | `3/7` | `42.9%` | 三个 design/contract 单元已满足固定审查条件；四个 runtime/recovery-exit 单元均未完成。 |
| Phase 1 design / contract | `3/3` | `100.0%` | 仅描述 `P1-U01`—`P1-U03`，不表示 Phase 1 runtime 或阶段退出。 |
| Phase 1 runtime / recovery exit | `0/4` | `0.0%` | P1-003 已通过其 fixed-review，但 `P1-U04` 的 task-packet acceptance 未在本快照完整核验；P1-004 R6 为 `NOT_STARTED`，P1-005 `d9dcf3e` 为 integration-blocked，故 `P1-U04`—`P1-U07` 均不计分。 |

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
或产品就绪；AI R3 `5f6273d`、P1-005 `d9dcf3e`、PostgreSQL/JetStream preflight 与 P1-003B isolated gates
也不自动补齐任何 runtime/Phase exit 单元。

## 4. 证据边界

`SERVER_EXTERNAL_EVIDENCE` 表示本地仓库不含该固定对象或 receipt；本快照通过只读 SSH
重算下列服务器文件的 SHA-256 并核对其关键 verdict。它只支持所列外部证据声明，不把文件
复制进仓库，也不自动成为本地 acceptance：

| 外部证据 | 服务器绝对路径 | 本快照重算 SHA-256 | 支持边界 |
| --- | --- | --- | --- |
| P1-003B independent fixed review | `/root/fit-trade-dev/evidence/p1-003b-fixed-independent-review-r1.txt` | `ca9ede4ad9be75e85f4c608d48b8e8ac9f2d084da342336e62733bfe7405bf2e` | `958195…` overall `CHANGES_REQUIRED 0/1/0`；包含 mostly-green isolated code gates |
| cross-phase T fixed gate | `/root/fit-trade-dev/evidence/p1-gate-crossphase-full-fixed-r1.txt` | `916f01e0f7ffe766414fe0da4ee0193a9ed0e97f543173860937c0b8636c7c86` | `72cf5…` self/full evidence；overall actual-`958195…` gate `CHANGES_REQUIRED 0/1/0` |
| R7 intake failure | `/root/fit-trade-dev/evidence/p1-gate-r7-intake-r1-failure.txt` | `437e8bedacc9c713bcf06932c0c1ed966fbdbab488c08edd5d4fd4090f35e468` | artifact=0、residue=0、`CHANGES_REQUIRED 0/1/0` |
| R7 build failure | `/root/fit-trade-dev/evidence/p1-gate-r7-artifact-build-r1-failure.txt` | `f2956ee0016fbfd05f8e58c8f8aab274104bb255ddc74566db86b25ddc181726` | final artifact=0、residue=0、third network unauthorized、`CHANGES_REQUIRED 0/1/0` |
| PG16 preflight R2 receipt | `/root/fit-trade-dev/evidence/p1-004-postgres16-preflight-r2-receipt.json` | `733a775ca4844de8df7258250010f51b5051f1c4606987a251e1ea867259854d` | `CHANGES_REQUIRED 0/1/0`；generation-1 dynamic-port failure，未进入 DB gate |
| PG16 preflight R2 manifest | `/root/fit-trade-dev/evidence/p1-004-postgres16-preflight-r2-manifest.json` | `0cfbe369a6b6d76937599737b563a972994c889e1f612d11d3ed4ea7b38a3e44` | receipt/raw/baseline hashes；status `CHANGES_REQUIRED` |
| PG16 preflight R2 raw | `/root/fit-trade-dev/evidence/p1-004-postgres16-preflight-r2-raw.log` | `ebbb6b9fa21adb5b04e31b391a035dafa8f5bb9a689f7fd1e5be85a1dbabea4d` | dynamic-port failure 的原始执行证据 |
| PG16 preflight R3 receipt | `/root/fit-trade-dev/evidence/p1-004-postgres16-preflight-r3-receipt.json` | `e2afa88852023fe2a33feecf0f4a1580aebf394129698219ad1d7c5999e0ad31` | `CHANGES_REQUIRED`；internal network 阻断 host port |
| PG16 preflight R3 manifest | `/root/fit-trade-dev/evidence/p1-004-postgres16-preflight-r3-manifest.json` | `4b9e6934fc7196ec37dc7cb5311b09cd6d913b90336ad7795663d9506df032fe` | receipt/raw/baseline hashes；status `CHANGES_REQUIRED` |
| PG16 preflight R3 raw | `/root/fit-trade-dev/evidence/p1-004-postgres16-preflight-r3-raw.log` | `3c1c58487dd6004dbd58f6c523619f938935574620eb0ba96723064d8a617530` | internal-network failure 的原始执行证据 |
| PG16 preflight R4 receipt | `/root/fit-trade-dev/evidence/p1-004-postgres16-preflight-r4-receipt.json` | `c1a6a2f3f6bff87336e63702597a815881db87df6c6dae6ff1ac7c0eac90ad11` | `CHANGES_REQUIRED`；ordinary bridge 连通后 Bash `set -u` 失败 |
| PG16 preflight R4 manifest | `/root/fit-trade-dev/evidence/p1-004-postgres16-preflight-r4-manifest.json` | `39816efcda9b8d2b7f07e7c1faa01a6393814a6e2c6c02068bebcc896d6f9f2c` | receipt/raw/baseline hashes；status `CHANGES_REQUIRED` |
| PG16 preflight R4 raw | `/root/fit-trade-dev/evidence/p1-004-postgres16-preflight-r4-raw.log` | `c4af7bb77635be04c4a1d38f76000d8a8bb3000dee433f047c59caf772fbe5ce` | ordinary-bridge/Bash failure 的原始执行证据 |
| PG16 preflight R5 harness | `/root/fit-trade-dev/evidence/p1-004-postgres16-preflight-r5-harness-preflight.log` | `8ed8f8ae79ca2c37543e5df4b36793b4880e9821731fc4a799e2a6e6bc2a67ea` | harness 自检 `PASS`；不等于 DB matrix PASS |
| PG16 preflight R5 raw | `/root/fit-trade-dev/evidence/p1-004-postgres16-preflight-r5-raw.log` | `106499669aed62b7a0e01035ac5adfeb4072fb58ec278336c6e22bf3473cb417` | 只支持 `FAIL_CLOSED_RC=2 / CHANGES_REQUIRED`；具体触发点、后续 gate 与 cleanup/residue 均 `UNVERIFIED` |
| JetStream R6 receipt | `/root/fit-trade-dev/evidence/p1-005-jetstream-environment-preflight-r6-final-r2.txt` | `7729fdc3983cb7c9ff58c2204c822d6c68c646b69f258d7b978b0c5fa2d60e55` | receipt declares semantic/evidence `PASS`；仅 preflight |
| JetStream R6 manifest | `/root/fit-trade-dev/evidence/p1-005-jetstream-environment-preflight-r6-final-r2.txt.hashes-r2` | `0f3751338a645527fdfc74f3c9beb7e0acb98f2fcc496f651d665ef21d4fefd0` | evidence file hashes |
| JetStream R6 verifier | `/root/fit-trade-dev/evidence/p1-005-jetstream-environment-preflight-r6-final-r2-verifier.txt` | `68e9b904b804f202b0efabf88a1f8dafd79cf4ca42d781441a9b64ded47f9aca` | verifier exit 0 twice；仅绑定 external evidence package |

- 官方 npm 证据仅可证明某次从官方 registry 解析到的包版本、integrity 和时间；
  它不证明代码安全、可运行、已接入或已部署。任何 npm 依赖结论必须在固定对象
  中保留 registry、版本、integrity、命令输出和时间，不能以本机缓存或口头结果替代。
- `go test -race` 仅能证明该固定候选在该 Go 进程/测试覆盖下未检测到 race；它
  不能证明 PostgreSQL 事务、NATS/JetStream 投递、跨进程竞争、真实网络或 AI 安全
  执行。P1-003B 的 fixed code gates、PG partial observations 和 JetStream preflight 都不能替代 P1-004/P1-005
  各自的 fixed runtime acceptance/review；P1-003 `PASS` 不会将其升级为 runtime `PASS`。
- 上表只引用固定 commit 或绑定到 as-of 的明确证据状态。移动工作树、运行中 review、
  远端分支存在和设计文档都不得替代固定候选审查。

## 5. 当前安全与发布状态

| 项目 | 状态 |
| --- | --- |
| `main` 集成 | `NO_MAIN_MERGE`；`origin/main` 仍精确为 `60850b69dd06c46bbe2a9cfab19d27cdb2ef0412`，本表所有 P1/AI Agent 分支均未合并 |
| 开发调度 | `DEVELOPMENT_PAUSED_BY_USER`；active writers=`0`；持续调度将在本轮文档复审后关闭 |
| 服务器部署 | `NOT_DEPLOYED` |
| 生产启用 | `PRODUCTION_DISABLED` |
| Hyperliquid 连接 | `NOT_CONNECTED` |
| 钱包/签名器 | `NOT_IMPLEMENTED` |
| 真实下单 | `NOT_AUTHORIZED` |
| 自动交易 | `NOT_AUTHORIZED` |
| 本文档候选授权 | `MERGE=false`、`DEPLOY=false`、`PRODUCTION=false`、`LIVE_TRADING=false` |

## 6. 下一门槛与已知风险

当前没有执行中的“下一步”：用户已暂停开发。恢复后仍按以下门槛重新授权和派工：

1. P1-003 `a310…` 已接受/推送，但 P1-003B `958195…` 的 R7 official artifact gate 仍
   `CHANGES_REQUIRED`；第三次官方下载未授权且已取消，恢复后须由用户重新授权。
2. P1-004 R6 只在 P1-003B accepted 后开始；persistence 不得自造 shared ports。PG16 preflight
   R2—R5 均为 `CHANGES_REQUIRED`、总体 `UNVERIFIED`；R5 只固定证明 harness 自检和
   `FAIL_CLOSED_RC=2`，具体触发点、后续 gate 与 cleanup/residue 均未固定。恢复后须先
   产生能绑定完整原因、矩阵与清理结果的 receipt/manifest，再以 exact digest、
   `--pull=never` 完整无网重放和独立 receipt verifier 覆盖 DB gate，且仍不是 P1-004
   runtime 结论。
3. P1-005 等待 accepted P1-004 receipt gate；JetStream preflight 之后仍需真实 NATS/JetStream
   的投递、重放、去重、顺序、故障 acceptance 和独立 review。
4. AI R3 `5f6273d` 的 Git object/push 已核验，但独立 fixed-review receipt unavailable/
   `UNVERIFIED`；补齐 durable review receipt 和 Phase 3 其余只读产物/真实 AI security execution。

项目现停在安全的开发/模拟阶段。未经用户明确恢复授权，不继续上述任何工作。
