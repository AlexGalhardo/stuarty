package shell

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AlexGalhardo/vukem-video/desktop/update"
)

type fakeClient struct {
	mu       sync.Mutex
	results  []error // one per CheckNow, the last repeats; nil means the offer
	offer    *update.Offer
	checks   int
	applied  []string
	applyErr error
}

func (f *fakeClient) CheckNow(context.Context) (*update.Offer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	i := min(f.checks, len(f.results)-1)
	f.checks++

	if f.results[i] != nil {
		return nil, f.results[i]
	}

	return f.offer, nil
}

func (f *fakeClient) Apply(_ context.Context, o *update.Offer) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.applied = append(f.applied, o.Version)

	return f.applyErr
}

func (f *fakeClient) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.checks
}

type fakeUI struct {
	mu       sync.Mutex
	consent  func(bool)
	asked    int
	offers   []string
	answer   *bool // answers every offer; nil leaves it unanswered
	informed []string
	failed   []string
	progress []int64
	shown    int
	hidden   int
}

func (u *fakeUI) AskConsent(answer func(bool)) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.asked++
	u.consent = answer
}

func (u *fakeUI) OfferUpdate(o *update.Offer, _ string, decide func(bool)) {
	u.mu.Lock()
	u.offers = append(u.offers, o.Version)
	answer := u.answer
	u.mu.Unlock()

	if answer != nil {
		decide(*answer)
	}
}

func (u *fakeUI) Undone(v string) {
	u.mu.Lock()
	u.informed = append(u.informed, "undone "+v)
	u.mu.Unlock()
}

func (u *fakeUI) Inform(m string)     { u.mu.Lock(); u.informed = append(u.informed, m); u.mu.Unlock() }
func (u *fakeUI) Fail(m string)       { u.mu.Lock(); u.failed = append(u.failed, m); u.mu.Unlock() }
func (u *fakeUI) ShowProgress(string) { u.mu.Lock(); u.shown++; u.mu.Unlock() }
func (u *fakeUI) HideProgress()       { u.mu.Lock(); u.hidden++; u.mu.Unlock() }
func (u *fakeUI) SetProgress(d, _ int64) {
	u.mu.Lock()
	u.progress = append(u.progress, d)
	u.mu.Unlock()
}

func (u *fakeUI) offered() []string {
	u.mu.Lock()
	defer u.mu.Unlock()

	return append([]string(nil), u.offers...)
}

// clock is the injected After: every wait is recorded and released by the test.
type clock struct {
	mu     sync.Mutex
	delays []time.Duration
	fire   chan time.Time
}

func newClock() *clock { return &clock{fire: make(chan time.Time)} }

func (c *clock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	c.delays = append(c.delays, d)
	c.mu.Unlock()

	return c.fire
}

func (c *clock) waits() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()

	return append([]time.Duration(nil), c.delays...)
}

func eventually(t *testing.T, what string, ok func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}

		time.Sleep(time.Millisecond)
	}
}

type harness struct {
	l      *Launcher
	client *fakeClient
	ui     *fakeUI
	clk    *clock
	saved  []Settings
}

func newHarness(t *testing.T, s Settings, client *fakeClient) *harness {
	t.Helper()

	h := &harness{client: client, ui: &fakeUI{}, clk: newClock()}
	cfg := LauncherConfig{
		Settings: s,
		Save:     func(s Settings) error { h.saved = append(h.saved, s); return nil },
		UI:       h.ui, Current: "0.20.0", After: h.clk.After,
	}

	if client != nil {
		cfg.Client = client
	}

	h.l = NewLauncher(cfg)

	t.Cleanup(func() { h.l.Close() })

	return h
}

var newer = &update.Offer{Version: "0.21.0", Notes: "Notes"}

