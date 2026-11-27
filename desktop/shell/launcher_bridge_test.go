package shell

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/desktop/update"
)

type stubUpdateClient struct {
	offer    *update.Offer
	checkErr error
	applyErr error
	applied  int
}

func (s *stubUpdateClient) CheckNow(context.Context) (*update.Offer, error) {
	return s.offer, s.checkErr
}

func (s *stubUpdateClient) Apply(context.Context, *update.Offer) error {
	s.applied++
	return s.applyErr
}

type noopLauncherUI struct{}

func (noopLauncherUI) AskConsent(func(bool))                         {}
func (noopLauncherUI) OfferUpdate(*update.Offer, string, func(bool)) {}
func (noopLauncherUI) Inform(string)                                 {}
func (noopLauncherUI) ShowProgress(string)                           {}
func (noopLauncherUI) SetProgress(int64, int64)                      {}
func (noopLauncherUI) HideProgress()                                 {}
func (noopLauncherUI) Undone(string)                                 {}
func (noopLauncherUI) Fail(string)                                   {}

func TestParseBridgeMessageUpdateOps(t *testing.T) {
	for _, op := range []string{OpGetUpdateState, OpCheckForUpdates, OpInstallUpdate} {
		req, ok := ParseBridgeMessage(`{"vukem":"` + op + `","id":12,"path":"C:/ignored"}`)
		if !ok || req.Op != op || req.ID != 12 || req.Path != "" || req.AutoUpdate != nil {
			t.Errorf("%s = %+v, %v", op, req, ok)
		}
	}

	onReq, ok := ParseBridgeMessage(`{"vukem":"setAutoUpdate","id":13,"auto_update":true}`)
	if !ok || onReq.AutoUpdate == nil || !*onReq.AutoUpdate {
		t.Errorf("setAutoUpdate true = %+v, %v", onReq, ok)
	}

	offReq, ok := ParseBridgeMessage(`{"vukem":"setAutoUpdate","id":14,"auto_update":false}`)
	if !ok || offReq.AutoUpdate == nil || *offReq.AutoUpdate {
		t.Errorf("setAutoUpdate false = %+v, %v", offReq, ok)
	}

	if _, ok := ParseBridgeMessage(`{"vukem":"setAutoUpdate","id":15}`); ok {
		t.Error("setAutoUpdate without auto_update boolean was accepted")
	}

	for _, member := range []string{"getUpdateState:", "checkForUpdates:", "installUpdate:", "setAutoUpdate:"} {
		if !strings.Contains(BridgeScript, member) {
			t.Errorf("BridgeScript missing %s", member)
		}
	}
}

func TestLauncherBridgeStateAndOfflineHandling(t *testing.T) {
	client := &stubUpdateClient{
		offer: &update.Offer{Version: "0.62.0", Notes: "Bug fixes"},
	}
	var saved Settings
	l := NewLauncher(LauncherConfig{
		Current:  "0.61.0",
		Client:   client,
		UI:       noopLauncherUI{},
		Settings: Settings{},
		Save: func(s Settings) error {
			saved = s
			return nil
		},
	})
	defer func() {
		l.Close()
		l.Wait()
	}()

	st := l.State()
	if st.CurrentVersion != "0.61.0" || st.Status != "idle" || st.AutoUpdate {
		t.Fatalf("initial state = %+v", st)
	}

	checked := l.CheckState(context.Background())
	if checked.Status != "available" || checked.AvailableVersion != "0.62.0" || checked.ReleaseNotes != "Bug fixes" {
		t.Fatalf("available state = %+v", checked)
	}

	client.offer = nil
	client.checkErr = update.ErrNotNewer
	upToDate := l.CheckState(context.Background())
	if upToDate.Status != "up_to_date" || upToDate.AvailableVersion != "" {
		t.Fatalf("up_to_date state = %+v", upToDate)
	}

	client.checkErr = errors.New("dial tcp: lookup releases.stuarty.com: no such host")
	offline := l.CheckState(context.Background())
	if offline.Status != "offline" || offline.Error != "" {
		t.Fatalf("offline state = %+v", offline)
	}

	if err := l.SetAutoCheck(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if !saved.AutoCheck() || !l.State().AutoUpdate {
		t.Fatalf("auto update not persisted: saved=%+v state=%+v", saved, l.State())
	}

	if err := l.SetAutoCheck(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if saved.AutoCheck() || l.State().AutoUpdate {
		t.Fatalf("auto update not disabled: saved=%+v state=%+v", saved, l.State())
	}

	// Unconfigured launcher reports unconfigured without making network calls.
	unconfigured := NewLauncher(LauncherConfig{Current: "0.61.0", UI: noopLauncherUI{}})
	if got := unconfigured.CheckState(context.Background()); got.Status != "unconfigured" {
		t.Fatalf("unconfigured status = %+v", got)
	}
}
