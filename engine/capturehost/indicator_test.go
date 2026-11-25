package capturehost

import (
	"context"
	"testing"
	"time"
)

func TestIndicatorCountsAndSignals(t *testing.T) {
	now := time.UnixMilli(5000)
	ind := &Indicator{Now: func() time.Time { return now }}

	if a := ind.State(); a.Active || a.Helpers != 0 || a.SinceMS != 0 {
		t.Fatalf("a new indicator: %+v", a)
	}

	changed := ind.Changed()

	ind.Enter()

	select {
	case <-changed:
	default:
		t.Fatal("Enter did not wake the waiter")
	}

	now = now.Add(time.Second)
	ind.Enter()

	if a := ind.State(); !a.Active || a.Helpers != 2 || a.SinceMS != 5000 {
		t.Fatalf("two helpers: %+v", a)
	}

	ind.Leave()

	if a := ind.State(); !a.Active || a.Helpers != 1 {
		t.Fatalf("one left: %+v", a)
	}

	ind.Leave()
	ind.Leave() // more leaves than enters never goes below zero

	if a := ind.State(); a.Active || a.Helpers != 0 || a.SinceMS != 0 {
		t.Fatalf("all left: %+v", a)
	}
}

func TestANilIndicatorIsInert(t *testing.T) {
	var ind *Indicator

	ind.Enter()
	ind.Leave()

	if a := ind.State(); a.Active || ind.Changed() != nil {
		t.Fatalf("nil indicator: %+v", a)
	}
}

// waitOff waits for the indicator to go off, woken by its own changes, and tells whether it did.
func waitOff(ind *Indicator, within time.Duration) bool {
	limit := time.NewTimer(within)
	defer limit.Stop()

	for {
		changed := ind.Changed()
		if !ind.State().Active {
			return true
		}

		select {
		case <-changed:
		case <-limit.C:
			return !ind.State().Active
		}
	}
}

// staysOnWhileAlive checks the indicator on every tick until the process of the helper is gone: it must never be
// off while the process has not exited. The state is read before the flag, and the flag is set before the
// indicator goes off, so an off state with the flag unset is a real fault.
func staysOnWhileAlive(t *testing.T, ind *Indicator, h *Helper) {
	t.Helper()

	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()

	limit := time.NewTimer(15 * time.Second)
	defer limit.Stop()

	for samples := 0; ; samples++ {
		if a := ind.State(); !a.Active && !h.exited.Load() {
			t.Fatalf("the indicator was off while the helper was alive (sample %d)", samples)
		}

		select {
		case <-h.done:
			if samples == 0 {
				t.Fatal("the helper had already gone before the first sample")
			}

			return
		case <-limit.C:
			t.Fatal("the helper outlived its deadline")
		case <-tick.C:
		}
	}
}

// T-167: the indicator is raised when the helper is asked to capture and lowered when its process has exited, and
// a helper that only started, or only listed the sources, never raises it.
func TestSecurity_T167_TheIndicatorFollowsTheHelperProcess(t *testing.T) {
	ind := &Indicator{}
	h := mustStart(t, "ok", Config{Indicator: ind})

	if ind.State().Active {
		t.Fatal("a helper that was not asked to capture lit the indicator")
	}

	_, ev, err := h.Begin(context.Background(), request(t))
	if err != nil {
		t.Fatal(err)
	}

	if a := ind.State(); !a.Active || a.Helpers != 1 || a.SinceMS == 0 {
		t.Fatalf("a capturing helper: %+v", a)
	}

	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}

	last(t, ev)

	if err := h.Close(); err != nil {
		t.Fatalf("Close = %v", err)
	}

	if !waitOff(ind, 10*time.Second) {
		t.Fatalf("the indicator stayed on after the process exited: %+v", ind.State())
	}
}

// T-167: the indicator does not follow the recording, the window or the page: a helper that is killed by its
// deadline, which the service does not stop, keeps it on until the process is really gone.
func TestSecurity_T167_TheIndicatorStaysOnForAHelperKilledByItsDeadline(t *testing.T) {
	ind := &Indicator{}
	h := mustStart(t, "silent", Config{Deadline: 2 * time.Second, Silence: time.Minute, Indicator: ind})

	if _, _, err := h.Begin(context.Background(), request(t)); err != nil {
		t.Fatal(err)
	}

	staysOnWhileAlive(t, ind, h)

	if !waitOff(ind, 5*time.Second) {
		t.Fatalf("the indicator stayed on after the deadline: %+v", ind.State())
	}
}

func TestTheIndicatorGoesOffWhenTheHelperCrashes(t *testing.T) {
	ind := &Indicator{}
	h := mustStart(t, "silent", Config{Silence: time.Minute, Indicator: ind})

	_, ev, err := h.Begin(context.Background(), request(t))
	if err != nil {
		t.Fatal(err)
	}

	h.Kill()
	last(t, ev)

	if !waitOff(ind, 10*time.Second) {
		t.Fatalf("a killed helper left the indicator on: %+v", ind.State())
	}
}

func TestAStartThatFailsAfterTheIndicatorWentOnLeavesItOff(t *testing.T) {
	ind := &Indicator{}
	h := mustStart(t, "helpererror", Config{Indicator: ind})

	if _, _, err := h.Begin(context.Background(), request(t)); err == nil {
		t.Fatal("the helper refused the start and Begin did not say so")
	}

	h.Kill()
	_ = h.Close()

	if !waitOff(ind, 10*time.Second) {
		t.Fatalf("a refused start left the indicator on: %+v", ind.State())
	}
}
