"""Adversarial differential tests: JSON Schema vs Pydantic across all 19 matrix entries.

Systematically mutates required fields, adds unknown fields, mutates enums,
violates array min/max/unique rules, conditionals, formats, and decimal
representation. Compares accept/reject results of JSON Schema versus Pydantic.
The goal is no known semantic drift.

If an edge is intentionally not representable in Pydantic, it must fail closed
and be documented here.
"""

import json
from pathlib import Path

import pytest

from hermes_agent.validator import (
    load_matrix_values,
    validate_with_pydantic,
    validate_with_jsonschema,
)

MATRIX = load_matrix_values()


def _matrix_entries():
    """Yield (name, value) for all 19 non-meta matrix entries."""
    for name, value in MATRIX.items():
        if name.startswith("_"):
            continue
        yield name, value


MATRIX_NAMES = [name for name in MATRIX if not name.startswith("_")]


# ---------------------------------------------------------------------------
# 1. All valid matrix values must pass both validators
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("name,value", _matrix_entries())
def test_matrix_valid_passes_jsonschema(name, value):
    """Every frozen matrix value must pass JSON Schema validation."""
    validate_with_jsonschema(name, value)


@pytest.mark.parametrize("name,value", _matrix_entries())
def test_matrix_valid_passes_pydantic(name, value):
    """Every frozen matrix value must pass Pydantic validation."""
    validate_with_pydantic(name, value)


# ---------------------------------------------------------------------------
# 2. Required field removal — both reject
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("name,value", _matrix_entries())
def test_missing_required_field_rejected_jsonschema(name, value):
    """Removing a required field must be rejected by JSON Schema."""
    from hermes_agent.contracts import DOMAIN_TYPES
    model = DOMAIN_TYPES.get(name)
    if model is None:
        pytest.skip(f"no Pydantic model for {name}")
    required_fields = [
        f for f in model.model_fields if model.model_fields[f].is_required()
    ]
    if not required_fields:
        pytest.skip(f"no required fields for {name}")
    for field_to_remove in required_fields:
        modified = json.loads(json.dumps(value))
        del modified[field_to_remove]
        with pytest.raises(Exception):
            validate_with_jsonschema(name, modified)


@pytest.mark.parametrize("name,value", _matrix_entries())
def test_missing_required_field_rejected_pydantic(name, value):
    """Removing a required field must be rejected by Pydantic."""
    from hermes_agent.contracts import DOMAIN_TYPES
    model = DOMAIN_TYPES.get(name)
    if model is None:
        pytest.skip(f"no Pydantic model for {name}")
    required_fields = [
        f for f in model.model_fields if model.model_fields[f].is_required()
    ]
    if not required_fields:
        pytest.skip(f"no required fields for {name}")
    for field_to_remove in required_fields:
        modified = json.loads(json.dumps(value))
        del modified[field_to_remove]
        with pytest.raises(Exception):
            validate_with_pydantic(name, modified)


# ---------------------------------------------------------------------------
# 3. Unknown field injection — both reject
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("name,value", _matrix_entries())
def test_unknown_field_rejected_jsonschema(name, value):
    """Adding an unknown field must be rejected by JSON Schema."""
    modified = json.loads(json.dumps(value))
    modified["__bogus_field__"] = "injected"
    with pytest.raises(Exception):
        validate_with_jsonschema(name, modified)


@pytest.mark.parametrize("name,value", _matrix_entries())
def test_unknown_field_rejected_pydantic(name, value):
    """Adding an unknown field must be rejected by Pydantic."""
    modified = json.loads(json.dumps(value))
    modified["__bogus_field__"] = "injected"
    with pytest.raises(Exception):
        validate_with_pydantic(name, modified)


# ---------------------------------------------------------------------------
# 4. Enum mutation — known bad values
# ---------------------------------------------------------------------------

