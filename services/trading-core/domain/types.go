package domain

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const SchemaVersion = "fit.trade.v1"

var (
	uuidPattern            = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	hashPattern            = regexp.MustCompile(`^[a-f0-9]{64}$`)
	cloidPattern           = regexp.MustCompile(`^[a-f0-9]{32}$`)
	noncePattern           = regexp.MustCompile(`^[A-Za-z0-9_-]{32,128}$`)
	riskVersionPattern     = regexp.MustCompile(`^risk-[a-z0-9][a-z0-9._-]{0,63}$`)
	strategyVersionPattern = regexp.MustCompile(`^strategy-[a-z0-9][a-z0-9._-]{0,63}$`)
	modelVersionPattern    = regexp.MustCompile(`^model-[a-z0-9][a-z0-9._-]{0,63}$`)
)

type StopMarket struct {
	Type         string `json:"type"`
	TriggerPrice string `json:"trigger_price"`
	ReduceOnly   bool   `json:"reduce_only"`
}

type TakeProfitLeg struct {
	TriggerPrice     string `json:"trigger_price"`
	QuantityFraction string `json:"quantity_fraction"`
	ReduceOnly       bool   `json:"reduce_only"`
}

type TradeIntent struct {
	SchemaVersion        string          `json:"schema_version"`
	IntentID             string          `json:"intent_id"`
	UserID               string          `json:"user_id"`
	AccountID            string          `json:"account_id"`
	Network              string          `json:"network"`
	Symbol               string          `json:"symbol"`
	Side                 string          `json:"side"`
	PositionEffect       string          `json:"position_effect"`
	OrderType            string          `json:"order_type"`
	TimeInForce          string          `json:"time_in_force"`
	Quantity             string          `json:"quantity"`
	Notional             string          `json:"notional"`
	MarginMode           string          `json:"margin_mode"`
	Leverage             int             `json:"leverage"`
	EntryPriceOrBound    string          `json:"entry_price_or_bound"`
	WorstAcceptablePrice string          `json:"worst_acceptable_price"`
	Stop                 *StopMarket     `json:"stop,omitempty"`
	TakeProfitPlan       []TakeProfitLeg `json:"take_profit_plan"`
	RiskPolicyVersion    string          `json:"risk_policy_version"`
	MarketSnapshotID     string          `json:"market_snapshot_id"`
	StrategyVersion      string          `json:"strategy_version"`
	ModelVersion         string          `json:"model_version"`
	Source               string          `json:"source"`
	CreatedAt            string          `json:"created_at"`
}

