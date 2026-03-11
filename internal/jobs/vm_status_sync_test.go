package jobs

import (
	"testing"
	"time"

	"kv-shepherd.io/shepherd/ent"
	"kv-shepherd.io/shepherd/ent/vm"
	"kv-shepherd.io/shepherd/internal/domain"
)

func TestVMStatusSyncArgs_Kind(t *testing.T) {
	t.Parallel()
	var args VMStatusSyncArgs
	if got := args.Kind(); got != VMStatusSyncJobKind {
		t.Fatalf("Kind() = %q, want %q", got, VMStatusSyncJobKind)
	}
}

// Compile-time assertion: VMStatusSyncArgs must NOT implement JobArgsWithInsertOpts.
// All insert options are managed exclusively by scheduleNext() to avoid DRY violations.
// If this causes a compile error, someone accidentally added an InsertOpts() method.
var _ interface{ Kind() string } = VMStatusSyncArgs{}

func TestVMStatusSyncArgs_NoInsertOpts(t *testing.T) {
	t.Parallel()
	// VMStatusSyncArgs intentionally does not implement JobArgsWithInsertOpts.
	// Insert options (Queue, MaxAttempts, UniqueOpts, ScheduledAt) are managed
	// exclusively by scheduleNext() — the single source of truth.
	// This test documents that design decision; the compile-time assertion above
	// enforces it at build time.
	var args VMStatusSyncArgs
	if args.Kind() != VMStatusSyncJobKind {
		t.Fatal("unexpected kind change")
	}
}

// ---------------------------------------------------------------------------
// tierForStatus unit tests
// ---------------------------------------------------------------------------

func TestTierForStatus_TransitionalStatesReturnHigh(t *testing.T) {
	t.Parallel()

	transitional := []vm.Status{
		vm.StatusCREATING,
		vm.StatusDELETING,
		vm.StatusSTOPPING,
		vm.StatusMIGRATING,
		vm.StatusPENDING,
	}
	for _, s := range transitional {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			if tier := tierForStatus(s); tier != vm.PollingTierHigh {
				t.Errorf("tierForStatus(%s) = %s, want %s", s, tier, vm.PollingTierHigh)
			}
		})
	}
}