ENUM_FIELDS = {
    "TradeIntent": [
        ("network", "BSC_MAINNET"),
        ("symbol", "DOGE-PERP"),
        ("side", "BUY_AND_HOLD"),
        ("position_effect", "MAX_LEVERAGE"),
        ("order_type", "TRAILING_STOP"),
        ("time_in_force", "FOK"),
        ("margin_mode", "PORTFOLIO"),
        ("source", "ANONYMOUS_TIP"),
    ],
    "ConfirmationTicket": [],
    "Operation": [("state", "EXECUTING")],
    "ExecutionAttempt": [("state", "COMPLETED")],
    "Order": [("state", "PARTIALLY_CANCELLED"), ("side", "NONE")],
    "Fill": [("side", "NONE"), ("symbol", "ADA-PERP")],
    "PositionSnapshot": [
        ("protection_state", "BROKEN"),
        ("data_status", "CORRUPTED"),
        ("symbol", "XRP-PERP"),
    ],
    "RiskPolicy": [
        ("allowed_symbols", ["RANDOM-PERP"]),
    ],
    "AutomationGrant": [
        ("state", "ACTIVE"),  # old value no longer in schema
        ("allowed_symbols", ["UNKNOWN-PERP"]),
    ],
    "ModelProposal": [("decision", "MAYBE")],
    "ModelReview": [
        ("decision", "HOLD"),
        ("reason_codes", ["bad_reason"]),
    ],
    "RiskDecision": [
        ("result", "PENDING"),
        ("reason_codes", ["bad_code"]),
    ],
    "ExecutionCommand": [
        ("action", "WITHDRAW"),
        ("authorization_type", "ADMIN_OVERRIDE"),
    ],
    "ExecutionResult": [
        ("status", "SUCCESS"),
    ],
    "ProtectionStatus": [
        ("state", "LOST"),
        ("data_status", "UNKNOWN"),
    ],
    "ReconciliationStatus": [
        ("state", "RESOLVED"),
        ("evidence_sources", ["TWITTER"]),
    ],
    "AgentFeedback": [],
    "AutomationAuthorization": [],
    "AuditEvent": [
        ("actor_type", "HACKER"),
        ("subject_type", "WALLET"),
    ],
}


@pytest.mark.parametrize(
    "name,field,invalid_value",
    [
        (name, field, val)
        for name in MATRIX_NAMES
        for field, val in ENUM_FIELDS.get(name, [])
    ],
)
def test_enum_mutation_rejected_jsonschema(name, field, invalid_value):
    """Mutating enum fields to invalid values must be rejected by JSON Schema."""
    value = MATRIX[name]
    modified = json.loads(json.dumps(value))
    modified[field] = invalid_value
    with pytest.raises(Exception):
        validate_with_jsonschema(name, modified)


@pytest.mark.parametrize(
    "name,field,invalid_value",
    [
        (name, field, val)
        for name in MATRIX_NAMES
        for field, val in ENUM_FIELDS.get(name, [])
    ],
)
def test_enum_mutation_rejected_pydantic(name, field, invalid_value):
    """Mutating enum fields to invalid values must be rejected by Pydantic."""
    value = MATRIX[name]
    modified = json.loads(json.dumps(value))
    modified[field] = invalid_value
    with pytest.raises(Exception):
        validate_with_pydantic(name, modified)


# ---------------------------------------------------------------------------
# 5. Decimal format mutations
# ---------------------------------------------------------------------------


DECIMAL_FIELDS = {
    "TradeIntent": [
        ("quantity", "0123"),          # leading zero
        ("quantity", "1.0"),           # trailing zero
        ("notional", "-100.5"),        # negative positive-only
        ("entry_price_or_bound", "1e5"),  # exponential
        ("worst_acceptable_price", "1.000"),  # trailing zeros
    ],
    "ConfirmationTicket": [
        ("maximum_loss", "-1"),        # negative
        ("liquidation_price", "0"),    # zero for positive
        ("estimated_fees", "00"),      # leading zero
    ],
    "Order": [
        ("quantity", "0"),             # zero for positive
        ("filled_quantity", "-0.5"),   # negative
    ],
    "Fill": [
        ("quantity", "abc"),           # not a number
        ("price", ""),                 # empty
    ],
    "PositionSnapshot": [
        ("entry_price", "1.0"),        # trailing zero
    ],
    "RiskPolicy": [
        ("maximum_trade_risk_fraction", "2.0"),  # >1
        ("maximum_slippage_fraction", "-0.1"),   # negative
    ],
    "AutomationGrant": [
        ("maximum_notional", "ten"),   # not a number
    ],
    "RiskDecision": [
        ("admitted_quantity", "-0.1"),
        ("maximum_loss", "NaN"),
    ],
    "ExecutionCommand": [
        ("quantity", "1.00"),
        ("worst_acceptable_price", "1.00"),
    ],
    "ProtectionStatus": [
        ("absolute_live_position_quantity", "-0.1"),
    ],
    "AutomationAuthorization": [
        ("maximum_notional", "0"),     # zero for positive
    ],
}


