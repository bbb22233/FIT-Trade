# Design QA — Frontend Command Center Shell

## Scope

- Source visual truth: `reference/institutional-risk-desk.png`
- Browser-rendered implementation: `qa-command-center-shell.png`
- Combined comparison: `qa-command-center-shell-compare.png`
- Interactive implementation: `http://localhost:4174/`
- Screen state: `BTC-PERP / 1h / LIVE / AWAITING_CONFIRMATION`
- Theme: dark graphite

This QA verifies the local simulated frontend shell only. It does not verify a
wallet, model, exchange, trading core, production backend, deployment, or live
authorization.

## Capture evidence

| Evidence | Pixels | CSS viewport | Device scale factor |
| --- | ---: | ---: | ---: |
| Source | 1487 × 1058 | 1487 × 1058 target | 1 |
| Implementation | 1487 × 1058 | 1487 × 1058 | 1 |
| Combined browser comparison | 1487 × 1058 | 1487 × 1058 | 1 |

The source and implementation use the same full-frame dimensions. No density
normalization or crop adjustment was required.

## Full-view comparison

The combined comparison preserves the selected visual hierarchy:

- persistent top status track;
- fixed left navigation;
- chart-centered workspace;
- lower position and account panel;
- permanently visible Hermes rail;
- separate structured risk ticket;
- fixed footer status line.

The implementation adds persistent `本地模拟`, `模拟行情`, `SIM`, and
`MAINNET 演练` labels. These are intentional safety constraints and do not
change the major-region proportions.

## Focused-region comparison

No additional focused crop was required. At the matched 1487 × 1058 frame, the
top status track, chart toolbar, position row, confirmation risk fields,
Hermes conversation, composer, and Kill Switch were all readable.

## Required fidelity surfaces

### Fonts and typography

- Inter remains the UI font and Roboto Mono remains the numeric/status font.
- Numeric columns use tabular, right-aligned values.
- The implementation retains the reference hierarchy for price, risk, order
  fields, labels, and status text.
- Simulation labels use the same mono/status vocabulary without competing with
  primary risk values.

### Spacing and layout rhythm

- Major grid tracks match the selected direction.
- The confirmation ticket begins at the same visual level as the source.
- The chat composer remains visible as an approved product requirement.
- No persistent control is hidden at the captured viewport.

### Colors and visual tokens

- Dark graphite surfaces, green-up/red-down market semantics, amber MAINNET
  state, purple Hermes accents, and red risk-stop controls remain consistent.
- Direction and status are also communicated with labels, icons, and signs;
  color is not the only signal.

### Image and asset quality

- The chart is rendered by `lightweight-charts`, not a placeholder raster.
- UI icons use the installed Phosphor icon library.
- No new raster, CSS-drawn, inline SVG, emoji, or placeholder asset replaces a
  visible source asset.

### Copy and content

- All market, account, chat, confirmation, execution, and footer values are
  visibly marked as local simulation.
- The confirmation remains structurally equivalent to a real-money ticket but
  its action states that it will not place an order.
- Kill Switch copy explicitly says it stops opening and increasing risk and
  does not imply liquidation.

## Interaction verification

Browser checks completed:

1. BTC → ETH → SOL → BTC updates the market header, chart, position, and ticket.
2. `1h` → `4h` updates the selected timeframe and chart dataset.
3. `LIVE` → `STALE` disables confirmation and displays the stale-data boundary.
4. `STALE` → `RECONCILING` keeps confirmation disabled and displays the
   reconciliation boundary.
5. `RECONCILING` → `LIVE` restores confirmation eligibility.
6. A Hermes message containing `加仓` generates a structured `BTC 加仓确认`
   ticket.
7. Confirmation advances through simulated risk revalidation, dispatch,
   acknowledgement, protection pending, and verified `PROTECTED`.
8. Repeated confirmation is disabled while the simulated Operation is active.
9. Kill Switch requires a second explicit action, stops new risk, leaves
   reduce-only `减仓` available, and exposes no one-click all-close action.

App-origin browser console errors or warnings: 0. Chrome-extension-origin
warnings were observed and excluded from the application result.

## Comparison history

### Earlier visual iterations

The existing v1/v2 history corrected chart path rhythm and Hermes ticket
vertical placement. Those earlier findings remain closed.

### Command-center shell iteration

Changes reviewed:

- split the single-file prototype into system chrome, market workspace, Hermes
  panel, shared simulation types, and a local simulation state hook;
- added persistent simulation disclosure;
- added `LIVE / STALE / RECONCILING` simulation controls and fail-closed
  confirmation behavior;
- added open/add confirmation variants and an explicit simulated execution
  state machine;
- separated Kill Switch state from automatic-trading state.

Post-change findings:

- P0: 0
- P1: 0
- P2: 0

Intentional differences from the visual target:

- personal-demo values replace institutional account values;
- simulation disclosure is more prominent;
- the chat composer remains permanently available;
- confirmation language states that no order is sent.

## Follow-up polish

- P3: the compact simulation badges could receive optical spacing refinement
  after the real server status contracts are available.

## Final

final result: passed
