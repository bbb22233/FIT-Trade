"""RFC 8785 confirmation hash binding for FIT-Trade v1.

Implements deterministic confirmation binding using:
  - RFC 8785 JSON Canonicalization Scheme (JCS) for serialization
  - SHA-256 for hashing
  - Exact parity with contracts/scripts/verify.mjs

The golden vector at contracts/fixtures/golden/confirmation-open.sha256
must match for cross-language agreement.
"""

from __future__ import annotations

import hashlib
import json
from typing import Any


def canonicalize(value: Any) -> str:
    """RFC 8785 JSON Canonicalization Scheme (JCS) serializer.

    Rules:
      - null, boolean, number: JSON literals (no non-finite numbers)
      - string: JSON quoted string
      - array: [element,…] in order
      - object: {key:value,…} sorted by JSON key string (UTF-16 code unit order)

    Per JCS, no whitespace is emitted.
    """
    if value is None:
        return "null"
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, int):
        # JCS: integers stay as integers (no decimal point)
        return str(value)
    if isinstance(value, float):
        # JCS forbids non-finite
        import math
        if not math.isfinite(value):
            raise ValueError(f"JCS forbids non-finite number: {value}")
        return json.dumps(value)
    if isinstance(value, str):
        return json.dumps(value, ensure_ascii=False)
    if isinstance(value, list):
        items = ",".join(canonicalize(v) for v in value)
        return f"[{items}]"
    if isinstance(value, dict):
        keys = sorted(value.keys())
        members = ",".join(
            f"{json.dumps(k, ensure_ascii=False)}:{canonicalize(v)}"
            for k, v in ((k, value[k]) for k in keys)
        )
        return f"{{{members}}}"
    raise TypeError(f"unsupported JCS value type: {type(value)}")


def confirmation_binding(ticket: dict[str, Any], fields: list[str]) -> dict[str, Any]:
    """Extract a flat object of bound fields from a ConfirmationTicket.

    Each dotted path is resolved; missing paths raise KeyError.
    confirmation_hash is intentionally excluded.
    """
    def value_at_path(obj: Any, dotted_path: str) -> Any:
        segments = dotted_path.split(".")
        current = obj
        for segment in segments:
            if not isinstance(current, dict) or segment not in current:
                raise KeyError(f"confirmation field missing: {dotted_path}")
            current = current[segment]
        return current

    return {field: value_at_path(ticket, field) for field in fields}


def compute_confirmation_hash(
    ticket: dict[str, Any],
    fields: list[str],
) -> str:
    """Compute SHA-256 confirmation hash for a ConfirmationTicket.

    Steps:
      1. Extract bound fields (excluding confirmation_hash)
      2. RFC 8785 canonicalize
      3. SHA-256 → 64 lowercase hex chars
    """
    binding = confirmation_binding(ticket, fields)
    canonical_bytes = canonicalize(binding).encode("utf-8")
    return hashlib.sha256(canonical_bytes).hexdigest()


def verify_confirmation_hash(
    ticket: dict[str, Any],
    fields: list[str],
    expected: str,
) -> bool:
    """Verify that a ticket's confirmation_hash matches the computed digest."""
    computed = compute_confirmation_hash(ticket, fields)
    return computed == expected
