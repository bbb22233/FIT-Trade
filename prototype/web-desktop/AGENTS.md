# Prototype Instructions

Run the local server yourself and open the preview in the browser available to this environment. Do not give the user server-start instructions when you can run it.

Before making substantial visual changes, use the Product Design plugin's `get-context` skill when the visual source is unclear or no longer matches the current goal. When the user gives durable prototype-specific design feedback, preferences, or decisions, record them in `AGENTS.md`.

When implementing from a selected generated mock, treat that image as the source of truth for layout, component anatomy, density, spacing, color, typography, visible content, and hierarchy.

Build app UI in `src/`. Keep `.openai/hosting.json`, `worker/index.js`, `scripts/prepare-sites-build.mjs`, and `tests/sites-worker.test.mjs` intact so the same local prototype can be handed to Sites. Before a Sites handoff, run `npm run build` and `npm run test:sites`; the build must leave `dist/client/index.html`, `dist/server/index.js`, and `dist/.openai/hosting.json`.

## Approved Product Direction

- User approval: the interactive desktop main screen was accepted on 2026-07-28 and is the baseline for subsequent frontend work.
- Selected visual target: `reference/institutional-risk-desk.png`.
- Desktop layout: chart-centered trading terminal with Hermes permanently visible on the right.
- Information density: professional medium-high density.
- Market colors: green up, red down; never rely on color alone.
- Theme: dark graphite first, with future light-theme tokens kept possible.
- Kill Switch: stops opening and increasing risk only; never imply that it closes all positions.
- Real-money confirmation: server-authored structured ticket, visually separate from Hermes prose.
- The first implementation is a local simulated prototype only. It must not call an exchange, wallet, signer, model API, or production backend.
