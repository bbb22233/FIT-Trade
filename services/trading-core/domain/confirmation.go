package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
)

var confirmationFields = []string{
	"schema_version", "confirmation_id", "device_id", "session_id",
	"intent.schema_version", "intent.intent_id", "intent.user_id", "intent.account_id",
	"intent.network", "intent.symbol", "intent.side", "intent.position_effect",
	"intent.order_type", "intent.time_in_force", "intent.quantity", "intent.notional",
	"intent.margin_mode", "intent.leverage", "intent.entry_price_or_bound",
	"intent.worst_acceptable_price", "intent.stop.type", "intent.stop.trigger_price",
	"intent.stop.reduce_only", "intent.take_profit_plan", "intent.risk_policy_version",
	"intent.market_snapshot_id", "intent.strategy_version", "intent.model_version",
	"intent.source", "intent.created_at", "maximum_loss", "maximum_loss_fraction",
	"post_trade_total_risk", "post_trade_total_risk_fraction", "liquidation_price",
	"estimated_fees", "slippage_budget", "expires_at", "confirmation_nonce", "created_at",
}

func ConfirmationFields() []string { return append([]string(nil), confirmationFields...) }

func ConfirmationDigest(ticket ConfirmationTicket) (string, error) {
	raw, err := json.Marshal(ticket)
	if err != nil {
		return "", err
	}
	var root map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return "", err
	}
	binding := make(map[string]any, len(confirmationFields))
	for _, path := range confirmationFields {
		value, err := atPath(root, path)
		if err != nil {
			return "", err
		}
		binding[path] = value
	}
	canonical, err := canonicalJSON(binding)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func atPath(root map[string]any, path string) (any, error) {
	var current any = root
	for _, segment := range strings.Split(path, ".") {
		object, ok := current.(map[string]any)
		if !ok {
			return nil, errors.New("confirmation path traverses non-object: " + path)
		}
		current, ok = object[segment]
		if !ok {
			return nil, errors.New("confirmation field missing: " + path)
		}
	}
	return current, nil
}

// canonicalJSON implements the RFC 8785 rules needed by the validated contract:
// lexicographically sorted object keys, minimal JSON strings, arrays in order,
// booleans/null, and integral JSON numbers. Contract financials are strings.
func canonicalJSON(value any) ([]byte, error) {
	var b strings.Builder
	var write func(any) error
	write = func(v any) error {
		switch x := v.(type) {
		case nil:
			b.WriteString("null")
		case bool:
			b.WriteString(strconv.FormatBool(x))
		case string:
			encoded, err := json.Marshal(x)
			if err != nil {
				return err
			}
			b.Write(encoded)
		case json.Number:
			if strings.ContainsAny(string(x), ".eE+") {
				return errors.New("non-integral number outside contract JCS profile")
			}
			b.WriteString(string(x))
		case []any:
			b.WriteByte('[')
			for i, item := range x {
				if i > 0 {
					b.WriteByte(',')
				}
				if err := write(item); err != nil {
					return err
				}
			}
			b.WriteByte(']')
		case map[string]any:
			keys := make([]string, 0, len(x))
			for key := range x {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			b.WriteByte('{')
			for i, key := range keys {
				if i > 0 {
					b.WriteByte(',')
				}
				encoded, err := json.Marshal(key)
				if err != nil {
					return err
				}
				b.Write(encoded)
				b.WriteByte(':')
				if err := write(x[key]); err != nil {
					return err
				}
			}
			b.WriteByte('}')
		default:
			return errors.New("unsupported JCS value")
		}
		return nil
	}
	if err := write(value); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}