@pytest.mark.parametrize(
    "name,field,invalid_value",
    [
        (name, field, val)
        for name in MATRIX_NAMES
        for field, val in DECIMAL_FIELDS.get(name, [])
    ],
)
def test_decimal_mutation_rejected_jsonschema(name, field, invalid_value):
    """Mutating decimal fields must be rejected by JSON Schema."""
    value = MATRIX[name]
    modified = json.loads(json.dumps(value))
    modified[field] = invalid_value
    with pytest.raises(Exception):
        validate_with_jsonschema(name, modified)


@pytest.mark.parametrize(
    "name,field,invalid_value",
    [
        (name, field, val)
        for name in MATRIX_NAMES
        for field, val in DECIMAL_FIELDS.get(name, [])
    ],
)
def test_decimal_mutation_rejected_pydantic(name, field, invalid_value):
    """Mutating decimal fields must be rejected by Pydantic."""
    value = MATRIX[name]
    modified = json.loads(json.dumps(value))
    modified[field] = invalid_value
    with pytest.raises(Exception):
        validate_with_pydantic(name, modified)


# ---------------------------------------------------------------------------
# 6. UUID format mutations
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("name,value", _matrix_entries())
def test_uuid_format_mutation_rejected_jsonschema(name, value):
    """Mutating UUID fields must be rejected by JSON Schema (format: uuid)."""
    # Find a UUID field
    modified = json.loads(json.dumps(value))

    uuid_fields = []
    def _find_uuids(obj, prefix=""):
        if isinstance(obj, dict):
            for k, v in obj.items():
                new_prefix = f"{prefix}.{k}" if prefix else k
                if isinstance(v, str) and len(v) == 36 and "-" in v:
                    uuid_fields.append(new_prefix)
                elif isinstance(v, dict):
                    _find_uuids(v, new_prefix)

    _find_uuids(modified)

    if not uuid_fields:
        pytest.skip(f"no UUID fields found in {name}")

    # Try mutating the first UUID field
    field = uuid_fields[0]
    parts = field.split(".")
    target = modified
    for part in parts[:-1]:
        target = target[part]
    target[parts[-1]] = "not-a-uuid-string"
    with pytest.raises(Exception):
        validate_with_jsonschema(name, modified)


@pytest.mark.parametrize("name,value", _matrix_entries())
def test_uuid_format_mutation_rejected_pydantic(name, value):
    """Mutating UUID fields must be rejected by Pydantic."""
    modified = json.loads(json.dumps(value))

    uuid_fields = []
    def _find_uuids(obj, prefix=""):
        if isinstance(obj, dict):
            for k, v in obj.items():
                new_prefix = f"{prefix}.{k}" if prefix else k
                if isinstance(v, str) and len(v) == 36 and "-" in v:
                    uuid_fields.append(new_prefix)
                elif isinstance(v, dict):
                    _find_uuids(v, new_prefix)

    _find_uuids(modified)

    if not uuid_fields:
        pytest.skip(f"no UUID fields found in {name}")

    field = uuid_fields[0]
    parts = field.split(".")
    target = modified
    for part in parts[:-1]:
        target = target[part]
    target[parts[-1]] = "not-a-uuid-string"
    with pytest.raises(Exception):
        validate_with_pydantic(name, modified)


# ---------------------------------------------------------------------------
# 7. Timestamp format mutations — adversarial tests across every
#    timestamp-bearing domain type.
#
# The jsonschema library's Draft202012Validator.FORMAT_CHECKER includes
# the full 'date-time' format checker (RFC 3339 §5.6).  Both JSON Schema
# and Pydantic must reject invalid timestamps.  We test:
#   - no zone (missing Z/z)
#   - numeric offset (even +00:00 is rejected per project UTC-only rule)
#   - invalid date (Feb 30, month 13, etc.)
#   - invalid time (25:00, 23:60 outside leap-second)
#   - type coercion (null, number, object)
#
# Acceptable UTC forms: YYYY-MM-DD[ Tt]HH:MM:SS[.f+][Zz]
# Leap second 23:59:60Z is valid.
# ---------------------------------------------------------------------------

