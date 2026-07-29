import { useEffect, useMemo, useRef, useState } from "react";
import {
  formatPrice,
  symbolProfiles,
  type SymbolProfile,
  type SymbolKey,
  type Timeframe,
} from "./market-data";
import type {
  ChatMessage,
  ConfirmationTicketSnapshot,
  DataState,
  OperationState,
  PositionEffect,
} from "./simulation-types";
import { isOperationActive } from "./simulation-types";

const defaultMessages: ChatMessage[] = [
  {
    id: 1,
    role: "hermes",
    content:
      "已载入 BTC 的本地模拟 1h / 4h 数据、账户风险和持仓样例。这里不会连接模型、钱包或交易所。",
  },
  {
    id: 2,
    role: "you",
    content: "BTC 5倍做多，止损放在1小时前低，仓位你计算。",
  },
  {
    id: 3,
    role: "hermes",
    content:
      "已在本地识别为增加风险的开仓演练。模拟 Go 风控计算为 0.025 BTC；请核对结构化确认票据。",
  },
];

const executionSequence: Array<[number, OperationState]> = [
  [650, "DISPATCH_PENDING"],
  [1_250, "DISPATCHED"],
  [1_900, "ACKNOWLEDGED"],
  [2_550, "PROTECTION_PENDING"],
  [3_450, "PROTECTED"],
];

const dataStates: DataState[] = ["LIVE", "STALE", "RECONCILING"];

function createConfirmationTicketSnapshot(
  profile: SymbolProfile,
  positionEffect: PositionEffect,
  sequence: number,
): ConfirmationTicketSnapshot {
  return Object.freeze({
    ticketId: `SIM-${String(sequence).padStart(4, "0")}`,
    symbol: profile.symbol,
    pair: profile.pair,
    positionEffect,
    direction: "BUY_LONG",
    directionLabel: positionEffect === "ADD" ? "买入加多" : "买入开多",
    quantity: profile.quantity,
    notional: profile.notional,
    leverage: "5x",
    marginMode: "逐仓",
    margin: profile.margin,
    referencePrice: formatPrice(profile, profile.entry),
    worstFillPrice: formatPrice(profile, profile.worstFill),
    stopMarket: formatPrice(profile, profile.stop),
    liquidationPrice: formatPrice(profile, profile.liquidation),
    maxLoss: profile.maxLoss,
    maxLossPercent: profile.maxLossPercent,
    totalRisk: profile.totalRisk,
    totalRiskPercent: profile.totalRiskPercent,
    feeBudget: profile.feeBudget,
  });
}

