package servicetest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service"
)

// SDD-007 T-100: a picture leaves the machine only after the person agreed, and only to the connection chosen.
func TestSecurity_T100_NoPictureLeavesBeforeApproval(t *testing.T) {
	t.Parallel()

	e := newEditEnv(t)

	job, err := e.removeBackground()
	if err != nil || job.State != service.JobWaitingApproval {
		t.Fatalf("job = %+v, %v", job, err)
	}

	// Waiting: no request, whatever else happens to the document.
	if e.api.edits.Load() != 0 {
		t.Fatal("the picture left while the request waited for a person")
	}

	// A dry run, a rejected request and a cancelled job never send it.
	if _, derr := e.svc.ImageModelEdit(e.as(token), service.ImageModelEditRequest{
		ProjectID: e.prj, DryRun: true,
		ConnectionID: e.imageConn, ImageID: e.image, Operation: provider.OperationRemoveBackground,
		Model: imageModel, LayerID: e.layer,
	}); derr != nil {
		t.Fatal(derr)
	}

	if _, rerr := e.svc.ApprovalDecide(e.asPerson(), service.ApprovalDecideRequest{
		ID: job.ApprovalID, Decision: "reject",
	}); rerr != nil {
		t.Fatal(rerr)
	}

	if done := e.jobOf(job.JobID); done.State != service.JobCancelled {
		t.Fatalf("a rejected request left the job %s", done.State)
	}

	// The agent cannot approve its own request.
	again, err := e.removeBackground()
	if err != nil {
		t.Fatal(err)
	}

	if _, aerr := e.svc.ApprovalDecide(e.as(token), service.ApprovalDecideRequest{
		ID: again.ApprovalID, Decision: "approve",
	}); code(aerr) != service.CodePrivilegedSession {
		t.Fatalf("an agent approved its own request: %v", aerr)
	}

	// A connection that accepts text only is not a place to send a picture.
	_, cerr := e.svc.ImageModelEdit(e.as(token), service.ImageModelEditRequest{
		ProjectID:    e.prj,
		ConnectionID: e.conn, ImageID: e.image, Operation: provider.OperationRemoveBackground,
		Model: imageModel, LayerID: e.layer,
	})
	if se, ok := svcErr(cerr); !ok || se.Code != "E_VALIDATION" || se.Path != "args.connection_id" {
		t.Fatalf("a connection that takes no pictures was accepted: %v", cerr)
	}

	// Above a ceiling nothing is even requested of the person.
	e.setCeiling("", 10_000, 0)

	if _, err := e.removeBackground(); code(err) != service.CodeCostCeiling {
		t.Fatalf("over the ceiling: %v", err)
	}

	if e.api.edits.Load() != 0 {
		t.Fatalf("%d pictures left without approval", e.api.edits.Load())
	}
}

// SDD-007 T-101: what the provider answers is checked before it becomes a layer, and never carries the key.
func TestSecurity_T101_ResultIsCheckedBeforeItBecomesALayer(t *testing.T) {
	t.Parallel()

	t.Run("a file that is not an image", func(t *testing.T) {
		t.Parallel()

		e := newEditEnv(t)
		e.api.setEdit("notimage")

		revision := e.describe().Revision

		done := e.approved(e.removeBackground())
		if done.State != service.JobFailed || done.Error == nil || done.Error.Code != service.CodeProvider {
			t.Fatalf("job = %+v (%+v)", done, done.Error)
		}

		if e.layerOf(e.layer).Asset != e.photo || e.describe().Revision != revision {
			t.Fatal("a file that is not an image changed the document")
		}

		if e.generatedAssets() != 0 {
			t.Error("a file that is not an image was stored")
		}
	})

	t.Run("a result on a host the connection does not list", func(t *testing.T) {
		t.Parallel()

		e := newEditEnv(t)

		var hits atomic.Int32

		other := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
		t.Cleanup(other.Close)

		e.api.mu.Lock()
		e.api.foreign = strings.Replace(other.URL, "127.0.0.1", "localhost", 1)
		e.api.mu.Unlock()
		e.api.setEdit("foreign")

		done := e.approved(e.removeBackground())
		if done.State != service.JobFailed || done.Error == nil ||
			done.Error.Code != service.CodeConnectionDestination {
			t.Fatalf("job = %+v (%+v), want E_CONNECTION_DESTINATION", done, done.Error)
		}

		if hits.Load() != 0 {
			t.Fatalf("the foreign host received %d requests", hits.Load())
		}

		if e.layerOf(e.layer).Asset != e.photo {
			t.Fatal("the layer changed")
		}
	})

	t.Run("a refusal of the provider shows no text of the provider", func(t *testing.T) {
		t.Parallel()

		e := newEditEnv(t)
		e.api.setEdit("refused")

		done := e.approved(e.removeBackground())
		if done.State != service.JobFailed || done.Error == nil || done.Error.Details["kind"] != "refused" {
			t.Fatalf("job = %+v (%+v)", done, done.Error)
		}

		if n := e.spentToday(); n != 0 {
			t.Errorf("a refused request recorded %d spent", n)
		}
	})

	t.Run("a revised prompt that repeats the key", func(t *testing.T) {
		t.Parallel()

		e := newEditEnv(t)
		e.api.setEdit("echo")

		done := e.approved(e.removeBackground())
		if done.State != service.JobSucceeded || done.Result.Generated.Items[0].RevisedPrompt == "the key is "+apiKey {
			t.Fatalf("job = %+v", done)
		}

		if strings.Contains(done.Text, apiKey) ||
			strings.Contains(done.Result.Generated.Items[0].RevisedPrompt, apiKey) {
			t.Fatal("the key is in the result")
		}
	})
}

