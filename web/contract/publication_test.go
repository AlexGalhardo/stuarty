package contract

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/fake"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// optFlag is a flag and its value, or nothing when the value is empty.
func optFlag(name, value string) []string {
	if value == "" {
		return nil
	}

	return []string{"--" + name, value}
}

// publicationCommand maps an operation of the calendar to the command that runs it and its arguments (SDD-020
// section 21).
func publicationCommand(s step) (name string, argv []string, stdin string, err error) {
	switch s.Call {
	case "PublicationCreate":
		var r service.PublicationCreateRequest
		if err := json.Unmarshal(s.Args, &r); err != nil {
			return "", nil, "", err
		}

		argv = slices.Concat(optFlag("project", r.ProjectID), optFlag("title", r.Title), optFlag("text", r.Text),
			optFlag("export", r.Export), optFlag("thumbnail", r.Thumbnail))

		argv, err = withTargets(argv, r.Targets)

		return "publication:create", argv, "", err
	case "PublicationGet":
		var r service.PublicationGetRequest
		if err := json.Unmarshal(s.Args, &r); err != nil {
			return "", nil, "", err
		}

		return "publication:show", []string{r.ID}, "", nil
	case "PublicationList":
		var r service.PublicationListRequest
		if err := json.Unmarshal(s.Args, &r); err != nil {
			return "", nil, "", err
		}

		argv = slices.Concat(optFlag("project", r.ProjectID), optFlag("state", r.State), optFlag("network", r.Network),
			optFlag("from", r.From), optFlag("until", r.To))
		if r.Limit > 0 {
			argv = append(argv, "--limit", strconv.Itoa(r.Limit))
		}

		return "publication:list", argv, "", nil
	case "PublicationUpdate":
		var r service.PublicationUpdateRequest
		if err := json.Unmarshal(s.Args, &r); err != nil {
			return "", nil, "", err
		}

		argv = []string{r.ID}
		for _, f := range []struct {
			name string
			v    *string
		}{{"title", r.Title}, {"text", r.Text}, {"export", r.Export}, {"thumbnail", r.Thumbnail}} {
			if f.v != nil {
				argv = append(argv, "--"+f.name, *f.v)
			}
		}

		if len(r.Targets) > 0 {
			raw, merr := json.Marshal(r.Targets)
			if merr != nil {
				return "", nil, "", merr
			}

			argv = append(argv, "--targets", string(raw))
		}

		return "publication:update", argv, "", nil
	case "PublicationSchedule":
		var r service.PublicationScheduleRequest
		if err := json.Unmarshal(s.Args, &r); err != nil {
			return "", nil, "", err
		}

		argv = slices.Concat(
			[]string{r.ID},
			optFlag("target", r.TargetID),
			optFlag("at", r.At),
			optFlag("zone", r.Zone),
		)
		if r.LateWindowMinutes != nil {
			argv = append(argv, "--late-window", strconv.Itoa(*r.LateWindowMinutes))
		}

		return "publication:schedule", argv, "", nil
	case "PublicationSendNow", "PublicationRetry", "PublicationCancel":
		var r service.PublicationTargetRequest
		if err := json.Unmarshal(s.Args, &r); err != nil {
			return "", nil, "", err
		}

		verb := map[string]string{
			"PublicationSendNow": "publication:send-now", "PublicationRetry": "publication:retry",
			"PublicationCancel": "publication:cancel",
		}[s.Call]

		return verb, slices.Concat([]string{r.ID}, optFlag("target", r.TargetID)), "", nil
	}

	return "", nil, "", fmt.Errorf("no CLI mapping for %s", s.Call)
}

