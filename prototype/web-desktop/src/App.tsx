import { HermesPanel } from "./components/HermesPanel";
import { MarketWorkspace } from "./components/MarketWorkspace";
import {
  GlobalStateBanner,
  KillSwitchDialog,
  LeftNavigation,
  StatusFooter,
  Toast,
  TopBar,
} from "./components/SystemChrome";
import { useCommandCenterSimulation } from "./use-command-center-simulation";

export function App() {
  const simulation = useCommandCenterSimulation();

  return (
    <div className="app-shell">
      <TopBar
        dataState={simulation.dataState}
        killSwitchActive={simulation.killSwitchActive}
        operationState={simulation.operationState}
        onCycleDataState={simulation.cycleDataState}
        onKillSwitch={() => simulation.setKillDialogOpen(true)}
      />

      <GlobalStateBanner
        dataState={simulation.dataState}
        killSwitchActive={simulation.killSwitchActive}
      />

      <main className="main-shell">
        <LeftNavigation
          active={simulation.activeNav}
          onSelect={(label) => {
            simulation.setActiveNav(label);
            if (label !== "仓位") {
              simulation.setToast(`${label}页面将在后续壳层任务中展开。`);
            }
          }}
        />

        <MarketWorkspace
          profile={simulation.profile}
          timeframe={simulation.timeframe}
          indicatorEnabled={simulation.indicatorEnabled}
          selectedTab={simulation.bottomTab}
          onCycleSymbol={simulation.cycleSymbol}
          onTimeframe={simulation.setTimeframe}
          onToggleIndicator={() =>
            simulation.setIndicatorEnabled((current) => !current)
          }
          onTab={simulation.setBottomTab}
          onAction={simulation.setToast}
        />

        <HermesPanel
          profile={simulation.profile}
          dataState={simulation.dataState}
          messages={simulation.messages}
          thinking={simulation.thinking}
          confirmationVisible={simulation.confirmationVisible}
          operationState={simulation.operationState}
          positionEffect={simulation.positionEffect}
          expiresIn={simulation.expiresIn}
          canIncreaseRisk={simulation.canIncreaseRisk}
          inputRef={simulation.inputRef}
          onSubmit={simulation.sendMessage}
          onConfirm={simulation.confirmOperation}
          onEdit={simulation.closeConfirmation}
        />
      </main>

      <StatusFooter
        dataState={simulation.dataState}
        killSwitchActive={simulation.killSwitchActive}
      />

      <KillSwitchDialog
        open={simulation.killDialogOpen}
        onClose={() => simulation.setKillDialogOpen(false)}
        onActivate={simulation.activateKillSwitch}
      />

      <Toast message={simulation.toast} />
    </div>
  );
}
