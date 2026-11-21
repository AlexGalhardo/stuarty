package servicetest

import (
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
	"github.com/AlexGalhardo/stuarty/service/publish/destinations"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// What belongs to the project, and not to a timeline document, works in a project that has no timeline yet: the
// audit and the calendar see it instead of answering that it does not exist.
func TestAuditAndPublicationsOfAProjectWithNoTimeline(t *testing.T) {
	c := newCalendar(t)
	ctx := c.as(token)

	made, err := testproject.Create(ctx, c.svc, "No timeline")
	if err != nil {
		t.Fatal(err)
	}

	audit, err := c.svc.AuditList(ctx, service.AuditListRequest{ProjectID: made.ProjectID})
	if err != nil {
		t.Fatalf("AuditList: %v", err)
	}

	if len(audit.Entries) == 0 || audit.Entries[0].Action != "project.create" {
		t.Errorf("the audit of the project: %+v", audit.Entries)
	}

	c.project = made.ProjectID
	conn := c.mustBlogConnection(
		destinations.KindFiles,
		"http://localhost",
		filesSecret(t, map[string]string{"folder": t.TempDir(), "template": "hugo"}),
	)

	p, err := c.svc.PublicationCreate(ctx, service.PublicationCreateRequest{
		ProjectID: made.ProjectID, Title: "A post with no video", Post: c.blogPost("A post with no video"),
		Targets: []pub.TargetInput{{
			ConnectionID: conn, At: c.at(time.Hour), Zone: "UTC", Fields: pub.Fields{Visibility: "public"},
		}},
	})
	if err != nil {
		t.Fatalf("PublicationCreate: %v", err)
	}

	got, err := c.svc.PublicationGet(ctx, service.PublicationGetRequest{ID: p.ID})
	if err != nil || got.ProjectID != made.ProjectID {
		t.Fatalf("PublicationGet: %+v, %v", got, err)
	}
}

// Asking for the document of a project with no timeline says what to do, not which timeline to choose among none.
func TestProjectGetDocumentOfAProjectWithNoTimeline(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	made, err := testproject.Create(e.as(ui), e.svc, "No timeline")
	if err != nil {
		t.Fatal(err)
	}

	_, err = e.svc.ProjectGet(e.as(ui), service.ProjectGetRequest{ProjectID: made.ProjectID, Detail: "document"})
	if code(err) != command.CodeInvalidState || !strings.Contains(serviceError(t, err).Hint, "timeline") {
		t.Fatalf("err = %v, want E_INVALID_STATE with the hint to add a timeline", err)
	}
}