# Timestamp fields per domain type (from contracts.py)
_TIMESTAMP_FIELDS = {
    "TradeIntent": ["created_at"],
    "ConfirmationTicket": ["expires_at", "created_at"],
    "Operation": ["created_at", "updated_at"],
    "ExecutionAttempt": ["created_at"],
    "Order": ["updated_at"],
    "Fill": ["occurred_at"],
    "PositionSnapshot": ["snapshot_at"],
    "RiskPolicy": ["created_at"],
    # AutomationGrant has no timestamp fields
    "ModelProposal": ["created_at"],
    "ModelReview": ["created_at"],
    "RiskDecision": ["decided_at"],
    "ExecutionCommand": ["created_at"],
    "ExecutionResult": ["observed_at"],
    "ProtectionStatus": ["observed_at"],
    "ReconciliationStatus": ["checked_at"],
    "AgentFeedback": ["created_at"],
    "AutomationAuthorization": ["authorized_at"],
    "AuditEvent": ["occurred_at"],
}

# Bad timestamp mutations to test rejection
_BAD_TIMESTAMPS = [
    ("2026-07-29T04:00:00", "no zone"),
    ("2026-07-29T04:00:00+00:00", "numeric offset +00:00"),
    ("2026-07-29T04:00:00+05:30", "numeric offset +05:30"),
    ("2026-02-30T04:00:00Z", "Feb 30 invalid date"),
    ("2026-13-01T04:00:00Z", "month 13"),
    ("2026-07-29T25:00:00Z", "hour 25"),
    ("2026-07-29T04:60:00Z", "minute 60"),
    ("2026-07-29T04:00:61Z", "second 61"),
    ("2026-07-29T04:00:60Z", "leap second at wrong time"),  # only 23:59:60 is valid
    ("not-a-date-at-all", "garbage"),
    ("20260729T040000Z", "missing dashes"),
]

# Valid UTC timestamps that both validators must accept
_VALID_UTC_TIMESTAMPS = [
    "2026-07-29T04:00:00Z",
    "2026-07-29t04:00:00z",
    "2026-07-29 04:00:00Z",
    "2026-07-29T04:00:00.123456Z",
    "2026-06-30T23:59:60Z",  # leap second
]


@pytest.mark.parametrize(
    "name,field,bad_ts,reason",
    [
        (name, field, ts, reason)
        for name in MATRIX_NAMES
        for field in _TIMESTAMP_FIELDS.get(name, [])
        for ts, reason in _BAD_TIMESTAMPS
    ],
)
def test_bad_timestamp_rejected_jsonschema(name, field, bad_ts, reason):
    """Invalid timestamps must be rejected by JSON Schema format checker."""
    value = MATRIX[name]
    modified = json.loads(json.dumps(value))
    modified[field] = bad_ts
    with pytest.raises(Exception):
        validate_with_jsonschema(name, modified)


@pytest.mark.parametrize(
    "name,field,bad_ts,reason",
    [
        (name, field, ts, reason)
        for name in MATRIX_NAMES
        for field in _TIMESTAMP_FIELDS.get(name, [])
        for ts, reason in _BAD_TIMESTAMPS
    ],
)
def test_bad_timestamp_rejected_pydantic(name, field, bad_ts, reason):
    """Invalid timestamps must be rejected by Pydantic strict validator."""
    value = MATRIX[name]
    modified = json.loads(json.dumps(value))
    modified[field] = bad_ts
    with pytest.raises(Exception):
        validate_with_pydantic(name, modified)


@pytest.mark.parametrize(
    "name,field,valid_ts",
    [
        (name, field, ts)
        for name in MATRIX_NAMES
        for field in _TIMESTAMP_FIELDS.get(name, [])
        for ts in _VALID_UTC_TIMESTAMPS
    ],
)
def test_valid_utc_timestamp_accepted_pydantic(name, field, valid_ts):
    """Valid UTC timestamps in various accepted forms must pass Pydantic."""
    value = MATRIX[name]
    modified = json.loads(json.dumps(value))
    modified[field] = valid_ts
    validate_with_pydantic(name, modified)  # should not raise


@pytest.mark.parametrize("name,value", _matrix_entries())
def test_timestamp_type_coercion_rejected_jsonschema(name, value):
    """Passing null or number where a timestamp string is expected must fail."""
    fields = _TIMESTAMP_FIELDS.get(name, [])
    if not fields:
        pytest.skip(f"no timestamp fields in {name}")
    modified = json.loads(json.dumps(value))
    modified[fields[0]] = None
    with pytest.raises(Exception):
        validate_with_jsonschema(name, modified)


