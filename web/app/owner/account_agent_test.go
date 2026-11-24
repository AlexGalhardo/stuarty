package owner

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/chat"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// When the studio locks, a hosted agent session ends cleanly and its transcript is saved first: the keys that seal
// it are still in memory when the session ends, and every tool of the agent would answer E_LOCKED afterwards
// (epic 57 review, item C; SDD-017 section 5).
func TestAHostedAgentSessionEndsAndIsSavedWhenTheStudioLocks(t *testing.T) {
	dir := t.TempDir()

	agent := filepath.Join(dir, "fakeagent")
	if runtime.GOOS == "windows" {
		agent += ".exe"
	}

	if out, err := exec.CommandContext(context.Background(), "go", "build", "-o", agent,
		"github.com/AlexGalhardo/stuarty/service/agenthost/fakeagent").CombinedOutput(); err != nil {
		t.Fatalf("building the fake agent: %v\n%s", err, out)
	}

	getenv := accountEnv(t)
	ctx := context.Background()

	o, err := Open(ctx, Config{Getenv: getenv})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = o.Close() })

	if _, err := o.Account.Create(ctx, migrateInput, o.createAccount); err != nil {
		t.Fatal(err)
	}

	if err := o.Account.Login(ctx, "stuart", migrateInput.Password); err != nil {
		t.Fatal(err)
	}

	person := service.WithCaller(ctx, service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true})

	if _, err := o.Service.AgentProfileSet(
		person,
		service.AgentProfileSetRequest{Name: "echo", Command: agent},
	); err != nil {
		t.Fatal(err)
	}

	project, err := testproject.CreateWithTimeline(person, o.Service, "Lock")
	if err != nil {
		t.Fatal(err)
	}

	session, err := o.Service.ChatSessionStart(person, service.ChatSessionStartRequest{
		ProjectID: project.ProjectID, Profile: "echo",
	})
	if err != nil {
		t.Fatal(err)
	}

	const words = "CANARY-LOCK-message-to-the-agent"

	if _, err := o.Service.ChatSend(person, service.ChatSendRequest{SessionID: session.ID, Text: words}); err != nil {
		t.Fatal(err)
	}

	if !o.Agents.Live(session.ID) {
		t.Fatal("the session is not live before the lock")
	}

	o.Account.Lock()

	if o.Agents.Live(session.ID) {
		t.Fatal("a hosted agent session stays alive while the studio is locked")
	}

	if err := o.Account.Login(ctx, "stuart", migrateInput.Password); err != nil {
		t.Fatal(err)
	}

	got, err := o.Service.ChatTranscriptGet(person, service.ChatTranscriptRequest{SessionID: session.ID})
	if err != nil {
		t.Fatal(err)
	}

	if got.Session.State != chat.StateEnded || got.Session.EndReason != chat.EndLocked {
		t.Errorf("the session after the lock: state %q, end reason %q", got.Session.State, got.Session.EndReason)
	}

	saved := false

	for _, e := range got.Events {
		if strings.Contains(e.Text, words) {
			saved = true
		}
	}

	if !saved {
		t.Errorf("the transcript saved at the lock does not hold the message: %+v", got.Events)
	}
}