func TestFirstStartAsksAndConnectsToNothingBeforeTheAnswer(t *testing.T) {
	h := newHarness(t, Settings{}, &fakeClient{results: []error{update.ErrNotNewer}})
	h.l.Start(context.Background())

	if h.ui.asked != 1 || h.client.count() != 0 || len(h.saved) != 0 {
		t.Fatalf("asked %d, checks %d, saved %d", h.ui.asked, h.client.count(), len(h.saved))
	}

	h.ui.consent(true)

	eventually(t, "the first check", func() bool { return h.client.count() == 1 })

	if len(h.saved) != 1 || !h.saved[0].AutoCheck() {
		t.Errorf("saved %+v", h.saved)
	}
}

func TestAnsweringNoSavesItAndNeverConnects(t *testing.T) {
	h := newHarness(t, Settings{}, &fakeClient{results: []error{update.ErrNotNewer}})
	h.l.Start(context.Background())
	h.ui.consent(false)

	if len(h.saved) != 1 || !h.saved[0].Asked() || h.saved[0].AutoCheck() || h.client.count() != 0 {
		t.Fatalf("saved %+v, checks %d", h.saved, h.client.count())
	}

	h.l.CheckNow(context.Background())
	h.l.Wait()

	if h.client.count() != 1 || len(h.ui.informed) != 1 {
		t.Errorf("the manual check must run once: checks %d, informed %v", h.client.count(), h.ui.informed)
	}
}

func TestDismissedQuestionSavesNothingAndConnectsToNothing(t *testing.T) {
	h := newHarness(t, Settings{}, &fakeClient{results: []error{update.ErrNotNewer}})
	h.l.Start(context.Background())

	if len(h.saved) != 0 || h.client.count() != 0 || h.l.AutoCheck() {
		t.Errorf("saved %v, checks %d", h.saved, h.client.count())
	}
}

func TestBuildWithoutAClientNeverAsks(t *testing.T) {
	h := newHarness(t, Settings{}, nil)
	h.l.Start(context.Background())

	if h.ui.asked != 0 {
		t.Error("a build that cannot update must not ask")
	}
}

func TestTurningTheScheduleOnWithoutAClientDoesNotPanic(t *testing.T) {
	h := newHarness(t, Settings{}.WithAutoCheck(true), nil)
	h.l.Start(context.Background())

	if err := h.l.SetAutoCheck(context.Background(), true); err != nil {
		t.Fatal(err)
	}

	h.l.Wait()

	if h.l.CanUpdate() || h.l.AutoCheck() && len(h.saved) != 0 {
		t.Errorf("CanUpdate %v, saved %v", h.l.CanUpdate(), h.saved)
	}
}

func TestAnsweredYesChecksAtOnceThenEvery24Hours(t *testing.T) {
	h := newHarness(t, Settings{}.WithAutoCheck(true), &fakeClient{results: []error{update.ErrNotNewer}})
	h.l.Start(context.Background())

	eventually(t, "check 1", func() bool { return h.client.count() == 1 })
	eventually(t, "the first wait", func() bool { return len(h.clk.waits()) == 1 })

	h.clk.fire <- time.Now()

	eventually(t, "check 2", func() bool { return h.client.count() == 2 })
	eventually(t, "the second wait", func() bool { return len(h.clk.waits()) == 2 })

	for _, d := range h.clk.waits() {
		if d != CheckInterval {
			t.Errorf("wait %v, want %v", d, CheckInterval)
		}
	}

	if h.ui.asked != 0 {
		t.Error("an answered question must not be asked again")
	}
}

func TestAFailedCheckIsSilentAndRetriedSooner(t *testing.T) {
	h := newHarness(t, Settings{}.WithAutoCheck(true),
		&fakeClient{results: []error{errors.New("offline"), update.ErrNotNewer}})
	h.l.Start(context.Background())

	eventually(t, "the wait", func() bool { return len(h.clk.waits()) == 1 })

	if h.clk.waits()[0] != RetryInterval {
		t.Errorf("retry after %v", h.clk.waits()[0])
	}

	if len(h.ui.offers)+len(h.ui.informed)+len(h.ui.failed) != 0 {
		t.Error("a failed automatic check must show nothing")
	}

	h.clk.fire <- time.Now()
	eventually(t, "the second wait", func() bool { return len(h.clk.waits()) == 2 })

	if h.clk.waits()[1] != CheckInterval {
		t.Errorf("after a good check wait %v", h.clk.waits()[1])
	}
}

