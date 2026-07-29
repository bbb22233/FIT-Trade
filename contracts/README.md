# FIT-Trade Contracts

`contracts/` is the coordinator-owned source of truth for Phase 0. Go, Python,
TypeScript, MCP, REST, and WebSocket implementations consume these files; they
must not redefine trading semantics independently.

## Canonical rules

- Contract version: `fit.trade.v1`
- JSON Schema dialect: Draft 2020-12
- Transport timestamps: RFC 3339 UTC strings
- IDs: UUID strings unless an external exchange ID is explicitly named
- Price, quantity, notional, margin, fee, and risk values: canonical decimal
  strings without leading or redundant trailing zeros; JSON floating-point
  values are forbidden
- Security-sensitive objects reject unknown fields
- User/account scope is injected by the authenticated server session and is
  absent from model-controlled MCP inputs
- Confirmation digests use RFC 8785 JSON Canonicalization Scheme semantics and
  SHA-256 over the exact field set in `confirmation-fields.json`

## Layout

- `jsonschema/fit-trade-v1.schema.json`: canonical domain objects
- `jsonschema/mcp-tools-v1.schema.json`: MCP inventory validation schema
- `mcp-tools-v1.json`: complete allowed MCP inventory
- `state-machines/*.json`: allowed transition graphs
- `confirmation-fields.json`: confirmation-bound field order
- `proto/fit/v1/trading.proto`: internal service contract
- `openapi/openapi.yaml`: client API contract
- `fixtures/`: shared positive and negative vectors
- `scripts/verify.mjs`: contract, state-machine, digest, and MCP checks

## Verification

```bash
npm ci
npm test
```

These checks are development-only and make no network or exchange calls.
