# Phase 0 threat model

## Assets and trust boundaries

Protected assets are account ownership, confirmation nonces, device/session
bindings, risk policy versions, operation and attempt identifiers, audit
integrity, and—only in later phases—isolated signing material.

Untrusted inputs include user text, model output, MCP arguments, client payloads,
exchange responses, WebSocket frames, queue messages, timestamps, and all
replayed or duplicated messages. Schema-valid data is not automatically
authorized. Authorized data is not automatically admitted by risk controls.

## Required controls

| Threat | Required control | Fail-closed result |
| --- | --- | --- |
| Prompt injection selects another account | Authenticated server injects user, account, and session scope; MCP schemas forbid those fields. | `SCOPE_MISMATCH` or `VALIDATION_FAILED` |
| Model invokes raw execution or signing | Exact MCP inventory; no generic URL, SQL, shell, signer, transfer, withdrawal, or raw-order tool. | `TOOL_NOT_AUTHORIZED` |
| Confirmation is changed or replayed | Exact bound-field digest, expiry, device/session binding, and atomic one-time nonce consumption. | Mismatch, expired, or existing result |
| Float or cross-language ambiguity changes risk | Canonical decimal strings and shared golden vectors; no financial floating point. | `VALIDATION_FAILED` |
| Timeout causes duplicate order | Immutable Operation plus unique Attempt and client-order IDs; unknown results enter reconciliation. | `UNKNOWN_REQUIRES_RECONCILIATION` |
| Stale state admits unsafe risk | Snapshot IDs, timestamps, freshness gates, and risk revalidation after confirmation. | `DATA_STALE` or `RISK_REJECTED` |
| Partial fill remains unprotected | Protection state machine and coverage invariant; emergency reduce-only path on failure. | `PROTECTION_FAILED` |
| Logs or fixtures expose secrets | Structured allowlist logging, redaction tests, secret scanning, synthetic fixtures. | Drop/redact field and fail CI |
| Supply-chain change alters contracts | Locked dependencies, fixed-commit review, generated-drift check, and signed release process later. | Build/review failure |

## Phase 0 non-capabilities

There is no wallet, signer, private-key reader, exchange write client,
production model call, deployment path, or automatic trading path in Phase 0.
Tests use only synthetic identifiers and local files.

## Residual risks

Phase 0 proves contract and pure-domain behavior only. It does not prove
exchange semantics, database atomicity, queue delivery, device attestation,
production secret custody, or safe live execution. Those require separate
phase gates and explicit authorization.
