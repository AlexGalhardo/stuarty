package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	corepub "github.com/AlexGalhardo/stuarty/core/publish"
	core "github.com/AlexGalhardo/stuarty/core/webhook"
	"github.com/AlexGalhardo/stuarty/service/approval"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
	"github.com/AlexGalhardo/stuarty/service/webhooks"
)

// The events of the bus that become events of a webhook, and the text of the product that must not go with them.
// The link of an unlisted or private post is what keeps it private: the event says the post went out, and only a
// public post gives its address.
func TestWebhookEventCarriesTheAddressOfAPublicPostOnly(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	s := &svc{now: func() time.Time { return now }}

	for visibility, want := range map[string]string{
		string(corepub.Public):   "https://youtu.be/abc",
		string(corepub.Unlisted): "",
		string(corepub.Private):  "",
		"":                       "",
	} {
		got := s.webhookEvents(Event{Type: EventPublicationUpdated, Publication: &pub.Publication{
			ID: "pub_aaaaaaaaaa", ProjectID: "prj_aaaaaaaaaa", Targets: []pub.Target{{
				ID: "tgt_aaaaaaaaa1", Network: "youtube", ConnectionID: "con_aaaaaaaaaa",
				State: corepub.TargetPublished, PublishedAt: &now, URL: "https://youtu.be/abc",
				Fields: pub.Fields{Visibility: visibility},
			}},
		}})
		if len(got) != 1 {
			t.Fatalf("visibility %q: %d events", visibility, len(got))
		}

		d, ok := got[0].(core.PublicationPublishedData)
		if !ok || d.URL != want {
			t.Errorf("visibility %q: url %q, want %q", visibility, d.URL, want)
		}

		body, _, err := core.Build(got[0], now, "wsp_x")
		if err != nil {
			t.Fatal(err)
		}

		if want == "" && strings.Contains(string(body), "youtu.be") {
			t.Errorf("visibility %q: the body holds the address: %s", visibility, body)
		}
	}
}

// hookStore keeps what the dispatcher stores for one webhook that chose every event.
type hookStore struct {
	webhooks.Store

	got []webhooks.Delivery
}

func (h *hookStore) List(context.Context) ([]webhooks.Row, error) {
	return []webhooks.Row{{
		ID: "whk_aaaaaaaaaa", Enabled: true, Events: []string{core.JobFailed, core.ExportReady},
	}}, nil
}

func (h *hookStore) Enqueue(_ context.Context, d webhooks.Delivery) (bool, error) {
	h.got = append(h.got, d)

	return true, nil
}

// A job with no project and no file has no stream to publish on, and a webhook that chose job.failed still hears it.
func TestWebhookHearsTheFailureOfAJobWithNoProject(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	st := &hookStore{}
	s := &svc{now: func() time.Time { return now }}
	s.hooks = webhooks.New(webhooks.Options{
		Store: st, Secrets: webhookSecrets{s}, NewID: func(p string) string { return p + "_aaaaaaaaaa" },
		Clock: svcClock{now: s.now},
	})

	s.emitJob(context.Background(), &JobRow{
		ID: "job_aaaaaaaaaa", Kind: JobImageGenerate, State: JobFailed,
		Error: json.RawMessage(`{"code":"E_PROVIDER","message":"A MESSAGE WITH A PROMPT"}`),
	})

	if len(st.got) != 1 || st.got[0].Event != core.JobFailed {
		t.Fatalf("deliveries = %+v", st.got)
	}

	body := string(st.got[0].Body)
	if !strings.Contains(body, "E_PROVIDER") || strings.Contains(body, "PROMPT") || strings.Contains(body, "project_id") {
		t.Fatalf("body = %s", body)
	}
}