@pytest.mark.parametrize("name,value", _matrix_entries())
def test_timestamp_type_coercion_rejected_pydantic(name, value):
    """Passing null or number where a timestamp string is expected must fail."""
    fields = _TIMESTAMP_FIELDS.get(name, [])
    if not fields:
        pytest.skip(f"no timestamp fields in {name}")
    modified = json.loads(json.dumps(value))
    modified[fields[0]] = None
    with pytest.raises(Exception):
        validate_with_pydantic(name, modified)


# ---------------------------------------------------------------------------
# (old test_timestamp_mutation_rejected_pydantic replaced by the parameterized
#  tests above — removed)
# ---------------------------------------------------------------------------


# ---------------------------------------------------------------------------
# 8. Array constraint violations (min/max length, uniqueItems)
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("name,value", _matrix_entries())
def test_duplicate_array_items_rejected_jsonschema(name, value):
    """Duplicate items in uniqueItems arrays must be rejected by JSON Schema."""
    modified = json.loads(json.dumps(value))

    unique_array_fields = {
        "RiskPolicy": ["allowed_symbols"],
        "AutomationGrant": ["allowed_symbols"],
        "AutomationAuthorization": ["allowed_symbols"],
        "RiskDecision": ["reason_codes"],
        "ProtectionStatus": ["active_stop_order_ids"],
        "ReconciliationStatus": ["evidence_sources"],
    }

    fields_to_try = unique_array_fields.get(name, [])
    mutated = False
    for field in fields_to_try:
        arr = modified.get(field, [])
        if arr and len(arr) >= 1:
            # Duplicate by appending first item
            modified[field] = arr + [arr[0]]
            mutated = True
            break

    if not mutated:
        pytest.skip(f"no uniqueItems array to mutate in {name}")

    with pytest.raises(Exception):
        validate_with_jsonschema(name, modified)


@pytest.mark.parametrize("name,value", _matrix_entries())
def test_duplicate_array_items_rejected_pydantic(name, value):
    """Duplicate items in uniqueItems arrays must be rejected by Pydantic."""
    modified = json.loads(json.dumps(value))

    unique_array_fields = {
        "RiskPolicy": ["allowed_symbols"],
        "AutomationGrant": ["allowed_symbols"],
        "AutomationAuthorization": ["allowed_symbols"],
        "RiskDecision": ["reason_codes"],
        "ProtectionStatus": ["active_stop_order_ids"],
        "ReconciliationStatus": ["evidence_sources"],
    }

    fields_to_try = unique_array_fields.get(name, [])
    mutated = False
    for field in fields_to_try:
        arr = modified.get(field, [])
        if arr and len(arr) >= 1:
            modified[field] = arr + [arr[0]]
            mutated = True
            break

    if not mutated:
        pytest.skip(f"no uniqueItems array to mutate in {name}")

    with pytest.raises(Exception):
        validate_with_pydantic(name, modified)


# ---------------------------------------------------------------------------
# 9. Conditional rule violations
# ---------------------------------------------------------------------------


def test_protection_status_protected_requires_coverage_hash():
    """PROTECTED state without coverage_evidence_hash must be rejected by both."""
    value = MATRIX["ProtectionStatus"]
    # Remove coverage_evidence_hash
    modified = json.loads(json.dumps(value))
    del modified["coverage_evidence_hash"]
    assert modified["state"] == "PROTECTED"

    with pytest.raises(Exception):
        validate_with_jsonschema("ProtectionStatus", modified)
    with pytest.raises(Exception):
        validate_with_pydantic("ProtectionStatus", modified)


def test_protection_status_protected_requires_min_1_stop():
    """PROTECTED state with empty active_stop_order_ids must be rejected."""
    value = MATRIX["ProtectionStatus"]
    modified = json.loads(json.dumps(value))
    modified["active_stop_order_ids"] = []
    assert modified["state"] == "PROTECTED"

    with pytest.raises(Exception):
        validate_with_jsonschema("ProtectionStatus", modified)
    with pytest.raises(Exception):
        validate_with_pydantic("ProtectionStatus", modified)


def test_model_proposal_no_trade_with_intent():
    """NO_TRADE decision with an intent present must be rejected."""
    value = MATRIX["ModelProposal"]
    modified = json.loads(json.dumps(value))
    modified["decision"] = "NO_TRADE"
    modified["intent"] = MATRIX["TradeIntent"]
    with pytest.raises(Exception):
        validate_with_pydantic("ModelProposal", modified)


