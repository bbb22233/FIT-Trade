# Protection coverage invariant v1

`PROTECTED` is an aggregate assertion, not a label that may be trusted from one
JSON object.

The authoritative validator must load a `PositionSnapshot` and its referenced
`ProtectionStatus`, then enforce `PROTECTION_FULL_COVERAGE` from
`semantic-invariants-v1.json` using exact decimal arithmetic:

1. `protection_status_id` and `position_id` match.
2. Both state fields are `PROTECTED`.
3. The absolute signed position quantity equals the protection object's
   `absolute_live_position_quantity`.
4. At least one active Stop Market order is recorded.
5. A lowercase SHA-256 evidence hash binds the reconciled exchange evidence.

The wire contract intentionally has no separate `protected_quantity` field.
That field allowed a contradictory payload such as live quantity `1`,
protected quantity `0`, and state `PROTECTED`. Any legacy payload containing it
is rejected as an unknown field.

JSON Schema validates each object structurally; it cannot compare decimal
values across two objects. Go and Python consumers must therefore run the
shared aggregate invariant after Schema validation and before treating a
position as protected or allowing an increase in risk. Failure transitions to
`PROTECTION_FAILED`; it never silently downgrades to a warning.
