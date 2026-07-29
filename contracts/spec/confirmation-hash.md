# Confirmation hash v1

`fit.confirmation-hash.v1` prevents a user approval from being reused for a
different trade or risk snapshot.

## Input

1. Validate the complete `ConfirmationTicket` against
   `fit-trade-v1.schema.json#/$defs/ConfirmationTicket`.
2. Reject expired tickets before accepting a confirmation.
3. Read every dotted path from `confirmation-fields.json`. Missing paths are an
   error; no default value is inserted.
4. Build one flat JSON object whose keys are the dotted paths and whose values
   are the exact validated JSON values. `confirmation_hash` is intentionally
   excluded because it is the output.

Wire-level decimal strings are already canonical: no leading zeros, exponent,
plus sign, negative zero, or redundant fractional trailing zeros. Producers
must canonicalize before validation; consumers never silently repair an
invalid value.

## Digest

Serialize the flat object with RFC 8785 JSON Canonicalization Scheme semantics,
encode it as UTF-8 without a byte-order mark or trailing newline, then compute
SHA-256. The external form is 64 lowercase hexadecimal characters.

The golden vector is:

- input: `fixtures/valid/confirmation-ticket-open.json`
- canonical bytes: generated deterministically by `scripts/verify.mjs`
- digest: `fixtures/golden/confirmation-open.sha256`

Key insertion order must not affect the digest. A semantic mutation of any
bound field must change the digest. Textually different decimal
representations are rejected before hashing rather than treated as aliases.

## Consumption and replay rules

- Bind the digest to the authenticated user, account, device session, one-time
  nonce, and expiry.
- Consume a nonce atomically at most once.
- A duplicate submission returns the existing Operation result; it must not
  produce a second order effect.
- Any changed bound field requires a new ticket, nonce, expiry, digest, and user
  confirmation.
- Hash equality proves binding only. It does not replace authentication,
  authorization, current-data checks, deterministic risk admission, or exchange
  reconciliation.
