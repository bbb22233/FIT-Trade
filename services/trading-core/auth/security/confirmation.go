package security

import (
	"context"
	"errors"
)

type ConfirmationPurpose uint8

const (
	OpenPosition ConfirmationPurpose = iota + 1
	IncreasePosition
)

type OperationState uint8

const (
	OperationAwaitingConfirmation OperationState = iota + 1
	OperationConfirmed
	OperationExpired
)

// ConfirmationFact is the server-loaded immutable ticket projection needed by
// this core. Its constructor rejects partial authority tuples.
type ConfirmationFact struct {
	id               ConfirmationID
	authority        AuthorityScope
	purpose          ConfirmationPurpose
	intentDigest     Digest32
	confirmationHash Digest32
	expiresAtMS      int64
	state            ConfirmationState
	consumedClaim    ClaimID
	hasConsumedClaim bool
}

func NewConfirmationFact(id ConfirmationID, authority AuthorityScope, purpose ConfirmationPurpose, intentDigest, confirmationHash Digest32, expiresAtMS int64, state ConfirmationState) (ConfirmationFact, error) {
	if !id.value.valid() || authority.Kind() != AuthorityOwner || !authority.SessionID().value.valid() || !authority.DeviceID().value.valid() || (purpose != OpenPosition && purpose != IncreasePosition) || expiresAtMS < 0 || state < ConfirmationIssued || state > ConfirmationExpired {
		return ConfirmationFact{}, ErrInvalidDTO
	}
	return ConfirmationFact{id: id, authority: authority, purpose: purpose, intentDigest: intentDigest, confirmationHash: confirmationHash, expiresAtMS: expiresAtMS, state: state}, nil
}
func (v ConfirmationFact) ID() ConfirmationID       { return v.id }
func (v ConfirmationFact) State() ConfirmationState { return v.state }

type ConfirmationOperation struct {
	id             OperationID
	confirmationID ConfirmationID
	authority      AuthorityScope
	intentDigest   Digest32
	state          OperationState
	version        uint64
}

func NewConfirmationOperation(id OperationID, confirmationID ConfirmationID, authority AuthorityScope, intentDigest Digest32, state OperationState, version uint64) (ConfirmationOperation, error) {
	if !id.value.valid() || !confirmationID.value.valid() || authority.Kind() != AuthorityOwner || !authority.SessionID().value.valid() || !authority.DeviceID().value.valid() || state < OperationAwaitingConfirmation || state > OperationExpired {
		return ConfirmationOperation{}, ErrInvalidDTO
	}
	return ConfirmationOperation{id: id, confirmationID: confirmationID, authority: authority, intentDigest: intentDigest, state: state, version: version}, nil
}
func (v ConfirmationOperation) ID() OperationID       { return v.id }
func (v ConfirmationOperation) State() OperationState { return v.state }

// ConfirmationTransaction makes the two CAS operations and immutable link
// explicit. There is intentionally no Attempt, Order, signer, or dispatch
// method on this interface.
type ConfirmationTransaction interface {
	Transaction
	ValidateActiveSessionDevice(context.Context, AuthorityScope) error
	LoadConfirmation(context.Context, ConfirmationID) (ConfirmationFact, bool, error)
	LoadConfirmationOperation(context.Context, ConfirmationID) (ConfirmationOperation, bool, error)
	CASConfirmation(context.Context, ConfirmationID, ConfirmationState, ConfirmationState, ClaimID, ClockSnapshot) error
	CASOperation(context.Context, OperationID, OperationState, uint64, OperationState) error
	LinkConfirmationConsumption(context.Context, ConfirmationFact, ConfirmationOperation, ResultClaim, ClockSnapshot) error
	RecordConfirmationAudit(context.Context, ConfirmationOperation, ClockSnapshot) error
	ReconcileConfirmationOperation(context.Context, ConfirmationID, AuthorityScope) (ConfirmationOperation, bool, error)
}

type ConfirmationConsumeRequest struct {
	request        Request
	confirmationID ConfirmationID
	hash           Digest32
}

func NewConfirmationConsumeRequest(request Request, confirmationID ConfirmationID, hash Digest32) (ConfirmationConsumeRequest, error) {
	if request.scope.route.String() != "/v1/confirmations/{confirmation_id}/consume" || request.scope.keyKind != StableKeyIdempotency || request.scope.authority.Kind() != AuthorityOwner || !request.scope.authority.SessionID().value.valid() || !request.scope.authority.DeviceID().value.valid() || !confirmationID.value.valid() {
		return ConfirmationConsumeRequest{}, ErrInvalidDTO
	}
	return ConfirmationConsumeRequest{request: request, confirmationID: confirmationID, hash: hash}, nil
}

