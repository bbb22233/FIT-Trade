"""Test all invalid fixtures are rejected with fail-closed semantics."""
import pytest
from hermes_agent.contracts import DOMAIN_TYPES
from hermes_agent.validator import load_invalid_fixtures


@pytest.mark.parametrize(
    "schema_name,value,reason,path",
    load_invalid_fixtures(),
    ids=lambda x: f"{x[3].name}: {x[2]}" if isinstance(x, tuple) else str(x),
)
def test_invalid_fixture_rejected(schema_name, value, reason, path):
    """Every invalid fixture must be rejected by Pydantic validation."""
    model = DOMAIN_TYPES.get(schema_name)
    assert model is not None, f"Unknown schema: {schema_name}"
    with pytest.raises(Exception):
        model.model_validate(value)