func (v TradeIntent) Validate() error {
	if v.SchemaVersion != SchemaVersion {
		return errors.New("unsupported schema version")
	}
	for name, id := range map[string]string{"intent_id": v.IntentID, "user_id": v.UserID, "account_id": v.AccountID, "market_snapshot_id": v.MarketSnapshotID} {
		if !uuidPattern.MatchString(id) {
			return fmt.Errorf("%s is not a UUID", name)
		}
	}
	if v.Network != "HYPERLIQUID_MAINNET" && v.Network != "HYPERLIQUID_TESTNET" {
		return errors.New("unsupported network")
	}
	if v.Symbol != "BTC-PERP" && v.Symbol != "ETH-PERP" && v.Symbol != "SOL-PERP" {
		return errors.New("symbol is not allowed")
	}
	if v.Side != "BUY" && v.Side != "SELL" {
		return errors.New("invalid side")
	}
	if v.PositionEffect != "OPEN" && v.PositionEffect != "INCREASE" && v.PositionEffect != "REDUCE" && v.PositionEffect != "CLOSE" {
		return errors.New("invalid position effect")
	}
	if v.OrderType == "MARKET" && v.TimeInForce != "IOC" {
		return errors.New("market order requires IOC")
	}
	if v.OrderType == "LIMIT" && v.TimeInForce != "GTC" && v.TimeInForce != "ALO" {
		return errors.New("limit order requires GTC or ALO")
	}
	if v.OrderType != "MARKET" && v.OrderType != "LIMIT" {
		return errors.New("invalid order type")
	}
	if v.MarginMode != "CROSS" && v.MarginMode != "ISOLATED" {
		return errors.New("invalid margin mode")
	}
	if v.Leverage < 1 || v.Leverage > 100 {
		return errors.New("invalid leverage")
	}
	for name, value := range map[string]string{"quantity": v.Quantity, "notional": v.Notional, "entry_price_or_bound": v.EntryPriceOrBound, "worst_acceptable_price": v.WorstAcceptablePrice} {
		d, err := ParseDecimal(value)
		if err != nil || d.Sign() <= 0 {
			return fmt.Errorf("%s must be a positive canonical decimal", name)
		}
	}
	if v.PositionEffect == "OPEN" || v.PositionEffect == "INCREASE" {
		if v.Stop == nil {
			return errors.New("risk increase requires reduce-only Stop Market")
		}
	}
	if v.Stop != nil {
		if v.Stop.Type != "STOP_MARKET" || !v.Stop.ReduceOnly {
			return errors.New("stop must be a reduce-only Stop Market")
		}
		d, err := ParseDecimal(v.Stop.TriggerPrice)
		if err != nil || d.Sign() <= 0 {
			return errors.New("invalid stop trigger")
		}
	}
	if v.TakeProfitPlan == nil || len(v.TakeProfitPlan) > 8 {
		return errors.New("invalid take profit plan")
	}
	for _, leg := range v.TakeProfitPlan {
		trigger, triggerErr := ParseDecimal(leg.TriggerPrice)
		fraction, fractionErr := ParseDecimal(leg.QuantityFraction)
		if triggerErr != nil || trigger.Sign() <= 0 || fractionErr != nil ||
			fraction.Sign() <= 0 || decimalGreaterThanOne(fraction) || !leg.ReduceOnly {
			return errors.New("invalid take profit leg")
		}
	}
	if !riskVersionPattern.MatchString(v.RiskPolicyVersion) ||
		!strategyVersionPattern.MatchString(v.StrategyVersion) ||
		!modelVersionPattern.MatchString(v.ModelVersion) {
		return errors.New("invalid version field")
	}
	if v.Source != "USER_DIRECTED" && v.Source != "AUTOMATION" {
		return errors.New("invalid source")
	}
	if _, err := parseUTCTimestamp(v.CreatedAt); err != nil {
		return errors.New("invalid created_at")
	}
	return nil
}

type ConfirmationTicket struct {
	SchemaVersion              string      `json:"schema_version"`
	ConfirmationID             string      `json:"confirmation_id"`
	DeviceID                   string      `json:"device_id"`
	SessionID                  string      `json:"session_id"`
	Intent                     TradeIntent `json:"intent"`
	MaximumLoss                string      `json:"maximum_loss"`
	MaximumLossFraction        string      `json:"maximum_loss_fraction"`
	PostTradeTotalRisk         string      `json:"post_trade_total_risk"`
	PostTradeTotalRiskFraction string      `json:"post_trade_total_risk_fraction"`
	LiquidationPrice           string      `json:"liquidation_price"`
	EstimatedFees              string      `json:"estimated_fees"`
	SlippageBudget             string      `json:"slippage_budget"`
	ExpiresAt                  string      `json:"expires_at"`
	ConfirmationNonce          string      `json:"confirmation_nonce"`
	ConfirmationHash           string      `json:"confirmation_hash"`
	CreatedAt                  string      `json:"created_at"`
}

func (v ConfirmationTicket) Validate(now time.Time) error {
	if v.SchemaVersion != SchemaVersion || !uuidPattern.MatchString(v.ConfirmationID) || !uuidPattern.MatchString(v.DeviceID) || !uuidPattern.MatchString(v.SessionID) {
		return errors.New("invalid confirmation identity")
	}
	if err := v.Intent.Validate(); err != nil {
		return err
	}
	if v.Intent.PositionEffect != "OPEN" && v.Intent.PositionEffect != "INCREASE" {
		return errors.New("confirmation ticket must increase risk")
	}
	for name, value := range map[string]string{"maximum_loss": v.MaximumLoss, "post_trade_total_risk": v.PostTradeTotalRisk, "estimated_fees": v.EstimatedFees, "slippage_budget": v.SlippageBudget} {
		d, err := ParseDecimal(value)
		if err != nil || d.Sign() < 0 {
			return fmt.Errorf("%s must be non-negative", name)
		}
	}
	for name, value := range map[string]string{"maximum_loss_fraction": v.MaximumLossFraction, "post_trade_total_risk_fraction": v.PostTradeTotalRiskFraction} {
		d, err := ParseDecimal(value)
		if err != nil || d.Sign() < 0 || decimalGreaterThanOne(d) {
			return fmt.Errorf("%s must be a fraction", name)
		}
	}
	d, err := ParseDecimal(v.LiquidationPrice)
	if err != nil || d.Sign() <= 0 {
		return errors.New("invalid liquidation price")
	}
	expires, err := parseUTCTimestamp(v.ExpiresAt)
	if err != nil || !expires.After(now) {
		return errors.New("confirmation expired")
	}
	if !noncePattern.MatchString(v.ConfirmationNonce) || !hashPattern.MatchString(v.ConfirmationHash) {
		return errors.New("invalid confirmation proof")
	}
	if _, err := parseUTCTimestamp(v.CreatedAt); err != nil {
		return errors.New("invalid created_at")
	}
	digest, err := ConfirmationDigest(v)
	if err != nil || digest != v.ConfirmationHash {
		return errors.New("confirmation digest mismatch")
	}
	return nil
}

