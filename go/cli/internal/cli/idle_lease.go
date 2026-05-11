package cli

import (
	"encoding/json"
	"fmt"
	"time"
)

type idleLeaseState string

const (
	idleLeaseStateActive idleLeaseState = "active"
	idleLeaseStateIdle   idleLeaseState = "idle"
	idleLeaseStateStale  idleLeaseState = "stale"
)

type idleLease struct {
	Kind             string            `json:"kind"`
	ID               string            `json:"id"`
	RootPID          int               `json:"rootPid"`
	ProcessGroup     int               `json:"processGroup"`
	User             string            `json:"user"`
	WorkingDirectory string            `json:"workingDirectory"`
	Command          string            `json:"command"`
	Interactive      bool              `json:"interactive"`
	StartedAt        time.Time         `json:"startedAt"`
	UpdatedAt        time.Time         `json:"updatedAt"`
	LastInputAt      *time.Time        `json:"lastInputAt,omitempty"`
	LastOutputAt     *time.Time        `json:"lastOutputAt,omitempty"`
	IdleAfter        idleLeaseDuration `json:"idleAfter"`
	ExpiresAt        time.Time         `json:"expiresAt"`
}

type idleLeaseDuration time.Duration

func (d *idleLeaseDuration) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("idleAfter must be a duration string: %w", err)
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return fmt.Errorf("parse idleAfter: %w", err)
	}
	*d = idleLeaseDuration(duration)
	return nil
}

func (d idleLeaseDuration) MarshalJSON() ([]byte, error) {
	if d == 0 {
		return []byte("null"), nil
	}
	return json.Marshal(time.Duration(d).String())
}

type idleLeaseTerminalActivityKind int

const (
	idleLeaseTerminalActivityInput idleLeaseTerminalActivityKind = iota
	idleLeaseTerminalActivityOutput
)

type idleLeaseTerminalActivityUpdate struct {
	Lease    idleLease
	FlushNow bool
}

func (lease idleLease) recordHeartbeat(now time.Time) idleLease {
	now = now.UTC()
	lease.UpdatedAt = now
	lease.ExpiresAt = lease.expiresAt(now)
	return lease
}

func (lease idleLease) recordTerminalActivity(now time.Time, kind idleLeaseTerminalActivityKind) idleLeaseTerminalActivityUpdate {
	now = now.UTC()
	previousActivity, hadPreviousActivity := lease.latestTerminalActivity()

	activityAt := now
	switch kind {
	case idleLeaseTerminalActivityInput:
		lease.LastInputAt = &activityAt
	case idleLeaseTerminalActivityOutput:
		lease.LastOutputAt = &activityAt
	default:
		return idleLeaseTerminalActivityUpdate{Lease: lease}
	}

	idleAfter := time.Duration(lease.IdleAfter)
	flushNow := !hadPreviousActivity || previousActivity.Before(now.Add(-idleAfter))
	return idleLeaseTerminalActivityUpdate{
		Lease:    lease,
		FlushNow: flushNow,
	}
}

type idleLeaseEvaluation struct {
	State  idleLeaseState
	Reason idleLeaseEvaluationReason
}

type idleLeaseEvaluationReason int

const (
	idleLeaseEvaluationReasonInvalidIdleAfter idleLeaseEvaluationReason = iota + 1
	idleLeaseEvaluationReasonMissingExpiresAt
	idleLeaseEvaluationReasonMissingHeartbeat
	idleLeaseEvaluationReasonLeaseExpired
	idleLeaseEvaluationReasonHeartbeatTooOld
	idleLeaseEvaluationReasonRootProcessNotRunning
	idleLeaseEvaluationReasonNoTerminalActivity
	idleLeaseEvaluationReasonTerminalActivityOlderThanIdleWindow
	idleLeaseEvaluationReasonTerminalActivityWithinIdleWindow
)

func (lease idleLease) evaluate(now time.Time, rootProcessAlive bool) idleLeaseEvaluation {
	now = now.UTC()
	idleAfter := time.Duration(lease.IdleAfter)
	if idleAfter <= 0 {
		return idleLeaseEvaluation{
			State:  idleLeaseStateStale,
			Reason: idleLeaseEvaluationReasonInvalidIdleAfter,
		}
	}
	if lease.ExpiresAt.IsZero() {
		return idleLeaseEvaluation{
			State:  idleLeaseStateStale,
			Reason: idleLeaseEvaluationReasonMissingExpiresAt,
		}
	}
	if lease.UpdatedAt.IsZero() {
		return idleLeaseEvaluation{
			State:  idleLeaseStateStale,
			Reason: idleLeaseEvaluationReasonMissingHeartbeat,
		}
	}
	if now.After(lease.ExpiresAt) {
		return idleLeaseEvaluation{
			State:  idleLeaseStateStale,
			Reason: idleLeaseEvaluationReasonLeaseExpired,
		}
	}
	if now.Sub(lease.UpdatedAt) > idleHeartbeatStaleAfter(idleAfter) {
		return idleLeaseEvaluation{
			State:  idleLeaseStateStale,
			Reason: idleLeaseEvaluationReasonHeartbeatTooOld,
		}
	}
	if !rootProcessAlive {
		return idleLeaseEvaluation{
			State:  idleLeaseStateStale,
			Reason: idleLeaseEvaluationReasonRootProcessNotRunning,
		}
	}

	lastActivity, ok := lease.latestTerminalActivity()
	if !ok {
		return idleLeaseEvaluation{
			State:  idleLeaseStateIdle,
			Reason: idleLeaseEvaluationReasonNoTerminalActivity,
		}
	}
	if !lastActivity.Before(now.Add(-idleAfter)) {
		return idleLeaseEvaluation{
			State:  idleLeaseStateActive,
			Reason: idleLeaseEvaluationReasonTerminalActivityWithinIdleWindow,
		}
	}

	return idleLeaseEvaluation{
		State:  idleLeaseStateIdle,
		Reason: idleLeaseEvaluationReasonTerminalActivityOlderThanIdleWindow,
	}
}

func (lease idleLease) latestTerminalActivity() (time.Time, bool) {
	var latest time.Time
	if lease.LastInputAt != nil {
		latest = *lease.LastInputAt
	}
	if lease.LastOutputAt != nil && lease.LastOutputAt.After(latest) {
		latest = *lease.LastOutputAt
	}
	if latest.IsZero() {
		return time.Time{}, false
	}
	return latest, true
}

func (lease idleLease) expiresAt(now time.Time) time.Time {
	return now.UTC().Add(idleHeartbeatStaleAfter(time.Duration(lease.IdleAfter)) + time.Minute)
}

func idleHeartbeatStaleAfter(idleAfter time.Duration) time.Duration {
	staleAfter := idleAfter * 2
	if staleAfter < 5*time.Minute {
		return 5 * time.Minute
	}
	return staleAfter
}
