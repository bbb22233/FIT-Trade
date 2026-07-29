# Design QA — Hermes Command

## Scope

- Source of truth: `reference/institutional-risk-desk.png`
- Implementation: `http://localhost:4173/`
- Screen under test: desktop main command center, default `BTC-PERP / 1h / AWAITING_CONFIRMATION`
- Comparison method: source and browser-rendered implementation were captured at matching dimensions, then reviewed together in `qa-compare.html`

## Capture evidence

| Evidence | Pixels | CSS viewport | Device scale factor |
| --- | ---: | ---: | ---: |
| Source | 1487 × 1058 | 1487 × 1058 target | 1 |
| Implementation v1 | 1487 × 1058 | 1487 × 1058 | 1 |
| Implementation v2 | 1487 × 1058 | 1487 × 1058 | 1 |

Files:

- `reference/institutional-risk-desk.png`
- `qa-implementation-v1.png`
- `qa-implementation-v2.png`
- `qa-compare.html`

No focused-region capture was needed: every persistent region—the top status track, left navigation, chart, position table, Hermes rail, confirmation ticket, composer, and footer—was visible in the matched full-frame captures.

## Comparison history

### Iteration 1

Findings:

- [P2] The initial chart moved upward too uniformly and did not preserve the reference’s alternating impulse, pullback, and recovery rhythm.
- [P2] The Hermes conversation row was too short, placing the confirmation ticket materially higher than in the reference.

Fixes:

- Reworked deterministic sample candles around a multi-segment market path with visible pullbacks, recovery legs, wick range, and volume response.
- Set the Hermes conversation row to match the reference’s ticket start line while preserving a visible built-in chat composer.

Post-fix evidence:

- `qa-implementation-v2.png`
- Side-by-side view in `qa-compare.html`

Result after re-comparison:

- P0: 0
- P1: 0
- P2: 0

Intentional product-driven differences:

- The reference’s large institutional account values were replaced with a realistic 10,000 USDC personal-demo account.
- A persistent chat composer is retained because built-in Hermes chat is an approved requirement.
- All displayed market and account values remain deterministic sample data; no exchange, signer, wallet, model, or production API is called.

## Primary interactions tested

- BTC → ETH symbol cycle updates market header, chart context, position context, and confirmation ticket together.
- `1h` → `4h` changes the selected timeframe and chart data.
- Sending a Hermes command appends the user message, returns a simulated Hermes response, and regenerates the structured confirmation.
- Confirming an opening order enters server risk revalidation, disables repeat submission, advances through dispatch/reconciliation/protection, and ends at verified `PROTECTED`.
- The safety control requires a second explicit action, disables new risk, keeps reduce-only exits and existing protection available, and does not imply automatic liquidation.

## Runtime checks

- Browser console warnings/errors: 0
- Hidden persistent controls at 1487 × 1058: 0
- Keyboard-visible focus treatment: present
- Color-only market/risk meaning: avoided with text, signs, labels, and icons
- Core layout overflow: none

## Final

final result: passed
