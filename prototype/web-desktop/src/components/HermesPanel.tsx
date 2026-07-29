import {
  ArrowsClockwise,
  CheckCircle,
  LockSimple,
  PaperPlaneTilt,
  Pulse,
  PushPin,
  ShieldCheck,
  ShieldWarning,
  Warning,
} from "@phosphor-icons/react";
import {
  useState,
  type FormEvent,
  type RefObject,
} from "react";
import type { SymbolProfile } from "../market-data";
import {
  operationLabels,
  type ChatMessage,
  type ConfirmationTicketSnapshot,
  type DataState,
  type OperationState,
} from "../simulation-types";

function ConfirmationTicket({
  ticket,
  activePair,
  dataState,
  operationState,
  expiresIn,
  canIncreaseRisk,
  onConfirm,
  onEdit,
}: {
  ticket: ConfirmationTicketSnapshot;
  activePair: string;
  dataState: DataState;
  operationState: OperationState;
  expiresIn: number;
  canIncreaseRisk: boolean;
  onConfirm: () => void;
  onEdit: () => void;
}) {
  const isProcessing = [
    "RISK_REVALIDATING",
    "DISPATCH_PENDING",
    "DISPATCHED",
    "ACKNOWLEDGED",
    "UNKNOWN_REQUIRES_RECONCILIATION",
    "PROTECTION_PENDING",
  ].includes(operationState);
  const isFinal = operationState === "PROTECTED";
  const isExpired = operationState === "EXPIRED";
  const isRejected = operationState === "REJECTED";
  const isBindingMismatch = activePair !== ticket.pair;
  const minutes = String(Math.floor(expiresIn / 60)).padStart(2, "0");
  const seconds = String(expiresIn % 60).padStart(2, "0");
  const effectLabel = ticket.positionEffect === "ADD" ? "加仓" : "开仓";
  const editDisabled = isProcessing || isFinal || isRejected;
  const editLabel = isFinal
    ? "已完成，不能修改"
    : isRejected
      ? "已拒绝，不能修改"
      : isProcessing
        ? "执行中，不能修改"
        : "返回修改";

  let disabledCopy = "";
  if (!canIncreaseRisk) {
    disabledCopy =
      dataState === "LIVE"
        ? "Kill Switch：禁止增加风险"
        : `${dataState}：禁止增加风险`;
  }

  return (
    <section
      className="confirmation-ticket"
      aria-labelledby="confirmation-title"
      data-testid="confirmation-ticket"
    >
      <div className="ticket-header">
        <div>
          <span className="ticket-kicker">
            MAINNET 界面演练 · 本地模拟 · 意图已冻结
          </span>
          <h2 id="confirmation-title">
            {ticket.symbol} {effectLabel}确认
          </h2>
          <span className="ticket-identity">
            {ticket.ticketId} · {ticket.pair}
          </span>
        </div>
        <span className="side-label">{ticket.directionLabel}</span>
      </div>

      {isBindingMismatch ? (
        <div
          className="ticket-binding-notice"
          role="status"
          data-testid="ticket-binding-notice"
        >
          图表已切换到 {activePair}；本票据仍锁定 {ticket.pair}，不会随图表改写。
        </div>
      ) : null}

      <div className="ticket-warning">
        <Warning size={17} weight="regular" aria-hidden="true" />
        <span>{operationLabels[operationState]}</span>
        <span className="expiry">
          {isExpired ? "已过期" : `${minutes}:${seconds}`}
        </span>
      </div>

      <div className="risk-ledger">
        <div className="ledger-row critical">
          <span>最大亏损</span>
          <strong>
            {ticket.maxLoss}
            <small>{ticket.maxLossPercent}</small>
          </strong>
        </div>
        <div className="ledger-row critical">
          <span>交易后总风险</span>
          <strong>
            {ticket.totalRisk}
            <small>{ticket.totalRiskPercent}</small>
          </strong>
        </div>
        <div className="ledger-row">
          <span>Stop Market</span>
          <strong>{ticket.stopMarket}</strong>
        </div>
        <div className="ledger-row">
          <span>强平价</span>
          <strong>{ticket.liquidationPrice}</strong>
        </div>
      </div>

      <div className="order-ledger">
        <div>
          <span>数量 / 名义价值</span>
          <strong>
            {ticket.quantity} / {ticket.notional}
          </strong>
        </div>
        <div>
          <span>杠杆 / 保证金</span>
          <strong>
            {ticket.leverage} {ticket.marginMode} / {ticket.margin}
          </strong>
        </div>
        <div>
          <span>参考 / 最差成交</span>
          <strong>
            {ticket.referencePrice} / {ticket.worstFillPrice}
          </strong>
        </div>
        <div>
          <span>费用与滑点预算</span>
          <strong>{ticket.feeBudget}</strong>
        </div>
      </div>

      <p className="ticket-footnote">
        本票据的品种、方向、数量、杠杆、止损和风险字段均已冻结；切换图表不会改写。接入后端后，全部字段必须由服务端权威状态替换。
      </p>

      {isProcessing || isFinal || isRejected ? (
        <div
          className={`operation-state ${isFinal ? "complete" : ""}`}
          aria-live="polite"
          data-testid="operation-progress"
        >
          {isFinal ? (
            <CheckCircle size={18} weight="fill" aria-hidden="true" />
          ) : (
            <ArrowsClockwise size={18} aria-hidden="true" />
          )}
          <div>
            <strong>权威执行进度样例：{operationLabels[operationState]}</strong>
            <span>
              {isFinal
                ? "本地模拟已核对成交、仓位和 Stop Market。"
                : operationState === "UNKNOWN_REQUIRES_RECONCILIATION"
                  ? "不得重试；模拟器会先查询现有 Operation。"
                  : "请勿重复操作；界面仅跟随状态机推进。"}
            </span>
          </div>
        </div>
      ) : null}

      <div className="ticket-actions">
        <button
          type="button"
          className="ticket-secondary"
          onClick={onEdit}
          disabled={editDisabled}
          data-testid="edit-confirmation"
        >
          {editLabel}
        </button>
        <button
          type="button"
          className="confirm-button"
          onClick={onConfirm}
          data-testid="confirm-operation"
          disabled={
            !canIncreaseRisk ||
            isProcessing ||
            isFinal ||
            isExpired ||
            isRejected
          }
        >
          {!canIncreaseRisk ? (
            <>
              <ShieldWarning size={17} aria-hidden="true" />
              {disabledCopy}
            </>
          ) : isFinal ? (
            <>
              <ShieldCheck size={17} aria-hidden="true" />
              模拟成交并确认保护
            </>
          ) : isProcessing ? (
            <>
              <ArrowsClockwise size={17} aria-hidden="true" />
              处理中，请勿重复操作
            </>
          ) : isExpired ? (
            "确认已过期，请重新生成"
          ) : (
            <>
              <LockSimple size={17} aria-hidden="true" />
              模拟确认 MAINNET {effectLabel}（不下单）
            </>
          )}
        </button>
      </div>
    </section>
  );
}
export function HermesPanel({
  profile,
  confirmationTicket,
  dataState,
  messages,
  thinking,
  confirmationVisible,
  operationState,
  expiresIn,
  canIncreaseRisk,
  inputRef,
  onSubmit,
  onConfirm,
  onEdit,
}: {
  profile: SymbolProfile;
  confirmationTicket: ConfirmationTicketSnapshot;
  dataState: DataState;
  messages: ChatMessage[];
  thinking: boolean;
  confirmationVisible: boolean;
  operationState: OperationState;
  expiresIn: number;
  canIncreaseRisk: boolean;
  inputRef: RefObject<HTMLInputElement | null>;
  onSubmit: (value: string) => void;
  onConfirm: () => void;
  onEdit: () => void;
}) {
  const [input, setInput] = useState("");

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    const trimmed = input.trim();
    if (!trimmed) return;
    onSubmit(trimmed);
    setInput("");
  };

  return (
    <aside className="hermes-rail" aria-label="Hermes 本地模拟交易助手">
      <div className="hermes-header">
        <div>
          <Pulse size={20} weight="duotone" aria-hidden="true" />
          <span>Hermes</span>
          <span className="simulated-label">本地模拟</span>
        </div>
        <button className="icon-button" type="button" aria-label="固定Hermes面板">
          <PushPin size={17} aria-hidden="true" />
        </button>
      </div>
      <div className="conversation">
        {messages.map((message) => (
          <article className={`chat-message ${message.role}`} key={message.id}>
            <div className="message-meta">
              <strong>{message.role === "you" ? "你" : "Hermes"}</strong>
              <span>SIM 14:{message.id === 1 ? "30" : "31"}</span>
            </div>
            <p>{message.content}</p>
          </article>
        ))}
        {thinking ? (
          <div className="thinking-row">
            <ArrowsClockwise size={16} aria-hidden="true" />
            本地模拟 Hermes 正在生成结构化意图…
          </div>
        ) : null}
      </div>

      {confirmationVisible ? (
        <ConfirmationTicket
          ticket={confirmationTicket}
          activePair={profile.pair}
          dataState={dataState}
          operationState={operationState}
          expiresIn={expiresIn}
          canIncreaseRisk={canIncreaseRisk}
          onConfirm={onConfirm}
          onEdit={onEdit}
        />
      ) : (
        <div className="no-confirmation">
          <ShieldCheck size={20} aria-hidden="true" />
          <strong>当前没有待确认的模拟增加风险操作</strong>
          <span>模型文本不能直接触发签名或下单。</span>
        </div>
      )}

      <form className="chat-composer" onSubmit={handleSubmit}>
        <label htmlFor="hermes-command" className="sr-only">
          向本地模拟 Hermes 发送消息
        </label>
        <input
          id="hermes-command"
          ref={inputRef}
          value={input}
          onChange={(event) => setInput(event.target.value)}
          placeholder="输入开仓或加仓演练指令…"
          disabled={thinking}
        />
        <button
          type="submit"
          aria-label="发送模拟消息"
          disabled={!input.trim() || thinking}
        >
          <PaperPlaneTilt size={18} weight="fill" aria-hidden="true" />
        </button>
      </form>
    </aside>
  );
}