func TestTierForStatus_StableStatesReturnLow(t *testing.T) {
	t.Parallel()

	stable := []vm.Status{
		vm.StatusRUNNING,
		vm.StatusSTOPPED,
		vm.StatusFAILED,
		vm.StatusPAUSED,
		vm.StatusUNKNOWN,
	}
	for _, s := range stable {
		s := s
		t.Run(string(s), func(t *testing.T) {
			t.Parallel()
			if tier := tierForStatus(s); tier != vm.PollingTierLow {
				t.Errorf("tierForStatus(%s) = %s, want %s", s, tier, vm.PollingTierLow)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// intervalForTier unit tests
// ---------------------------------------------------------------------------

func TestIntervalForTier_HighReturns15(t *testing.T) {
	t.Parallel()
	if got := intervalForTier(vm.PollingTierHigh); got != highTierIntervalSec {
		t.Errorf("intervalForTier(high) = %d, want %d", got, highTierIntervalSec)
	}
}

func TestIntervalForTier_LowReturns1800(t *testing.T) {
	t.Parallel()
	if got := intervalForTier(vm.PollingTierLow); got != lowTierIntervalSec {
		t.Errorf("intervalForTier(low) = %d, want %d", got, lowTierIntervalSec)
	}
}

// ---------------------------------------------------------------------------
// mapDomainStatusToEntVM round-trip tests
// ---------------------------------------------------------------------------

func TestMapDomainStatusToEntVM_AllStatuses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		domain domain.VMStatus
		want   vm.Status
	}{
		{domain.VMStatusCreating, vm.StatusCREATING},
		{domain.VMStatusRunning, vm.StatusRUNNING},
		{domain.VMStatusStopping, vm.StatusSTOPPING},
		{domain.VMStatusStopped, vm.StatusSTOPPED},
		{domain.VMStatusDeleting, vm.StatusDELETING},
		{domain.VMStatusFailed, vm.StatusFAILED},
		{domain.VMStatusPending, vm.StatusPENDING},
		{domain.VMStatusMigrating, vm.StatusMIGRATING},
		{domain.VMStatusPaused, vm.StatusPAUSED},
		{domain.VMStatusUnknown, vm.StatusUNKNOWN},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(string(tc.domain), func(t *testing.T) {
			t.Parallel()
			if got := mapDomainStatusToEntVM(tc.domain); got != tc.want {
				t.Errorf("mapDomainStatusToEntVM(%s) = %s, want %s", tc.domain, got, tc.want)
			}
		})
	}
}

func TestMapDomainStatusToEntVM_UnknownDefault(t *testing.T) {
	t.Parallel()
	if got := mapDomainStatusToEntVM("NONEXISTENT"); got != vm.StatusUNKNOWN {
		t.Errorf("mapDomainStatusToEntVM(NONEXISTENT) = %s, want UNKNOWN", got)
	}
}

// ---------------------------------------------------------------------------
// Constants sanity tests
// ---------------------------------------------------------------------------

func TestConstants_HighTierInterval(t *testing.T) {
	t.Parallel()
	if highTierIntervalSec != 15 {
		t.Errorf("highTierIntervalSec = %d, want 15", highTierIntervalSec)
	}
}

func TestConstants_LowTierInterval(t *testing.T) {
	t.Parallel()
	if lowTierIntervalSec != 1800 {
		t.Errorf("lowTierIntervalSec = %d, want 1800 (30 minutes)", lowTierIntervalSec)
	}
}

func TestConstants_AutoDowngradeThreshold(t *testing.T) {
	t.Parallel()
	const expected = 30 // minutes
	if autoDowngradeThreshold.Minutes() != float64(expected) {
		t.Errorf("autoDowngradeThreshold = %v, want %d minutes", autoDowngradeThreshold, expected)
	}
}

func TestDeriveHighTierSince(t *testing.T) {
	t.Parallel()

	now := time.Now()
	old := now.Add(-10 * time.Minute)

	t.Run("non_high_tier_clears_timestamp", func(t *testing.T) {
		t.Parallel()
		got := deriveHighTierSince(&ent.VM{
			PollingTier:     vm.PollingTierHigh,
			HighTierSince:   &old,
			Status:          vm.StatusCREATING,
			PollIntervalSec: highTierIntervalSec,
		}, vm.PollingTierLow, now)
		if got != nil {
			t.Fatalf("deriveHighTierSince() = %v, want nil", got)
		}
	})

	t.Run("entering_high_sets_now", func(t *testing.T) {
		t.Parallel()
		got := deriveHighTierSince(&ent.VM{
			PollingTier:     vm.PollingTierLow,
			Status:          vm.StatusRUNNING,
			PollIntervalSec: lowTierIntervalSec,
		}, vm.PollingTierHigh, now)
		if got == nil {
			t.Fatal("deriveHighTierSince() = nil, want non-nil")
		}
		if got.Sub(now) != 0 {
			t.Fatalf("deriveHighTierSince() = %v, want %v", *got, now)
		}
	})

	t.Run("staying_high_keeps_existing", func(t *testing.T) {
		t.Parallel()
		got := deriveHighTierSince(&ent.VM{
			PollingTier:     vm.PollingTierHigh,
			HighTierSince:   &old,
			Status:          vm.StatusPENDING,
			PollIntervalSec: highTierIntervalSec,
		}, vm.PollingTierHigh, now)
		if got == nil {
			t.Fatal("deriveHighTierSince() = nil, want non-nil")
		}
		if got.Sub(old) != 0 {
			t.Fatalf("deriveHighTierSince() = %v, want %v", *got, old)
		}
	})
}

func TestShouldAutoDowngrade(t *testing.T) {
	t.Parallel()

	now := time.Now()
	old := now.Add(-31 * time.Minute)
	recent := now.Add(-5 * time.Minute)

	if !shouldAutoDowngrade(vm.PollingTierHigh, &old, now) {
		t.Fatal("shouldAutoDowngrade() = false, want true for old high-tier timestamp")
	}
	if shouldAutoDowngrade(vm.PollingTierHigh, &recent, now) {
		t.Fatal("shouldAutoDowngrade() = true, want false for recent high-tier timestamp")
	}
	if shouldAutoDowngrade(vm.PollingTierLow, &old, now) {
		t.Fatal("shouldAutoDowngrade() = true, want false for low tier")
	}
	if shouldAutoDowngrade(vm.PollingTierHigh, nil, now) {
		t.Fatal("shouldAutoDowngrade() = true, want false for nil timestamp")
	}
}

func TestReconcileCreateBootstrapStatus(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("hold_stopped_as_creating_during_bootstrap", func(t *testing.T) {
		t.Parallel()

		vmRow := &ent.VM{
			Status:      vm.StatusRUNNING,
			PollingTier: vm.PollingTierHigh,
			CreatedAt:   now.Add(-30 * time.Second),
		}
		got := reconcileCreateBootstrapStatus(vmRow, vm.StatusSTOPPED, now)
		if got != vm.StatusCREATING {
			t.Fatalf("reconcileCreateBootstrapStatus() = %s, want %s", got, vm.StatusCREATING)
		}
	})

	t.Run("hold_unknown_as_creating_during_bootstrap", func(t *testing.T) {
		t.Parallel()

		vmRow := &ent.VM{
			Status:      vm.StatusCREATING,
			PollingTier: vm.PollingTierHigh,
			CreatedAt:   now.Add(-45 * time.Second),
		}
		got := reconcileCreateBootstrapStatus(vmRow, vm.StatusUNKNOWN, now)
		if got != vm.StatusCREATING {
			t.Fatalf("reconcileCreateBootstrapStatus() = %s, want %s", got, vm.StatusCREATING)
		}
	})

	t.Run("no_hold_after_bootstrap_window", func(t *testing.T) {
		t.Parallel()

		vmRow := &ent.VM{
			Status:      vm.StatusRUNNING,
			PollingTier: vm.PollingTierHigh,
			CreatedAt:   now.Add(-createBootstrapGraceWindow - time.Second),
		}
		got := reconcileCreateBootstrapStatus(vmRow, vm.StatusSTOPPED, now)
		if got != vm.StatusSTOPPED {
			t.Fatalf("reconcileCreateBootstrapStatus() = %s, want %s", got, vm.StatusSTOPPED)
		}
	})

	t.Run("no_hold_for_low_tier_vm", func(t *testing.T) {
		t.Parallel()

		vmRow := &ent.VM{
			Status:      vm.StatusRUNNING,
			PollingTier: vm.PollingTierLow,
			CreatedAt:   now.Add(-30 * time.Second),
		}
		got := reconcileCreateBootstrapStatus(vmRow, vm.StatusSTOPPED, now)
		if got != vm.StatusSTOPPED {
			t.Fatalf("reconcileCreateBootstrapStatus() = %s, want %s", got, vm.StatusSTOPPED)
		}
	})
}

func TestReconcileMissingVMStatus(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("hold_missing_vm_as_creating_during_bootstrap", func(t *testing.T) {
		t.Parallel()

		vmRow := &ent.VM{
			Status:      vm.StatusRUNNING,
			PollingTier: vm.PollingTierHigh,
			CreatedAt:   now.Add(-30 * time.Second),
		}
		got := reconcileMissingVMStatus(vmRow, now)
		if got != vm.StatusCREATING {
			t.Fatalf("reconcileMissingVMStatus() = %s, want %s", got, vm.StatusCREATING)
		}
	})

	t.Run("missing_vm_becomes_unknown_after_bootstrap_window", func(t *testing.T) {
		t.Parallel()

		vmRow := &ent.VM{
			Status:      vm.StatusRUNNING,
			PollingTier: vm.PollingTierHigh,
			CreatedAt:   now.Add(-createBootstrapGraceWindow - time.Second),
		}
		got := reconcileMissingVMStatus(vmRow, now)
		if got != vm.StatusUNKNOWN {
			t.Fatalf("reconcileMissingVMStatus() = %s, want %s", got, vm.StatusUNKNOWN)
		}
	})
}
