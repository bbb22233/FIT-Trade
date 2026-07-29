# Secrets and logging baseline

## Secret handling

- Never store API keys, wallet keys, model tokens, passwords, session cookies,
  confirmation nonces, or device private keys in Git, prompts, fixtures, crash
  dumps, screenshots, analytics, or ordinary logs.
- Development configuration contains names and placeholders only.
- Later runtime secrets must come from a least-privilege secret provider or a
  strict-permission environment file outside the repository.
- The read-only market connector and signing executor use different identities.
- The executor must never accept arbitrary signing payloads.

## Structured logging

Allowed identifiers are correlation ID, operation ID, attempt ID, event type,
state transition, stable error code, component, and redacted latency metadata.
User text, full MCP arguments, full request/response bodies, account addresses,
headers, cookies, authorization values, and confirmation nonces are denied by
default.

Redaction replaces a sensitive value with `[REDACTED]`; it does not hash a
low-entropy password or token. Unknown object fields are dropped rather than
logged.

## CI baseline

Phase 0 verification scans tracked candidate files for private-key markers,
credential assignments, bearer tokens, wallet seed phrases, and common secret
formats. The scan is a backstop, not permission to place real secrets in tests.
Any match fails closed and requires human review.