func TestNotNowIsNotOfferedAgainThisSession(t *testing.T) {
	h := newHarness(t, Settings{}.WithAutoCheck(true), &fakeClient{offer: newer, results: []error{nil}})
	no := false
	h.ui.answer = &no
	h.l.Start(context.Background())

	eventually(t, "the first wait", func() bool { return len(h.clk.waits()) == 1 })
	h.clk.fire <- time.Now()
	eventually(t, "check 2", func() bool { return h.client.count() == 2 })
	eventually(t, "the second wait", func() bool { return len(h.clk.waits()) == 2 })

	if got := h.ui.offered(); len(got) != 1 {
		t.Errorf("offered %v, want it once", got)
	}
}

func TestAcceptingInstallsWithProgressAndAFailureIsShown(t *testing.T) {
	client := &fakeClient{offer: newer, results: []error{nil}, applyErr: errors.New("disk full")}
	h := newHarness(t, Settings{}.WithAutoCheck(true), client)
	yes := true
	h.ui.answer = &yes
	h.l.Start(context.Background())

	eventually(t, "the failure", func() bool {
		h.ui.mu.Lock()
		defer h.ui.mu.Unlock()

		return len(h.ui.failed) == 1
	})

	if len(client.applied) != 1 || client.applied[0] != "0.21.0" || h.ui.shown != 1 || h.ui.hidden != 1 {
		t.Errorf("applied %v shown %d hidden %d", client.applied, h.ui.shown, h.ui.hidden)
	}
}

func TestTurningTheSettingOffStopsTheSchedule(t *testing.T) {
	h := newHarness(t, Settings{}.WithAutoCheck(true), &fakeClient{results: []error{update.ErrNotNewer}})
	h.l.Start(context.Background())

	eventually(t, "the wait", func() bool { return len(h.clk.waits()) == 1 })

	if err := h.l.SetAutoCheck(context.Background(), false); err != nil {
		t.Fatal(err)
	}

	h.l.Wait()

	if h.client.count() != 1 || h.l.AutoCheck() || len(h.saved) != 1 || h.saved[0].AutoCheck() {
		t.Errorf("checks %d, saved %+v", h.client.count(), h.saved)
	}
}

func TestCheckNowWithoutAClientShowsTheNotConfiguredMessage(t *testing.T) {
	ui := &fakeUI{}
	l := NewLauncher(LauncherConfig{UI: ui, BuildErr: update.ErrNotConfigured, Current: "0.20.0"})

	l.CheckNow(context.Background())
	l.Wait()

	if len(ui.informed) != 1 || !strings.Contains(ui.informed[0], "no update feed or signing key") {
		t.Errorf("informed %q", ui.informed)
	}
}

func TestARolledBackVersionIsToldOnceAndNeverScheduled(t *testing.T) {
	notified := 0
	ui := &fakeUI{}
	no := true
	ui.answer = &no
	client := &fakeClient{offer: &update.Offer{Version: "0.21.0"}, results: []error{nil}}
	l := NewLauncher(LauncherConfig{
		Settings: Settings{}.WithAutoCheck(true), UI: ui, Client: client, Current: "0.20.0",
		After: newClock().After, RolledBack: "0.21.0", Notify: true, Notified: func() { notified++ },
	})
	t.Cleanup(l.Close)

	l.Start(context.Background())
	eventually(t, "the check", func() bool { return client.count() == 1 })

	if notified != 1 || len(ui.informed) != 1 || ui.informed[0] != "undone 0.21.0" {
		t.Errorf("notified %d, informed %v", notified, ui.informed)
	}

	if len(ui.offered()) != 0 {
		t.Error("the schedule must not offer the version that was rolled back")
	}
}
