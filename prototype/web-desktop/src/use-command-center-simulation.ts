import { useEffect, useMemo, useRef, useState } from "react";
import {
  symbolProfiles,
  type SymbolKey,
  type Timeframe,
} from "./market-data";
import type {
  ChatMessage,
  DataState,
  OperationState,
  PositionEffect,
} from "./simulation-types";

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
  const [positionEffect, setPositionEffect] =
    useState<PositionEffect>("OPEN");
  const [operationState, setOperationState] =
    useState<OperationState>("AWAITING_CONFIRMATION");
  const [expiresIn, setExpiresIn] = useState(600);
  const [killDialogOpen, setKillDialogOpen] = useState(false);
  const [killSwitchActive, setKillSwitchActive] = useState(false);
  const [toast, setToast] = useState("");
  const inputRef = useRef<HTMLInputElement>(null);
  const timersRef = useRef<number[]>([]);
  const profile = useMemo(() => symbolProfiles[symbol], [symbol]);

  const clearExecutionTimers = () => {
    timersRef.current.forEach((timer) => window.clearTimeout(timer));
    timersRef.current = [];
  };

  const resetConfirmation = (effect: PositionEffect = positionEffect) => {
    clearExecutionTimers();
    setPositionEffect(effect);
    setOperationState("AWAITING_CONFIRMATION");
    setExpiresIn(600);
    setConfirmationVisible(true);
  };

  useEffect(() => {
    const timer = window.setInterval(() => {
      setExpiresIn((current) => {
        if (operationState !== "AWAITING_CONFIRMATION") return current;
        if (current <= 1) {
          setOperationState("EXPIRED");
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
    setSymbol(symbols[nextIndex]);
    resetConfirmation(positionEffect);
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
      operationState !== "AWAITING_CONFIRMATION" ||
      killSwitchActive ||
      dataState !== "LIVE"
    ) {
      return;
    }
    setOperationState("RISK_REVALIDATING");
    timersRef.current = executionSequence.map(([delay, state]) =>
      window.setTimeout(() => setOperationState(state), delay),
    );
  };

  const closeConfirmation = () => {
    if (
      operationState !== "AWAITING_CONFIRMATION" &&
      operationState !== "EXPIRED"
    ) {
      return;
    }
    setConfirmationVisible(false);
    setToast("模拟确认票据已关闭；新指令将重新计算全部风险字段。");
    window.setTimeout(() => inputRef.current?.focus(), 0);
  };

  const sendMessage = (content: string) => {
    const effect: PositionEffect = content.includes("加仓") ? "ADD" : "OPEN";
    setMessages((current) => [
      ...current,
      { id: current.length + 1, role: "you", content },
    ]);
    setThinking(true);
    window.setTimeout(() => {
      setMessages((current) => [
        ...current,
        {
          id: current.length + 1,
          role: "hermes",
          content: `已按 ${profile.pair} 本地模拟上下文生成${effect === "ADD" ? "加仓" : "开仓"}意图。模拟 Go 风控会重新计算仓位、止损与最大亏损。`,
        },
      ]);
      setThinking(false);
      resetConfirmation(effect);
    }, 700);
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
    positionEffect,
    operationState,
    expiresIn,
    killDialogOpen,
    killSwitchActive,
    toast,
    inputRef,
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
