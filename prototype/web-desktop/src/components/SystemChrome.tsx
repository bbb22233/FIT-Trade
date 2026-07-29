import {
  Briefcase,
  CheckCircle,
  Database,
  FileText,
  GearSix,
  HandPalm,
  Pulse,
  ShieldCheck,
  ShieldWarning,
  SquaresFour,
  TrendUp,
  X,
} from "@phosphor-icons/react";
import {
  dataStateCopy,
  type DataState,
  type OperationState,
} from "../simulation-types";

const navItems = [
  { label: "概览", icon: SquaresFour },
  { label: "市场", icon: TrendUp },
  { label: "仓位", icon: Briefcase },
  { label: "订单", icon: FileText },
  { label: "风险", icon: ShieldCheck },
  { label: "策略", icon: Pulse },
  { label: "数据", icon: Database },
  { label: "设置", icon: GearSix },
];

export function TopBar({
  dataState,
  killSwitchActive,
  operationState,
  onCycleDataState,
  onKillSwitch,
}: {
  dataState: DataState;
  killSwitchActive: boolean;
  operationState: OperationState;
  onCycleDataState: () => void;
  onKillSwitch: () => void;
}) {
  const protectionText =
    operationState === "PROTECTION_PENDING"
      ? "PROTECTION PENDING"
      : "PROTECTED";

  return (
    <header className="top-bar">
      <div className="brand-block">
        <span>Hermes Command</span>
        <span className="mock-badge">本地模拟</span>
      </div>
      <div className="system-track" aria-label="系统状态">
        <div className="system-item mainnet">
          <span className="status-dot" aria-hidden="true" />
          MAINNET 演练
        </div>
        <button
          className={`system-item data-state ${dataState.toLowerCase()}`}
          type="button"
          onClick={onCycleDataState}
          aria-label={`当前本地模拟数据状态 ${dataState}，点击切换状态`}
          title="本地模拟控制：切换 LIVE / STALE / RECONCILING"
        >
          <span className="status-dot" aria-hidden="true" />
          {dataState}
        </button>
        <div className="system-item protected">
          <ShieldCheck size={17} weight="regular" aria-hidden="true" />
          {protectionText}
        </div>
        <div className="system-item automation">
          <span className="status-dot" aria-hidden="true" />
          自动交易：关闭
        </div>
      </div>
      <button
        className={`kill-button ${killSwitchActive ? "is-active" : ""}`}
        type="button"
        onClick={onKillSwitch}
        disabled={killSwitchActive}
      >
        <HandPalm size={18} weight="bold" aria-hidden="true" />
        {killSwitchActive ? "已停止增加风险" : "停止开仓和增加风险"}
      </button>
      <div className="clock">2026-07-28&nbsp;&nbsp;14:33:18&nbsp;&nbsp;SIM</div>
    </header>
  );
}
export function GlobalStateBanner({
  dataState,
  killSwitchActive,
}: {
  dataState: DataState;
  killSwitchActive: boolean;
}) {
  if (!killSwitchActive && dataState === "LIVE") return null;

  if (killSwitchActive) {
    return (
      <div className="safety-banner" role="alert">
        <ShieldWarning size={17} weight="fill" aria-hidden="true" />
        <strong>Kill Switch 模拟已启用：</strong>
        已禁止开仓、加仓和其他增加风险操作；已有保护、减仓和平仓能力继续运行。
      </div>
    );
  }

  return (
    <div
      className={`safety-banner data-${dataState.toLowerCase()}`}
      role="status"
    >
      <ShieldWarning size={17} weight="fill" aria-hidden="true" />
      <strong>{dataState}：</strong>
      {dataStateCopy[dataState].detail}
    </div>
  );
}

export function LeftNavigation({
  active,
  onSelect,
}: {
  active: string;
  onSelect: (label: string) => void;
}) {
  return (
    <nav className="left-nav" aria-label="主要功能">
      <div className="nav-items">
        {navItems.map(({ label, icon: Icon }) => (
          <button
            className={`nav-item ${active === label ? "active" : ""}`}
            type="button"
            key={label}
            onClick={() => onSelect(label)}
            aria-current={active === label ? "page" : undefined}
          >
            <Icon size={22} weight="regular" aria-hidden="true" />
            <span>{label}</span>
          </button>
        ))}
      </div>
      <button className="nav-collapse" type="button" aria-label="收起导航">
        ‹‹
      </button>
    </nav>
  );
}

export function StatusFooter({
  dataState,
  killSwitchActive,
}: {
  dataState: DataState;
  killSwitchActive: boolean;
}) {
  return (
    <footer className="status-footer">
      <div>
        <span>数据状态</span>
        <strong className={dataState === "LIVE" ? "quote-up" : "state-warning"}>
          <span className="status-dot" aria-hidden="true" /> {dataState}
        </strong>
      </div>
      <div>
        <span>模拟延迟</span>
        <strong className="quote-up">24ms</strong>
      </div>
      <div>
        <span>模拟区块</span>
        <strong>31,542,871</strong>
      </div>
      <div>
        <span>资金费率样例</span>
        <strong>03:27:42</strong>
      </div>
      <div className="footer-right">
        <span className="status-dot" aria-hidden="true" />
        <strong>
          {killSwitchActive
            ? "本地模拟 / 增加风险已停"
            : "本地模拟 / 无外部连接"}
        </strong>
      </div>
    </footer>
  );
}

export function KillSwitchDialog({
  open,
  onClose,
  onActivate,
}: {
  open: boolean;
  onClose: () => void;
  onActivate: () => void;
}) {
  if (!open) return null;

  return (
    <div className="dialog-backdrop" role="presentation">
      <section
        className="safety-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="kill-title"
      >
        <div className="dialog-title">
          <ShieldWarning size={22} weight="fill" aria-hidden="true" />
          <h2 id="kill-title">模拟停止开仓和增加风险</h2>
          <button
            type="button"
            className="icon-button"
            onClick={onClose}
            aria-label="关闭"
          >
            <X size={18} aria-hidden="true" />
          </button>
        </div>
        <p>
          本地状态将立即禁止开仓、加仓、提高杠杆和放宽止损。这个操作不会自动平仓，也不会发送任何外部请求。
        </p>
        <ul>
          <li>
            <CheckCircle size={16} aria-hidden="true" />
            Stop Market、止盈和保护监控样例继续运行
          </li>
          <li>
            <CheckCircle size={16} aria-hidden="true" />
            reduce-only 减仓和平仓样例仍然可用
          </li>
        </ul>
        <div className="dialog-actions">
          <button type="button" onClick={onClose}>
            取消
          </button>
          <button
            type="button"
            className="danger-action"
            onClick={onActivate}
          >
            <HandPalm size={17} aria-hidden="true" />
            模拟停止增加风险
          </button>
        </div>
      </section>
    </div>
  );
}

export function Toast({ message }: { message: string }) {
  if (!message) return null;
  return (
    <div className="toast" role="status">
      {message}
    </div>
  );
}