func TestWebhookEventsFromTheBus(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	s := &svc{now: func() time.Time { return now }}

	prj, job, con := "prj_aaaaaaaaaa", "job_aaaaaaaaaa", "con_aaaaaaaaaa"
	recent, old := now.Add(-time.Minute), now.Add(-time.Hour)

	p := &pub.Publication{
		ID:        "pub_aaaaaaaaaa",
		ProjectID: prj,
		Title:     "A TITLE THAT IS NOT METADATA",
		Text:      "A CAPTION THAT IS NOT METADATA",
		Targets: []pub.Target{
			{
				ID:           "tgt_aaaaaaaaa1",
				Network:      "youtube",
				ConnectionID: con,
				State:        corepub.TargetPublished,
				PublishedAt:  &recent,
				URL:          "https://youtu.be/x",
				Fields:       pub.Fields{Visibility: string(corepub.Public)},
			},
			{
				ID: "tgt_aaaaaaaaa2", Network: "x", ConnectionID: con, State: corepub.TargetFailed, UpdatedAt: recent,
				Error: &pub.TargetError{Kind: "quota", Message: "THE ERROR TEXT", Hint: "A HINT"},
			},
			{
				ID:           "tgt_aaaaaaaaa3",
				Network:      "x",
				ConnectionID: con,
				State:        corepub.TargetPublished,
				PublishedAt:  &old,
				URL:          "https://x.example/old",
			},
			{ID: "tgt_aaaaaaaaa4", Network: "x", ConnectionID: con, State: corepub.TargetScheduled},
			{
				ID:           "tgt_aaaaaaaaa5",
				Network:      "x",
				ConnectionID: con,
				State:        corepub.TargetPublished,
				PublishedAt:  &recent,
				URL:          "http://127.0.0.1/local",
			},
		},
	}

	cases := map[string]struct {
		event Event
		want  []string
	}{
		"publication": {
			Event{Type: EventPublicationUpdated, Publication: p},
			[]string{core.PublicationPublished, core.PublicationFailed, core.PublicationPublished},
		},
		"export": {
			Event{
				Type:      "job.updated",
				Kind:      JobExport,
				State:     JobSucceeded,
				JobID:     job,
				ProjectID: prj,
				Result: &JobResult{
					Export: &ExportResult{Format: "mp4", Bytes: 5, Width: 1080, Height: 1920, Path: "/home/a/out.mp4"},
				},
			},
			[]string{core.ExportReady},
		},
		"image export": {
			Event{
				Type: "job.updated", Kind: JobImageExport, State: JobSucceeded, JobID: job,
				Result: &JobResult{Image: &ImageExportResult{
					Format: "png", Bytes: 5, Width: 1080, Height: 1920, Path: "/home/a/out.png", Text: "A DESCRIPTION",
				}},
			},
			[]string{core.ExportReady},
		},
		"export still running": {Event{Type: "job.updated", Kind: JobExport, State: JobRunning, JobID: job}, nil},
		"job failed": {
			Event{
				Type: "job.updated", Kind: "transcribe", State: JobFailed, JobID: job, ProjectID: prj,
				Error: &Error{Code: "E_TRANSCRIBE_FAILED", Message: "A MESSAGE WITH A PROMPT"},
			},
			[]string{core.JobFailed},
		},
		"job cancelled": {Event{Type: "job.updated", Kind: JobExport, State: JobCancelled, JobID: job}, nil},
		"approval": {
			Event{Type: EventApprovalRequested, Approval: &approval.Request{
				ID:           "apr_aaaaaaaaaa",
				Kind:         "publish",
				Summary:      "A SUMMARY THAT IS NOT METADATA",
				ProjectID:    &prj,
				JobID:        &job,
				ConnectionID: &con,
				ExpiresAt:    now.Add(24 * time.Hour),
			}},
			[]string{core.ApprovalRequested},
		},
		"batch":  {Event{Type: "batch.committed", ProjectID: prj, Description: "A DESCRIPTION"}, nil},
		"asset":  {Event{Type: "asset.updated", ProjectID: prj}, nil},
		"nobody": {Event{Type: EventPublicationUpdated}, nil},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			got := s.webhookEvents(tc.event)
			if len(got) != len(tc.want) {
				t.Fatalf("%d events %v, want %v", len(got), got, tc.want)
			}

			for i, d := range got {
				if d.EventName() != tc.want[i] {
					t.Errorf("event %d is %s, want %s", i, d.EventName(), tc.want[i])
				}

				body, _, err := core.Build(d, now, "wsp_x")
				if err != nil {
					t.Fatalf("%s: %v", d.EventName(), err)
				}

				for _, leak := range []string{"NOT METADATA", "ERROR TEXT", "A HINT", "PROMPT", "/home/a", "A DESCRIPTION", "127.0.0.1"} {
					if strings.Contains(string(body), leak) {
						t.Errorf("the body of %s holds %q: %s", d.EventName(), leak, body)
					}
				}
			}
		})
	}
}
