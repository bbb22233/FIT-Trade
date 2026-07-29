"""RFC 8785 confirmation hash binding for FIT-Trade v1.

Implements deterministic confirmation binding using:
  - RFC 8785 JSON Canonicalization Scheme (JCS) via rfc8785==0.1.4 (Trail of Bits)
  - SHA-256 for hashing
  - Exact parity with contracts/scripts/verify.mjs

The golden vector at contracts/fixtures/golden/confirmation-open.sha256
must match for cross-language agreement.
"""

from __future__ import annotations

import hashlib
from typing import Any

from rfc8785 import CanonicalizationError, dumps as rfc8785_dumps


def canonicalize(value: Any) -> str:
    """RFC 8785 JSON Canonicalization Scheme (JCS) serializer.

    Delegates to Trail of Bits' rfc8785==0.1.4 for a correct,
    independently tested, pinned implementation.

    Returns JCS canonical bytes decoded to UTF-8 str.
    Raises CanonicalizationError (including FloatDomainError /
    IntegerDomainError) on non-finite numbers or unsupported types.
    """
    return rfc8785_dumps(value).decode("utf-8")


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