export function useCommandCenterSimulation() {
  const [symbol, setSymbol] = useState<SymbolKey>("BTC");
  const [timeframe, setTimeframe] = useState<Timeframe>("1h");
  const [dataState, setDataState] = useState<DataState>("LIVE");
  const [indicatorEnabled, setIndicatorEnabled] = useState(true);
  const [activeNav, setActiveNav] = useState("仓位");
  const [bottomTab, setBottomTab] = useState("持仓 (1)");
  const [messages, setMessages] = useState(defaultMessages);
  const [thinking, setThinking] = useState(false);
  const [confirmationVisible, setConfirmationVisible] = useState(true);
  const [confirmationTicket, setConfirmationTicket] =
    useState<ConfirmationTicketSnapshot>(() =>
      createConfirmationTicketSnapshot(symbolProfiles.BTC, "OPEN", 1),
    );
  const [operationState, setOperationState] =
    useState<OperationState>("AWAITING_CONFIRMATION");
  const [expiresIn, setExpiresIn] = useState(600);
  const [killDialogOpen, setKillDialogOpen] = useState(false);
  const [killSwitchActive, setKillSwitchActive] = useState(false);
  const [toast, setToast] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const timersRef = useRef<number[]>([]);
  const ticketSequenceRef = useRef(1);
  const operationStateRef = useRef<OperationState>("AWAITING_CONFIRMATION");
  const profile = useMemo(() => symbolProfiles[symbol], [symbol]);

  const transitionOperationState = (state: OperationState) => {
    operationStateRef.current = state;
    setOperationState(state);
  };

  const clearExecutionTimers = () => {
    timersRef.current.forEach((timer) => window.clearTimeout(timer));
    timersRef.current = [];
  };

  const replaceConfirmation = (ticket: ConfirmationTicketSnapshot) => {
    if (isOperationActive(operationStateRef.current)) {
      setToast(
        "当前 Operation 必须先完成或核对；原票据和执行进度保持不变。",
      );
      return false;
    }
    clearExecutionTimers();
    setConfirmationTicket(ticket);
    transitionOperationState("AWAITING_CONFIRMATION");
    setExpiresIn(600);
    setConfirmationVisible(true);
    return true;
  };

  useEffect(() => {
    const timer = window.setInterval(() => {
      setExpiresIn((current) => {
        if (operationState !== "AWAITING_CONFIRMATION") return current;
        if (current <= 1) {
          transitionOperationState("EXPIRED");
          return 0;
        }
        return current - 1;
      });
    }, 1_000);
    return () => window.clearInterval(timer);
  }, [operationState]);

  useEffect(() => clearExecutionTimers, []);

  useEffect(() => {
    if (!toast) return;
    const timer = window.setTimeout(() => setToast(""), 2_400);
    return () => window.clearTimeout(timer);
  }, [toast]);

  const cycleSymbol = () => {
    const symbols = Object.keys(symbolProfiles) as SymbolKey[];
    const nextIndex = (symbols.indexOf(symbol) + 1) % symbols.length;
    const nextSymbol = symbols[nextIndex];
    setSymbol(nextSymbol);
    setToast(
      `图表已切换为 ${symbolProfiles[nextSymbol].pair}；待确认票据仍锁定 ${confirmationTicket.pair}，不会静默改写。`,
    );
  };

  const cycleDataState = () => {
    const nextIndex =
      (dataStates.indexOf(dataState) + 1) % dataStates.length;
    const next = dataStates[nextIndex];
    setDataState(next);
    setToast(`本地模拟数据状态已切换为 ${next}。`);
  };

  const confirmOperation = () => {
    if (
      operationStateRef.current !== "AWAITING_CONFIRMATION" ||
      killSwitchActive ||
      dataState !== "LIVE"
    ) {
      return;
    }
    transitionOperationState("RISK_REVALIDATING");
    timersRef.current = executionSequence.map(([delay, state]) =>
      window.setTimeout(() => transitionOperationState(state), delay),
    );
  };

  const closeConfirmation = () => {
    if (
      operationStateRef.current !== "AWAITING_CONFIRMATION" &&
      operationStateRef.current !== "EXPIRED"
    ) {
      return;
    }
    setConfirmationVisible(false);
    setToast("模拟确认票据已关闭；新指令将重新计算全部风险字段。");
    window.setTimeout(() => inputRef.current?.focus(), 0);
  };

  const sendMessage = (content: string) => {
    if (isOperationActive(operationStateRef.current)) {
      setToast(
        "当前 Operation 必须先完成或核对；不能生成新票据或取消现有执行。",
      );
      return false;
    }
    const effect: PositionEffect = content.includes("加仓") ? "ADD" : "OPEN";
    const normalized = content.toUpperCase();
    const mentionedSymbol = (
      Object.keys(symbolProfiles) as SymbolKey[]
    ).find((candidate) => normalized.includes(candidate));
    const intentProfile = symbolProfiles[mentionedSymbol ?? symbol];
    ticketSequenceRef.current += 1;
    const ticket = createConfirmationTicketSnapshot(
      intentProfile,
      effect,
      ticketSequenceRef.current,
    );
    setMessages((current) => [
      ...current,
      { id: current.length + 1, role: "you", content },
    ]);
    setThinking(true);
    window.setTimeout(() => {
      setThinking(false);
      const replaced = replaceConfirmation(ticket);
      setMessages((current) => [
        ...current,
        {
          id: current.length + 1,
          role: "hermes",
          content: replaced
            ? `已按 ${ticket.pair} 本地模拟上下文生成${effect === "ADD" ? "加仓" : "开仓"}意图。交易意图和全部风险字段已冻结到票据 ${ticket.ticketId}。`
            : "当前 Operation 必须先完成或核对；这条指令未生成新票据，原 Operation 继续推进。",
        },
      ]);
    }, 700);
    return true;
  };

  const activateKillSwitch = () => {
    setKillSwitchActive(true);
    setKillDialogOpen(false);
    setToast("已在本地模拟停止开仓和增加风险；保护、减仓和平仓仍可用。");
  };

  return {
    profile,
    symbol,
    timeframe,
    dataState,
    indicatorEnabled,
    activeNav,
    bottomTab,
    messages,
    thinking,
    confirmationVisible,
    confirmationTicket,
    operationState,
    expiresIn,
    killDialogOpen,
    killSwitchActive,
    toast,
    inputRef,
    canSubmitHermesInstruction: !isOperationActive(operationState),
    canIncreaseRisk: dataState === "LIVE" && !killSwitchActive,
    setTimeframe,
    setIndicatorEnabled,
    setActiveNav,
    setBottomTab,
    setKillDialogOpen,
    setToast,
    cycleSymbol,
    cycleDataState,
    confirmOperation,
    closeConfirmation,
    sendMessage,
    activateKillSwitch,
  };
}