def test_model_proposal_propose_trade_without_intent():
    """PROPOSE_TRADE decision without intent must be rejected."""
    value = MATRIX["ModelProposal"]
    modified = json.loads(json.dumps(value))
    modified["decision"] = "PROPOSE_TRADE"
    # The matrix value is NO_TRADE, which has no intent field
    if "intent" in modified:
        del modified["intent"]
    with pytest.raises(Exception):
        validate_with_pydantic("ModelProposal", modified)
    with pytest.raises(Exception):
        validate_with_jsonschema("ModelProposal", modified)


def test_execution_command_place_order_without_required_fields():
    """PLACE_ORDER action without order fields must be rejected."""
    value = MATRIX["ExecutionCommand"]
    modified = json.loads(json.dumps(value))
    modified["action"] = "PLACE_ORDER"
    del modified["client_order_id"]
    del modified["symbol"]
    with pytest.raises(Exception):
        validate_with_pydantic("ExecutionCommand", modified)
    with pytest.raises(Exception):
        validate_with_jsonschema("ExecutionCommand", modified)


def test_execution_command_risk_reduction_reduce_only_not_true():
    """RISK_REDUCTION with reduce_only=false must be rejected."""
    value = MATRIX["ExecutionCommand"]
    modified = json.loads(json.dumps(value))
    modified["authorization_type"] = "RISK_REDUCTION"
    modified["reduce_only"] = False
    with pytest.raises(Exception):
        validate_with_pydantic("ExecutionCommand", modified)


def test_trade_intent_market_with_gtc_rejected_by_both():
    """MARKET + GTC must be rejected by both validators."""
    value = MATRIX["TradeIntent"]
    modified = json.loads(json.dumps(value))
    modified["order_type"] = "MARKET"
    modified["time_in_force"] = "GTC"
    with pytest.raises(Exception):
        validate_with_pydantic("TradeIntent", modified)
    with pytest.raises(Exception):
        validate_with_jsonschema("TradeIntent", modified)


# ---------------------------------------------------------------------------
# 10. Type coercion attacks (int where string expected, etc.)
# ---------------------------------------------------------------------------


@pytest.mark.parametrize("name,value", _matrix_entries())
def test_int_in_decimal_field_rejected_jsonschema(name, value):
    """Passing an int where a decimal string is expected must be rejected."""
    modified = json.loads(json.dumps(value))
    # Find decimal string fields
    decimal_like = {"quantity", "notional", "price", "fee", "entry_price",
                    "mark_price", "trigger_price", "worst_acceptable_price",
                    "maximum_loss", "liquidation_price", "estimated_fees",
                    "slippage_budget", "admitted_quantity", "maximum_notional",
                    "entry_price_or_bound", "signed_quantity", "filled_quantity",
                    "absolute_live_position_quantity"}
    mutated = False
    for field in decimal_like:
        if field in modified and isinstance(modified[field], str):
            try:
                int_val = int(modified[field].split(".")[0])
                modified[field] = int_val
                mutated = True
                break
            except (ValueError, IndexError):
                pass
    if not mutated:
        pytest.skip(f"no decimal string field to coerce in {name}")
    with pytest.raises(Exception):
        validate_with_jsonschema(name, modified)


@pytest.mark.parametrize("name,value", _matrix_entries())
def test_int_in_decimal_field_rejected_pydantic(name, value):
    """Passing an int where a decimal string is expected must be rejected."""
    modified = json.loads(json.dumps(value))
    decimal_like = {"quantity", "notional", "price", "fee", "entry_price",
                    "mark_price", "trigger_price", "worst_acceptable_price",
                    "maximum_loss", "liquidation_price", "estimated_fees",
                    "slippage_budget", "admitted_quantity", "maximum_notional",
                    "entry_price_or_bound", "signed_quantity", "filled_quantity",
                    "absolute_live_position_quantity"}
    mutated = False
    for field in decimal_like:
        if field in modified and isinstance(modified[field], str):
            try:
                int_val = int(modified[field].split(".")[0])
                modified[field] = int_val
                mutated = True
                break
            except (ValueError, IndexError):
                pass
    if not mutated:
        pytest.skip(f"no decimal string field to coerce in {name}")
    with pytest.raises(Exception):
        validate_with_pydantic(name, modified)
