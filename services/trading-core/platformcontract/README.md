# Platform contract consumer

This package is a pure, strict consumer of the frozen Phase 1 contracts. It
does not provide HTTP, authentication storage, databases, NATS connectivity,
signing, wallets, exchange access, or network writes.

`go generate ./platformcontract` recreates the byte-frozen inputs, all 59
field-level contract types, all ten immutable manifest views, and the pinned
Unicode normalization tables. Tests hash every source input and run each
generator in check mode, so schema, manifest, type, or normalization drift
fails before a consumer can silently use stale mappings.

Use `Validator.Decode`, `Validator.DecodeTyped`, or a generated
`Decode<Schema>` method for raw untrusted JSON. They detect duplicate keys at
every nesting level, reject unsafe prototype keys, preserve allowed JSON nulls
and finite numbers without coercion, recursively enforce JSON Schema and
`x-fit-*` semantics, and return a sealed named value. The exact 439-property
inventory maps each path to a generated field, tagged-union disposition, or
the single transport-only `writeOnly` field.