// withTargets writes the targets of a request as flags when there is one, and as a JSON list otherwise.
func withTargets(argv []string, targets []pub.TargetInput) ([]string, error) {
	switch {
	case len(targets) == 0:
		return argv, nil
	case len(targets) == 1 && targets[0].ID == "" && !targets[0].Remove:
		t := targets[0]
		argv = slices.Concat(argv, optFlag("connection", t.ConnectionID), optFlag("at", t.At), optFlag("zone", t.Zone),
			optFlag("target-title", t.Fields.Title), optFlag("target-description", t.Fields.Description),
			optFlag("tags", strings.Join(t.Fields.Tags, ",")), optFlag("visibility", t.Fields.Visibility))

		if t.LateWindowMinutes != nil {
			argv = append(argv, "--late-window", strconv.Itoa(*t.LateWindowMinutes))
		}

		if t.Fields.MadeForKids != nil {
			argv = append(argv, "--made-for-kids", map[bool]string{true: "yes", false: "no"}[*t.Fields.MadeForKids])
		}

		if t.Fields.AIDisclosure {
			argv = append(argv, "--ai-disclosure")
		}

		return argv, nil
	}

	raw, err := json.Marshal(targets)
	if err != nil {
		return nil, err
	}

	return append(argv, "--targets", string(raw)), nil
}

