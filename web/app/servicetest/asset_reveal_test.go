package servicetest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/internal/testfs"
	"github.com/AlexGalhardo/stuarty/service"
)

func (m *mediaEnv) reveal(ctx context.Context, hash string) (*service.AssetRevealResult, *service.Error) {
	m.t.Helper()

	res, err := m.svc.AssetReveal(ctx, service.AssetRevealRequest{Hash: hash})
	if err == nil {
		return res, nil
	}

	se, ok := errors.AsType[*service.Error](err)
	if !ok {
		m.t.Fatalf("AssetReveal: %v is not a service error", err)
	}

	return nil, se
}

func wantCode(t *testing.T, se *service.Error, code, reason string) {
	t.Helper()

	if se == nil {
		t.Fatalf("no error, want %s %s", code, reason)
	}

	if se.Code != code || reason != "" && se.Details["reason"] != reason {
		t.Errorf("error = %s %v (%s), want %s %s", se.Code, se.Details, se.Message, code, reason)
	}
}

// pathFree fails when the error names a place on the disk: no message, hint, path member or detail may.
func pathFree(t *testing.T, se *service.Error, places ...string) {
	t.Helper()

	raw, err := json.Marshal(se)
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range places {
		for _, form := range []string{p, strings.ReplaceAll(p, `\`, `\\`), filepath.ToSlash(p)} {
			if form != "" && strings.Contains(string(raw), form) {
				t.Errorf("the error carries the place %q: %s", p, raw)
			}
		}
	}
}

func (m *mediaEnv) storedFile(hash string) string {
	m.t.Helper()

	p, err := enginestore.New(m.ws).Path(strings.TrimPrefix(hash, "sha256:"))
	if err != nil {
		m.t.Fatal(err)
	}

	return p
}

func TestAssetRevealAPersonGetsTheStoredPath(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	hash := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0].Hash

	res, se := m.reveal(m.asPerson(), hash)
	if se != nil {
		t.Fatalf("reveal = %v", se)
	}

	want, err := filepath.EvalSymlinks(m.storedFile(hash))
	if err != nil {
		t.Fatal(err)
	}

	if res.Path != want || res.Display != res.Path {
		t.Errorf("path = %q display = %q, want %q", res.Path, res.Display, want)
	}

	if body, rerr := os.ReadFile(res.Path); rerr != nil || string(body) != clipBytes {
		t.Errorf("the path does not lead to the stored file: %v", rerr)
	}

	// A bare 64 hex string is the same hash, as everywhere in the library.
	if again, se := m.reveal(m.asPerson(), strings.TrimPrefix(hash, "sha256:")); se != nil || again.Path != res.Path {
		t.Errorf("bare hash = %+v, %v", again, se)
	}
}

// The request carries a hash and never a path: a path where the hash goes is a validation error, and the path
// that was sent is not echoed.
func TestAssetRevealTheRequestCarriesAHashNeverAPath(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	hash := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0].Hash
	file := m.storedFile(hash)

	for _, sent := range []string{file, filepath.Dir(file), "../../etc/passwd", `C:\Windows\win.ini`, "", "sha256:short"} {
		_, se := m.reveal(m.asPerson(), sent)
		wantCode(t, se, command.CodeValidation, "")
		pathFree(t, se, sent)
	}
}

func TestAssetRevealAnUnknownHashIsNotFound(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})

	_, se := m.reveal(m.asPerson(), "sha256:"+strings.Repeat("a", 64))
	wantCode(t, se, command.CodeNotFound, "")
}

// A file whose only use is in the trash is refused with a reason a person can act on, and the same file comes
// back when it is restored.
func TestAssetRevealAFileInTheTrashIsRefused(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	in := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0]

	if _, err := m.svc.MediaTrash(
		m.as(ui),
		service.MediaTrashRequest{ProjectID: project, AssetID: in.AssetID},
	); err != nil {
		t.Fatal(err)
	}

	_, se := m.reveal(m.asPerson(), in.Hash)
	wantCode(t, se, command.CodeNotFound, "in_trash")
	pathFree(t, se, m.ws)

	if se.Hint == "" {
		t.Error("the refusal has no hint")
	}

	if _, err := m.svc.MediaRestore(
		m.as(ui),
		service.MediaTrashRequest{ProjectID: project, AssetID: in.AssetID},
	); err != nil {
		t.Fatal(err)
	}

	if _, se := m.reveal(m.asPerson(), in.Hash); se != nil {
		t.Errorf("a restored file was refused: %v", se)
	}
}

func TestAssetRevealAMissingFileIsRefusedWithNoPath(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	hash := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0].Hash
	file := m.storedFile(hash)
	m.removeStored(hash)

	_, se := m.reveal(m.asPerson(), hash)
	wantCode(t, se, "E_FILE_NOT_FOUND", "file_missing")
	pathFree(t, se, m.ws, file, filepath.Dir(file))

	if !strings.Contains(se.Message, "missing") {
		t.Errorf("the message does not say the file is missing: %q", se.Message)
	}
}

func TestAssetRevealAFileStillImportingIsRefused(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{noQueue: true})
	project := m.newProject("p")
	hash := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0].Hash

	_, se := m.reveal(m.asPerson(), hash)
	wantCode(t, se, "E_ASSET_NOT_READY", "")
	pathFree(t, se, m.ws)
}

// The same file in two projects is one stored file: one path, from either.
func TestAssetRevealASharedFileHasOnePath(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	one, two := m.newProject("one"), m.newProject("two")
	hash := m.importOne(one, m.write("clip.mp4", clipBytes)).Assets[0].Hash
	m.importOne(two, m.write("again.mp4", clipBytes))

	res, se := m.reveal(m.asPerson(), hash)
	if se != nil {
		t.Fatal(se)
	}

	if res.Path != filepath.Clean(res.Path) || strings.Count(res.Path, hash[len("sha256:"):]) != 1 {
		t.Errorf("path = %q", res.Path)
	}
}

// T-123: an agent, an API key, the local token and a hosted agent can never read a path.
func TestSecurity_T123_AnAgentCannotReadAFilePath(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	hash := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0].Hash
	file := m.storedFile(hash)

	callers := map[string]context.Context{
		"an MCP client or a script with the local token": m.as(ui),
		"an agent of the local machine":                  m.as(agent),
		"an API key with every scope": service.WithCaller(context.Background(), service.Caller{
			Actor: key, Scopes: service.AllScopes,
		}),
		"an API key of a user": service.WithCaller(context.Background(), service.Caller{
			Actor: key, Scopes: service.AllScopes, OwnerUserID: owner,
		}),
		"a hosted agent": service.WithCaller(context.Background(), service.Caller{
			Actor: "agent:claude-code", Scopes: service.AllScopes,
		}),
		"an operator of the CLI": service.WithCaller(context.Background(), service.Caller{
			Actor: ui, Scopes: service.AllScopes, Operator: true,
		}),
		"a key with the read scope only": service.WithCaller(context.Background(), service.Caller{
			Actor: key, Scopes: []string{service.ScopeRead},
		}),
	}

	for name, ctx := range callers {
		_, se := m.reveal(ctx, hash)
		wantCode(t, se, service.CodePrivilegedSession, "")
		pathFree(t, se, m.ws, file, filepath.Dir(file))

		if se.Hint == "" {
			t.Errorf("%s: the refusal has no hint", name)
		}
	}

	// No caller at all is unauthorized, not privileged.
	_, se := m.reveal(context.Background(), hash)
	wantCode(t, se, service.CodeUnauthorized, "")
}

// T-123: in network mode no caller, a person included, gets a path of the server's disk.
func TestSecurity_T123_NetworkModeNeverRevealsAPath(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	hash := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0].Hash

	person := service.WithCaller(context.Background(), service.Caller{
		Actor: "user:" + owner, Scopes: service.AllScopes, OwnerUserID: owner, Browser: true,
	})
	admin := service.WithCaller(context.Background(), service.Caller{
		Actor: "user:" + owner, Scopes: service.AllScopes, OwnerUserID: owner, Browser: true,
	})

	for _, ctx := range []context.Context{person, admin} {
		res, se := m.reveal(ctx, hash)
		if res != nil {
			t.Fatalf("a path was returned in network mode: %+v", res)
		}

		wantCode(t, se, command.CodeNotFound, "network_mode")
		pathFree(t, se, m.ws)
	}
}

// T-123: a link inside the store that leads out of it is not followed.
func TestSecurity_T123_ASymlinkOutOfTheMediaRootIsRefused(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	project := m.newProject("p")
	hash := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0].Hash
	file := m.storedFile(hash)

	outside := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(outside, filepath.Base(file)),
		[]byte("a file the store never held"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	if err := os.RemoveAll(filepath.Dir(file)); err != nil {
		t.Fatal(err)
	}

	testfs.LinkDir(t, filepath.Dir(file), outside)

	_, se := m.reveal(m.asPerson(), hash)
	if se == nil || se.Code != "E_PATH_NOT_ALLOWED" {
		t.Fatalf("error = %+v, want E_PATH_NOT_ALLOWED", se)
	}

	pathFree(t, se, m.ws, outside)
}

// T-123: a locked studio reveals nothing, to a person either.
func TestSecurity_T123_ALockedStudioRevealsNothing(t *testing.T) {
	g := &gate{}
	e := newEnvWith(t, func(_ *env, o *service.Options) { o.Gate = g })
	hash := "sha256:" + strings.Repeat("a", 64)

	g.locked.Store(true)

	for name, ctx := range map[string]context.Context{"a person": e.asPerson(), "the local token": e.as(ui)} {
		_, err := e.svc.AssetReveal(ctx, service.AssetRevealRequest{Hash: hash})

		se, ok := errors.AsType[*service.Error](err)
		if !ok || se.Code != service.CodeLocked {
			t.Errorf("%s while locked: %v, want %s", name, err, service.CodeLocked)
		}
	}
}

// T-123: the path is in the answer to the person and nowhere else: not in a log line, an event, the audit, an
// error, the entry of the library or its text.
func TestSecurity_T123_NoPathInAnErrorAnEventOrTheAudit(t *testing.T) {
	var (
		mu   sync.Mutex
		logs bytes.Buffer
	)

	m := newMediaEnv(t, mediaCfg{
		log: slog.New(
			slog.NewTextHandler(&lockedWriter{mu: &mu, w: &logs}, &slog.HandlerOptions{Level: slog.LevelDebug}),
		),
	})
	project := m.newProject("p")
	hash := m.importOne(project, m.write("clip.mp4", clipBytes)).Assets[0].Hash

	res, se := m.reveal(m.asPerson(), hash)
	if se != nil {
		t.Fatal(se)
	}

	// Refusals as well, so the error paths are covered too.
	m.reveal(m.as(agent), hash)
	m.reveal(m.asPerson(), "sha256:"+strings.Repeat("b", 64))
	m.removeStored(hash)
	m.reveal(m.asPerson(), hash)

	places := []string{res.Path, filepath.Dir(res.Path), m.ws}

	mu.Lock()
	text := logs.String()
	mu.Unlock()

	check := func(where, body string) {
		t.Helper()

		for _, p := range places {
			for _, form := range []string{p, strings.ReplaceAll(p, `\`, `\\`), filepath.ToSlash(p)} {
				if strings.Contains(body, form) {
					t.Errorf("%s carries the place %q", where, p)
				}
			}
		}
	}

	check("the log", text)

	m.mu.Lock()
	events := fmt.Sprintf("%+v", m.events)
	m.mu.Unlock()

	check("an event", events)

	audit, err := json.Marshal(m.audits("asset.rename", "media.import", "media.trash"))
	if err != nil {
		t.Fatal(err)
	}

	check("the audit", string(audit))

	entry, err := m.svc.AssetGet(m.asPerson(), service.AssetRef{Hash: hash})
	if err != nil {
		t.Fatal(err)
	}

	got, err := json.Marshal(entry)
	if err != nil {
		t.Fatal(err)
	}

	check("the entry of the library", string(got))

	list, err := m.svc.AssetList(m.asPerson(), service.AssetListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	got, err = json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}

	check("the list of the library", string(got))
}