// SDD-007 T-102: a job edits only an image its caller could edit, as that caller.
func TestSecurity_T102_JobCannotEditAnImageItsCallerCannot(t *testing.T) {
	t.Parallel()

	e := newEditEnv(t)

	args := service.ImageModelArgs{
		ConnectionID: e.imageConn, ImageID: e.image, Operation: provider.OperationRemoveBackground,
		Model: imageModel, LayerID: e.layer,
	}

	start := func(ctx context.Context) error {
		_, err := e.svc.ImageModelEdit(ctx, service.ImageModelEditRequest{ProjectID: e.prj, ImageModelArgs: args})

		return err
	}

	// A key limited to the project reaches no image.
	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:limited", Scopes: service.AllScopes, ProjectID: e.prj,
	})
	if err := start(limited); code(err) != service.CodeForbidden {
		t.Fatalf("a key limited to one project: %v", err)
	}

	// A key with no edit scope starts nothing.
	readOnly := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:reader", Scopes: []string{service.ScopeRead},
	})
	if err := start(readOnly); code(err) != service.CodeForbidden {
		t.Fatalf("a key with no edit scope: %v", err)
	}

	// In network mode the image of another user is as unknown as one that does not exist, whatever the project.
	alice := service.Caller{Actor: "user:alice", Scopes: service.AllScopes, OwnerUserID: "alice"}
	bob := service.Caller{Actor: "user:bob", Scopes: service.AllScopes, OwnerUserID: "bob"}

	proj, err := e.svc.ProjectCreate(
		service.WithCaller(context.Background(), bob),
		service.ProjectCreateRequest{Name: "bobs"},
	)
	if err != nil {
		t.Fatal(err)
	}

	made, err := e.svc.ImageCreate(
		service.WithCaller(context.Background(), alice),
		service.ImageCreateRequest{Preset: "video_thumbnail", Name: "Alices"},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, theirs := e.svc.ImageModelEdit(service.WithCaller(context.Background(), bob), service.ImageModelEditRequest{
		ProjectID:    proj.ProjectID,
		ConnectionID: e.imageConn, ImageID: made.ImageID, Operation: provider.OperationRemoveBackground,
		Model: imageModel, LayerID: e.layer,
	})

	_, none := e.svc.ImageModelEdit(service.WithCaller(context.Background(), bob), service.ImageModelEditRequest{
		ProjectID:    proj.ProjectID,
		ConnectionID: e.imageConn, ImageID: "img_0000009999", Operation: provider.OperationRemoveBackground,
		Model: imageModel, LayerID: e.layer,
	})

	st, ok1 := svcErr(theirs)
	sn, ok2 := svcErr(none)

	if !ok1 || !ok2 || st.Code != "E_NOT_FOUND" || st.Code != sn.Code {
		t.Fatalf("another user's image: %v, and an unknown one: %v", theirs, none)
	}

	if e.api.edits.Load() != 0 {
		t.Fatal("a refused job reached the provider")
	}
}
