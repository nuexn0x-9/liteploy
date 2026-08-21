package deployment

import (
	"testing"
	"time"
)

func TestValidTransitions(t *testing.T) {
	transitions := []struct {
		from Status
		to   Status
		ok   bool
	}{
		{StatusQueued, StatusPreparing, true},
		{StatusPreparing, StatusBuilding, true},
		{StatusBuilding, StatusStarting, true},
		{StatusStarting, StatusHealthCheck, true},
		{StatusHealthCheck, StatusRouting, true},
		{StatusRouting, StatusSuccess, true},

		// Any active state can fail.
		{StatusQueued, StatusFailed, true},
		{StatusBuilding, StatusFailed, true},
		{StatusHealthCheck, StatusFailed, true},

		// Cancellation.
		{StatusBuilding, StatusCancelling, true},
		{StatusCancelling, StatusCancelled, true},

		// Invalid.
		{StatusSuccess, StatusFailed, false},
		{StatusFailed, StatusSuccess, false},
		{StatusQueued, StatusSuccess, false},
		{StatusBuilding, StatusRouting, false},
		{StatusSuccess, StatusPreparing, false},
	}

	for _, tc := range transitions {
		d := &Deployment{Status: tc.from}
		err := d.Transition(tc.to)
		if tc.ok && err != nil {
			t.Errorf("Transition(%s → %s): expected ok, got %v", tc.from, tc.to, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("Transition(%s → %s): expected error, got nil", tc.from, tc.to)
		}
	}
}

func TestDeploymentFail(t *testing.T) {
	d := &Deployment{Status: StatusBuilding}
	d.Start()
	d.Fail("build", "image build failed: exit code 1")

	if d.Status != StatusFailed {
		t.Errorf("Status = %q, want failed", d.Status)
	}
	if d.Error == "" {
		t.Error("Error should be set after Fail()")
	}
	if d.FinishedAt == nil {
		t.Error("FinishedAt should be set after Fail()")
	}
}

func TestDeploymentSucceed(t *testing.T) {
	d := &Deployment{Status: StatusRouting}
	d.Start()
	time.Sleep(10 * time.Millisecond)
	d.Succeed()

	if d.Status != StatusSuccess {
		t.Errorf("Status = %q, want success", d.Status)
	}
	if d.FinishedAt == nil {
		t.Error("FinishedAt should be set after Succeed()")
	}
	if d.Duration <= 0 {
		t.Errorf("Duration should be > 0, got %f", d.Duration)
	}
}

func TestIsTerminal(t *testing.T) {
	terminal := []Status{StatusSuccess, StatusFailed, StatusCancelled}
	nonTerminal := []Status{StatusQueued, StatusPreparing, StatusBuilding, StatusStarting, StatusHealthCheck, StatusRouting, StatusCancelling}

	for _, s := range terminal {
		if !s.IsTerminal() {
			t.Errorf("%s should be terminal", s)
		}
	}
	for _, s := range nonTerminal {
		if s.IsTerminal() {
			t.Errorf("%s should not be terminal", s)
		}
	}
}
