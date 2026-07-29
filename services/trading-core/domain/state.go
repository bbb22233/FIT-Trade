package domain

import "fmt"

type OperationState string

const (
	OperationDraft           OperationState = "DRAFT"
	OperationAwaiting        OperationState = "AWAITING_CONFIRMATION"
	OperationExpired         OperationState = "EXPIRED"
	OperationConfirmed       OperationState = "CONFIRMED"
	OperationRiskValidating  OperationState = "RISK_REVALIDATING"
	OperationRejected        OperationState = "REJECTED"
	OperationAdmitted        OperationState = "ADMITTED"
	OperationDispatchPending OperationState = "DISPATCH_PENDING"
	OperationDispatched      OperationState = "DISPATCHED"
	OperationAcknowledged    OperationState = "ACKNOWLEDGED"
	OperationUnknown         OperationState = "UNKNOWN_REQUIRES_RECONCILIATION"
	OperationManual          OperationState = "MANUAL_RECONCILIATION"
	OperationFinal           OperationState = "FINAL"
)

var operationEdges = edgeSet[OperationState](
	[2]OperationState{OperationDraft, OperationAwaiting},
	[2]OperationState{OperationAwaiting, OperationExpired},
	[2]OperationState{OperationAwaiting, OperationConfirmed},
	[2]OperationState{OperationConfirmed, OperationRiskValidating},
	[2]OperationState{OperationRiskValidating, OperationRejected},
	[2]OperationState{OperationRiskValidating, OperationAdmitted},
	[2]OperationState{OperationAdmitted, OperationDispatchPending},
	[2]OperationState{OperationDispatchPending, OperationDispatched},
	[2]OperationState{OperationDispatched, OperationAcknowledged},
	[2]OperationState{OperationDispatched, OperationUnknown},
	[2]OperationState{OperationUnknown, OperationAcknowledged},
	[2]OperationState{OperationUnknown, OperationRejected},
	[2]OperationState{OperationUnknown, OperationManual},
	[2]OperationState{OperationAcknowledged, OperationFinal},
	[2]OperationState{OperationRejected, OperationFinal},
	[2]OperationState{OperationExpired, OperationFinal},
)

type ProtectionState string

const (
	ProtectionNoPosition       ProtectionState = "NO_POSITION"
	ProtectionEntryPending     ProtectionState = "ENTRY_PENDING"
	ProtectionPartiallyFilled  ProtectionState = "PARTIALLY_FILLED"
	ProtectionFilled           ProtectionState = "FILLED"
	ProtectionPending          ProtectionState = "PROTECTION_PENDING"
	ProtectionProtected        ProtectionState = "PROTECTED"
	ProtectionAdjusting        ProtectionState = "ADJUSTING_PROTECTION"
	ProtectionFailed           ProtectionState = "PROTECTION_FAILED"
	ProtectionEmergencyClosing ProtectionState = "EMERGENCY_CLOSING"
	ProtectionManual           ProtectionState = "MANUAL_RECONCILIATION"
	ProtectionClosed           ProtectionState = "CLOSED"
)

var protectionEdges = edgeSet[ProtectionState](
	[2]ProtectionState{ProtectionNoPosition, ProtectionEntryPending},
	[2]ProtectionState{ProtectionEntryPending, ProtectionPartiallyFilled},
	[2]ProtectionState{ProtectionEntryPending, ProtectionFilled},
	[2]ProtectionState{ProtectionPartiallyFilled, ProtectionPending},
	[2]ProtectionState{ProtectionFilled, ProtectionPending},
	[2]ProtectionState{ProtectionPending, ProtectionProtected},
	[2]ProtectionState{ProtectionPending, ProtectionFailed},
	[2]ProtectionState{ProtectionPending, ProtectionPartiallyFilled},
	[2]ProtectionState{ProtectionProtected, ProtectionAdjusting},
	[2]ProtectionState{ProtectionProtected, ProtectionPartiallyFilled},
	[2]ProtectionState{ProtectionAdjusting, ProtectionProtected},
	[2]ProtectionState{ProtectionAdjusting, ProtectionFailed},
	[2]ProtectionState{ProtectionAdjusting, ProtectionPartiallyFilled},
	[2]ProtectionState{ProtectionFailed, ProtectionEmergencyClosing},
	[2]ProtectionState{ProtectionEmergencyClosing, ProtectionClosed},
	[2]ProtectionState{ProtectionEmergencyClosing, ProtectionManual},
	[2]ProtectionState{ProtectionProtected, ProtectionClosed},
	[2]ProtectionState{ProtectionClosed, ProtectionNoPosition},
)

func edgeSet[T comparable](edges ...[2]T) map[[2]T]struct{} {
	result := make(map[[2]T]struct{}, len(edges))
	for _, edge := range edges {
		result[edge] = struct{}{}
	}
	return result
}

func ValidateOperationTransition(from, to OperationState) error {
	if _, ok := operationEdges[[2]OperationState{from, to}]; !ok {
		return fmt.Errorf("illegal Operation transition %s -> %s", from, to)
	}
	return nil
}

func ValidateProtectionTransition(from, to ProtectionState) error {
	if _, ok := protectionEdges[[2]ProtectionState{from, to}]; !ok {
		return fmt.Errorf("illegal protection transition %s -> %s", from, to)
	}
	return nil
}
