package domain

// FrozenScenarioResult captures the fail-closed domain result required by each
// synthetic adapter scenario. Unknown scenario identifiers are rejected.
func FrozenScenarioResult(id string) (string, bool) {
	result, ok := frozenScenarioResults[id]
	return result, ok
}

var frozenScenarioResults = map[string]string{
	"read_snapshot_ok":                               "SUCCESS",
	"dispatch_acknowledged":                          "ACKNOWLEDGED",
	"dispatch_timeout_before_send":                   "NOT_DISPATCHED",
	"dispatch_timeout_after_send":                    "UNKNOWN_REQUIRES_RECONCILIATION",
	"executor_crash_after_exchange_success":          "UNKNOWN_REQUIRES_RECONCILIATION",
	"executor_crash_before_signing":                  "NOT_DISPATCHED",
	"executor_crash_after_signing_before_submit":     "NOT_DISPATCHED",
	"executor_crash_after_submit_before_record":      "UNKNOWN_REQUIRES_RECONCILIATION",
	"executor_crash_after_record_before_report":      "EXISTING_RESULT",
	"result_published_ack_lost":                      "EXISTING_RESULT",
	"duplicate_client_order_id":                      "EXISTING_RESULT",
	"duplicate_event":                                "EXISTING_RESULT",
	"partial_fill_then_protection_ok":                "PROTECTED",
	"partial_fill_then_parent_cancel":                "PROTECTION_PENDING",
	"rapid_consecutive_partial_fills":                "PROTECTION_PENDING",
	"protection_response_lost":                       "MANUAL_RECONCILIATION",
	"replacement_stop_create_failed_old_stop_active": "PROTECTED",
	"partial_fill_protection_failed":                 "EMERGENCY_CLOSING",
	"emergency_close_residual_position":              "MANUAL_RECONCILIATION",
	"manual_native_position_change":                  "RECONCILING",
	"database_restart_during_operation":              "RECOVER_FROM_DURABLE_STATE",
	"websocket_disconnect_and_resubscribe":           "RECONCILING",
	"client_disconnect_then_duplicate_submit":        "EXISTING_RESULT",
	"stale_market_snapshot":                          "DATA_STALE",
}