// seedPublishedExport writes a succeeded export job of a project and the file it made, as the exporter does.
func seedPublishedExport(t *testing.T, e *testenv.Env, project, id string) {
	t.Helper()

	dir := filepath.Join(string(e.Dir), "exports", project)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}

	data := []byte("an mp4, more or less")
	if err := os.WriteFile(filepath.Join(dir, id+".mp4"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	sum := sha256.Sum256(data)

	res, err := json.Marshal(service.JobResult{Export: &service.ExportResult{
		Format: "mp4", Bytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:]),
	}})
	if err != nil {
		t.Fatal(err)
	}

	err = e.Store.Write(context.Background(), func(ctx context.Context, tx service.Tx) error {
		row := service.JobRow{
			ID: id, Kind: service.JobExport, ProjectID: &project, Actor: "local:" + actor, State: service.JobQueued,
			Input: json.RawMessage(`{}`),
		}
		if err := tx.Jobs().CreateLimited(ctx, row, nil); err != nil {
			return err
		}

		if _, err := tx.Jobs().Claim(ctx, id, fixedNow.UnixMilli()); err != nil {
			return err
		}

		_, err := tx.Jobs().Finish(ctx, id, service.JobSucceeded, res, nil, fixedNow.UnixMilli())

		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The calendar answers the same through the Service, HTTP and the CLI (acceptance criterion 7 of epic 20): the
// same plan, the same request for approval, the same cancel, byte for byte, on a network that is a fake.
func TestPublicationOperationsAreTheSameOnEverySurface(t *testing.T) {
	master := bytes.Repeat([]byte{0x5B}, 32)
	got := map[string][]json.RawMessage{}

	for name, newDriver := range map[string]func(*testing.T, testenv.Options) driver{
		"in-process": newDirectWith, "http": newHTTPWith, "cli": newCLIWith,
	} {
		t.Run(name, func(t *testing.T) {
			network := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/token":
					_, _ = w.Write([]byte(`{"access_token":"a","refresh_token":"r","expires_in":3600}`))
				case "/account":
					_, _ = w.Write([]byte(`{"id":"UC-1","title":"Channel"}`))
				default:
					_, _ = w.Write([]byte("pong"))
				}
			}))
			defer network.Close()

			reg := social.NewRegistry()
			if err := reg.Register(fake.New(network.URL)); err != nil {
				t.Fatal(err)
			}

			d := newDriver(t, testenv.Options{MasterKey: master, Social: reg})
			svc := d.service()
			person := service.WithCaller(
				callerCtx(), service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true},
			)

			begun, err := svc.SocialConnectBegin(person, service.SocialConnectBeginRequest{
				Network: fake.Network, ClientID: "client", ClientSecret: "secret",
				RedirectURI: "http://127.0.0.1:7878/social/callback",
			})
			if err != nil {
				t.Fatal(err)
			}

			authorize, err := url.Parse(begun.AuthorizeURL)
			if err != nil {
				t.Fatal(err)
			}

			state := authorize.Query().Get("state")
			if _, err := svc.SocialConnectComplete(
				person,
				service.SocialConnectCompleteRequest{State: state, Code: "c"},
			); err != nil {
				t.Fatal(err)
			}

			project, err := svc.ProjectCreate(callerCtx(), service.ProjectCreateRequest{Name: "calendar"})
			if err != nil {
				t.Fatal(err)
			}

			export := "job_0000000901"
			seedPublishedExport(t, d.env(), project.ProjectID, export)

			kids := false
			targets := []pub.TargetInput{
				{
					ConnectionID: begun.ConnectionID, At: "2026-12-01T09:00", Zone: "America/Sao_Paulo",
					Fields: pub.Fields{MadeForKids: &kids},
				},
				{
					ConnectionID: begun.ConnectionID, At: "2026-12-02T09:30", Zone: "UTC",
					Fields: pub.Fields{MadeForKids: &kids, Title: "Second"},
				},
			}
			title := "A better title"

			mk := func(call string, v any) step {
				raw, err := json.Marshal(v)
				if err != nil {
					t.Fatal(err)
				}

				return step{Call: call, Args: raw}
			}

			var results []json.RawMessage

			run := func(s step) {
				raw, _, fail := d.do(t, s)
				if fail != nil {
					t.Fatalf("%s %s: %+v", d.name(), s.Call, fail)
				}

				results = append(results, raw)
			}

			// The ids are those of the counter of the environment, the same in each run: the plan is made once and
			// the later calls name what it answered.
			create := mk("PublicationCreate", service.PublicationCreateRequest{
				ProjectID: project.ProjectID, Title: "Week one", Text: "A caption", Export: export, Targets: targets,
			})
			raw, _, fail := d.do(t, create)

			if fail != nil {
				t.Fatalf("%s PublicationCreate: %+v", d.name(), fail)
			}

			results = append(results, raw)

			var made service.Publication
			if err := json.Unmarshal(raw, &made); err != nil || len(made.Targets) != 2 {
				t.Fatalf("%s: %v in %s", d.name(), err, raw)
			}

			first, second := made.Targets[0].ID, made.Targets[1].ID

			run(mk("PublicationGet", service.PublicationGetRequest{ID: made.ID}))
			run(mk("PublicationList", service.PublicationListRequest{ProjectID: project.ProjectID, State: "draft"}))
			run(mk("PublicationUpdate", service.PublicationUpdateRequest{ID: made.ID, Title: &title}))
			run(mk("PublicationSchedule", service.PublicationScheduleRequest{ID: made.ID, TargetID: first}))
			run(mk("PublicationSchedule", service.PublicationScheduleRequest{
				ID: made.ID, TargetID: second, At: "2026-12-03T10:00", Zone: "Europe/Lisbon",
			}))
			run(mk("PublicationSendNow", service.PublicationSendNowRequest{ID: made.ID, TargetID: second}))
			run(mk("PublicationCancel", service.PublicationCancelRequest{ID: made.ID, TargetID: second}))
			run(mk("PublicationGet", service.PublicationGetRequest{ID: made.ID}))

			// An agent cannot approve what it planned: nothing has left, and the request waits.
			pending, err := svc.ApprovalList(callerCtx(), service.ApprovalListRequest{})
			if err != nil || len(pending.Requests) != 1 || pending.Requests[0].Kind != service.KindPublishScheduled {
				t.Fatalf("%s: pending = %+v, %v", d.name(), pending, err)
			}

			got[name] = results
		})
	}

	base := got["in-process"]
	for name, other := range got {
		if len(other) != len(base) {
			t.Fatalf("%s answered %d calls, in-process %d", name, len(other), len(base))
		}

		for i := range base {
			if canonical(t, base[i]) != canonical(t, other[i]) {
				t.Errorf("call %d differs between in-process and %s:\n%s\n%s", i, name, base[i], other[i])
			}
		}
	}
}