func decimalGreaterThanOne(d Decimal) bool {
	one, _ := ParseDecimal("1")
	var left, right big.Int
	left.Set(&d.coefficient)
	right.Set(&one.coefficient)
	if d.scale < one.scale {
		left.Mul(&left, pow10(one.scale-d.scale))
	} else {
		right.Mul(&right, pow10(d.scale-one.scale))
	}
	return left.Cmp(&right) > 0
}

func parseUTCTimestamp(value string) (time.Time, error) {
	if len(value) < 2 || (value[len(value)-1] != 'Z' && value[len(value)-1] != 'z') {
		return time.Time{}, errors.New("timestamp must use a UTC Z designator")
	}
	normalized := []byte(value)
	if len(normalized) > len("2006-01-02") && (normalized[10] == 't' || normalized[10] == ' ') {
		normalized[10] = 'T'
	}
	normalized[len(normalized)-1] = 'Z'
	leapSecond := len(normalized) >= len("2006-01-02T15:04:05Z") &&
		normalized[11] == '2' && normalized[12] == '3' &&
		normalized[14] == '5' && normalized[15] == '9' &&
		normalized[17] == '6' && normalized[18] == '0'
	if leapSecond {
		normalized[17], normalized[18] = '5', '9'
	}
	parsed, err := time.Parse(time.RFC3339, string(normalized))
	if err != nil {
		return time.Time{}, err
	}
	if leapSecond {
		parsed = parsed.Add(time.Second)
	}
	return parsed, nil
}

func StrictDecode(data []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("trailing JSON value")
	}
	return nil
}

type PositionSnapshot struct {
	PositionID         string
	SignedQuantity     string
	ProtectionStatusID string
	ProtectionState    ProtectionState
}

type ProtectionStatus struct {
	ProtectionStatusID         string
	PositionID                 string
	State                      ProtectionState
	AbsoluteLivePositionAmount string
	ActiveStopOrderIDs         []string
	CoverageEvidenceHash       string
}

func ValidateProtectionCoverage(position PositionSnapshot, protection ProtectionStatus) error {
	if !uuidPattern.MatchString(position.PositionID) ||
		!uuidPattern.MatchString(position.ProtectionStatusID) ||
		!uuidPattern.MatchString(protection.PositionID) ||
		!uuidPattern.MatchString(protection.ProtectionStatusID) {
		return errors.New("invalid protection identity")
	}
	if position.ProtectionState != ProtectionProtected || protection.State != ProtectionProtected ||
		position.ProtectionStatusID != protection.ProtectionStatusID || position.PositionID != protection.PositionID {
		return errors.New("protection identity or state mismatch")
	}
	signed, err := ParseDecimal(position.SignedQuantity)
	if err != nil {
		return err
	}
	covered, err := ParseDecimal(protection.AbsoluteLivePositionAmount)
	if err != nil || covered.Sign() <= 0 || !signed.EqualAbs(covered) {
		return errors.New("stop coverage does not equal live position")
	}
	if len(protection.ActiveStopOrderIDs) < 1 || len(protection.ActiveStopOrderIDs) > 32 ||
		!hashPattern.MatchString(protection.CoverageEvidenceHash) {
		return errors.New("protection evidence missing")
	}
	seen := make(map[string]struct{}, len(protection.ActiveStopOrderIDs))
	for _, id := range protection.ActiveStopOrderIDs {
		if utf8.RuneCountInString(id) < 1 || utf8.RuneCountInString(id) > 128 ||
			strings.TrimSpace(id) == "" {
			return errors.New("invalid stop order identifier")
		}
		if _, duplicate := seen[id]; duplicate {
			return errors.New("duplicate stop order identifier")
		}
		seen[id] = struct{}{}
	}
	return nil
}