type ConfirmationResult struct {
	decision     ConfirmationConsumeDecision
	operation    OperationID
	hasOperation bool
}

func (v ConfirmationResult) Decision() ConfirmationConsumeDecision { return v.decision }
func (v ConfirmationResult) OperationID() (OperationID, bool)      { return v.operation, v.hasOperation }

type confirmationResponseBinding struct {
	confirmationID   ConfirmationID
	authority        AuthorityScope
	purpose          ConfirmationPurpose
	intentDigest     Digest32
	confirmationHash Digest32
	expiresAtMS      int64
}

func bindConfirmationResponse(request Request, confirmation ConfirmationFact) Request {
	request.confirmation = confirmationResponseBinding{
		confirmationID:   confirmation.id,
		authority:        confirmation.authority,
		purpose:          confirmation.purpose,
		intentDigest:     confirmation.intentDigest,
		confirmationHash: confirmation.confirmationHash,
		expiresAtMS:      confirmation.expiresAtMS,
	}
	request.hasConfirmation = true
	return request
}

func bindUnknownConfirmationRejection(request Request, confirmationID ConfirmationID) Request {
	request.confirmation = confirmationResponseBinding{
		confirmationID: confirmationID,
		authority:      request.scope.authority,
	}
	request.hasConfirmation = true
	return request
}

func serverFailClosedResponse() (ExactResponse, error) {
	body, err := NewCanonicalJSON([]byte(`{"code":"VALIDATION_FAILED","retry":"NEVER_SAME_INPUT"}`))
	if err != nil {
		return ExactResponse{}, err
	}
	headers, err := NewResponseHeaders(nil, "")
	if err != nil {
		return ExactResponse{}, err
	}
	return NewExactResponse(400, body, headers, NonCredentialRejection)
}

func failClosedPreparation(request Request) (preparedClaim, error) {
	response, err := serverFailClosedResponse()
	if err != nil {
		return preparedClaim{}, err
	}
	request.response = response
	return preparedClaim{request: request, effect: func(context.Context, Transaction, ResultClaim, ClockSnapshot) error { return nil }}, nil
}

