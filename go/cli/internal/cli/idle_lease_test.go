package cli

import (
	"testing"
	"time"
)

func TestIdleLeaseEvaluatePreservesStatePriority(t *testing.T) {
	now := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	activeAt := now.Add(-10 * time.Second)
	idleAt := now.Add(-45 * time.Minute)
	base := idleLease{
		RootPID:     1234,
		UpdatedAt:   now.Add(-10 * time.Second),
		LastInputAt: &activeAt,
		IdleAfter:   idleLeaseDuration(30 * time.Minute),
		ExpiresAt:   now.Add(30 * time.Minute),
	}

	tests := []struct {
		name             string
		lease            idleLease
		rootProcessAlive bool
		wantState        idleLeaseState
		wantReason       idleLeaseEvaluationReason
	}{
		{
			name:             "invalid idle window wins before missing expiresAt",
			lease:            idleLease{IdleAfter: 0},
			rootProcessAlive: true,
			wantState:        idleLeaseStateStale,
			wantReason:       idleLeaseEvaluationReasonInvalidIdleAfter,
		},
		{
			name:             "missing expiresAt",
			lease:            withIdleLease(base, func(lease *idleLease) { lease.ExpiresAt = time.Time{} }),
			rootProcessAlive: true,
			wantState:        idleLeaseStateStale,
			wantReason:       idleLeaseEvaluationReasonMissingExpiresAt,
		},
		{
			name:             "missing heartbeat",
			lease:            withIdleLease(base, func(lease *idleLease) { lease.UpdatedAt = time.Time{} }),
			rootProcessAlive: true,
			wantState:        idleLeaseStateStale,
			wantReason:       idleLeaseEvaluationReasonMissingHeartbeat,
		},
		{
			name:             "expired lease",
			lease:            withIdleLease(base, func(lease *idleLease) { lease.ExpiresAt = now.Add(-time.Second) }),
			rootProcessAlive: true,
			wantState:        idleLeaseStateStale,
			wantReason:       idleLeaseEvaluationReasonLeaseExpired,
		},
		{
			name: "heartbeat too old",
			lease: withIdleLease(base, func(lease *idleLease) {
				lease.UpdatedAt = now.Add(-61 * time.Minute)
				lease.ExpiresAt = now.Add(time.Minute)
			}),
			rootProcessAlive: true,
			wantState:        idleLeaseStateStale,
			wantReason:       idleLeaseEvaluationReasonHeartbeatTooOld,
		},
		{
			name:             "root process not running",
			lease:            base,
			rootProcessAlive: false,
			wantState:        idleLeaseStateStale,
			wantReason:       idleLeaseEvaluationReasonRootProcessNotRunning,
		},
		{
			name:             "no terminal activity",
			lease:            withIdleLease(base, func(lease *idleLease) { lease.LastInputAt = nil }),
			rootProcessAlive: true,
			wantState:        idleLeaseStateIdle,
			wantReason:       idleLeaseEvaluationReasonNoTerminalActivity,
		},
		{
			name:             "terminal activity older than idle window",
			lease:            withIdleLease(base, func(lease *idleLease) { lease.LastInputAt = &idleAt }),
			rootProcessAlive: true,
			wantState:        idleLeaseStateIdle,
			wantReason:       idleLeaseEvaluationReasonTerminalActivityOlderThanIdleWindow,
		},
		{
			name:             "terminal activity within idle window",
			lease:            base,
			rootProcessAlive: true,
			wantState:        idleLeaseStateActive,
			wantReason:       idleLeaseEvaluationReasonTerminalActivityWithinIdleWindow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.lease.evaluate(now, tt.rootProcessAlive)
			if got.State != tt.wantState || got.Reason != tt.wantReason {
				t.Fatalf("evaluation mismatch: want %s/%d, got %s/%d", tt.wantState, tt.wantReason, got.State, got.Reason)
			}
		})
	}
}

func TestIdleLeaseRecordTerminalActivityReturnsFlushDecision(t *testing.T) {
	firstActivity := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	lease := idleLease{IdleAfter: idleLeaseDuration(time.Minute)}

	first := lease.recordTerminalActivity(firstActivity, idleLeaseTerminalActivityOutput)
	if !first.FlushNow {
		t.Fatal("first terminal activity should flush immediately")
	}
	if first.Lease.LastOutputAt == nil || !first.Lease.LastOutputAt.Equal(firstActivity) {
		t.Fatalf("first output activity mismatch: %#v", first.Lease.LastOutputAt)
	}
	if first.Lease.LastInputAt != nil {
		t.Fatalf("output activity should not update input activity: %#v", first.Lease.LastInputAt)
	}

	secondActivity := firstActivity.Add(10 * time.Second)
	second := first.Lease.recordTerminalActivity(secondActivity, idleLeaseTerminalActivityOutput)
	if second.FlushNow {
		t.Fatal("continuing terminal activity should wait for the next heartbeat")
	}
	if second.Lease.LastOutputAt == nil || !second.Lease.LastOutputAt.Equal(secondActivity) {
		t.Fatalf("second output activity mismatch: %#v", second.Lease.LastOutputAt)
	}

	resumedActivity := firstActivity.Add(2 * time.Minute)
	resumed := second.Lease.recordTerminalActivity(resumedActivity, idleLeaseTerminalActivityInput)
	if !resumed.FlushNow {
		t.Fatal("activity after the idle window should flush immediately")
	}
	if resumed.Lease.LastInputAt == nil || !resumed.Lease.LastInputAt.Equal(resumedActivity) {
		t.Fatalf("input activity mismatch: %#v", resumed.Lease.LastInputAt)
	}
}

func TestIdleLeaseRecordHeartbeatUpdatesExpiry(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	lease := idleLease{IdleAfter: idleLeaseDuration(30 * time.Minute)}

	got := lease.recordHeartbeat(now)
	if !got.UpdatedAt.Equal(now) {
		t.Fatalf("updatedAt mismatch: want %s, got %s", now, got.UpdatedAt)
	}
	if want := now.Add(61 * time.Minute); !got.ExpiresAt.Equal(want) {
		t.Fatalf("expiresAt mismatch: want %s, got %s", want, got.ExpiresAt)
	}
}

func withIdleLease(lease idleLease, mutate func(*idleLease)) idleLease {
	mutate(&lease)
	return lease
}