// ConsumeConfirmation uses full authority equality before either CAS. Cache
// misses and pre-existing consumed tickets use only reconciliation; they never
// return to the effect path.
func ConsumeConfirmation(ctx context.Context, store TransactionalStore, clock Clock, keyring AEADKeyring, nonces NonceSource, input ConfirmationConsumeRequest) (ConfirmationResult, error) {
	var outcome ConfirmationResult
	prepared, err := executePrepared(ctx, store, clock, keyring, nonces, input.request, func(context.Context, Transaction, ResultClaim, ClockSnapshot) error {
		return nil
	}, func(ctx context.Context, tx Transaction, snapshot ClockSnapshot, existing bool) (preparedClaim, error) {
		confirmationTx, ok := tx.(ConfirmationTransaction)
		if !ok {
			return preparedClaim{}, ErrDependencyUnavailable
		}
		if err := confirmationTx.ValidateActiveSessionDevice(ctx, input.request.scope.authority); err != nil {
			outcome = ConfirmationResult{decision: FailClosedGenericRejection}
			if existing {
				return preparedClaim{skip: true}, nil
			}
			return failClosedPreparation(bindUnknownConfirmationRejection(input.request, input.confirmationID))
		}
		confirmation, found, err := confirmationTx.LoadConfirmation(ctx, input.confirmationID)
		if err != nil {
			return preparedClaim{}, err
		}
		if !found {
			outcome = ConfirmationResult{decision: FailClosedGenericRejection}
			if existing {
				return preparedClaim{skip: true}, nil
			}
			return failClosedPreparation(bindUnknownConfirmationRejection(input.request, input.confirmationID))
		}
		request := bindConfirmationResponse(input.request, confirmation)
		if !sameFullAuthority(confirmation.authority, input.request.scope.authority) || !confirmation.confirmationHash.equal(input.hash) {
			outcome = ConfirmationResult{decision: FailClosedGenericRejection}
			return failClosedPreparation(request)
		}
		if existing {
			return preparedClaim{request: request, effect: func(context.Context, Transaction, ResultClaim, ClockSnapshot) error { return nil }}, nil
		}
		if confirmation.state != ConfirmationIssued {
			outcome = ConfirmationResult{decision: DurableOperationReconciliation}
			return preparedClaim{skip: true}, nil
		}
		operation, found, err := confirmationTx.LoadConfirmationOperation(ctx, input.confirmationID)
		if err != nil {
			return preparedClaim{}, err
		}
		if !found || operation.confirmationID != confirmation.id || !sameFullAuthority(operation.authority, confirmation.authority) || !operation.intentDigest.equal(confirmation.intentDigest) || operation.state != OperationAwaitingConfirmation {
			outcome = ConfirmationResult{decision: FailClosedGenericRejection}
			return failClosedPreparation(request)
		}
		if snapshot.utcNowMS >= confirmation.expiresAtMS {
			prepared, err := failClosedPreparation(request)
			if err != nil {
				return preparedClaim{}, err
			}
			prepared.effect = func(ctx context.Context, _ Transaction, claim ResultClaim, snapshot ClockSnapshot) error {
				if err := confirmationTx.CASConfirmation(ctx, confirmation.id, ConfirmationIssued, ConfirmationExpired, claim.id, snapshot); err != nil {
					return err
				}
				if err := confirmationTx.CASOperation(ctx, operation.id, OperationAwaitingConfirmation, operation.version, OperationExpired); err != nil {
					return err
				}
				if err := confirmationTx.RecordConfirmationAudit(ctx, operation, snapshot); err != nil {
					return err
				}
				outcome = ConfirmationResult{decision: ExpireAndReject, operation: operation.id, hasOperation: true}
				return nil
			}
			return prepared, nil
		}
		return preparedClaim{request: request, effect: func(ctx context.Context, _ Transaction, claim ResultClaim, snapshot ClockSnapshot) error {
			if err := confirmationTx.CASConfirmation(ctx, confirmation.id, ConfirmationIssued, ConfirmationConsumed, claim.id, snapshot); err != nil {
				return err
			}
			if err := confirmationTx.CASOperation(ctx, operation.id, OperationAwaitingConfirmation, operation.version, OperationConfirmed); err != nil {
				return err
			}
			if err := confirmationTx.LinkConfirmationConsumption(ctx, confirmation, operation, claim, snapshot); err != nil {
				return err
			}
			if err := confirmationTx.RecordConfirmationAudit(ctx, operation, snapshot); err != nil {
				return err
			}
			outcome = ConfirmationResult{decision: ConsumeAndConfirm, operation: operation.id, hasOperation: true}
			return nil
		}}, nil
	})
	if err != nil {
		return ConfirmationResult{}, err
	}
	if prepared.skipped {
		return reconcileConfirmation(ctx, store, input)
	}
	switch prepared.result.decision {
	case IdempotencyConflict:
		return ConfirmationResult{decision: ConfirmationIdempotencyConflict}, nil
	case ReplayExact:
		return ConfirmationResult{decision: ConfirmationReplayExact}, nil
	case ReconciliationRequiredNoReexecution:
		return reconcileConfirmation(ctx, store, input)
	case ClaimNew:
		if outcome.decision == 0 {
			return ConfirmationResult{}, errors.New("auth security: confirmation effect returned no decision")
		}
		return outcome, nil
	default:
		return ConfirmationResult{}, ErrInvalidDTO
	}
}

func reconcileConfirmation(ctx context.Context, store TransactionalStore, input ConfirmationConsumeRequest) (ConfirmationResult, error) {
	if store == nil {
		return ConfirmationResult{}, ErrDependencyUnavailable
	}
	var result ConfirmationResult
	err := store.WithinTransaction(ctx, func(tx Transaction) error {
		confirmationTx, ok := tx.(ConfirmationTransaction)
		if !ok {
			return ErrDependencyUnavailable
		}
		if err := confirmationTx.ValidateAuthority(ctx, input.request.scope.authority); err != nil {
			return err
		}
		if err := confirmationTx.ValidateActiveSessionDevice(ctx, input.request.scope.authority); err != nil {
			result = ConfirmationResult{decision: FailClosedGenericRejection}
			return nil
		}
		op, found, err := confirmationTx.ReconcileConfirmationOperation(ctx, input.confirmationID, input.request.scope.authority)
		if err != nil {
			return err
		}
		if !found || !sameFullAuthority(op.authority, input.request.scope.authority) {
			result = ConfirmationResult{decision: FailClosedGenericRejection}
			return nil
		}
		result = ConfirmationResult{decision: DurableOperationReconciliation, operation: op.id, hasOperation: true}
		return nil
	})
	if err != nil {
		return ConfirmationResult{}, err
	}
	return result, nil
}

func sameFullAuthority(a, b AuthorityScope) bool {
	return a.Kind() == AuthorityOwner && b.Kind() == AuthorityOwner && a.UserID() == b.UserID() && a.AccountID() == b.AccountID() && a.OwnershipID() == b.OwnershipID() && a.SessionID() == b.SessionID() && a.DeviceID() == b.DeviceID()
}
