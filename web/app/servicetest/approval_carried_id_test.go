package servicetest

import (
	"context"
	"errors"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// VV-373: a timeline row that carries the id of another project, kept by a workspace that imported a forged bundle
// before the import refused it, does not make a key limited to its holder reach that other project, however the id
// is resolved: an id that names a project is that project (D-132).
func TestSecurity_T06_ATimelineCarryingAProjectIDDoesNotWidenAKey(t *testing.T) {
	e := newProjectWideEnv(t, &probe{estimate: usd(1)})
	user := "usr_0000000001"
	owner := networkPerson(user, service.AllScopes, false)

	victim, err := testproject.Create(owner, e.svc, "Victim")
	if err != nil {
		t.Fatal(err)
	}

	holder, err := testproject.CreateWithTimeline(owner, e.svc, "Holder")
	if err != nil {
		t.Fatal(err)
	}

	job, err := e.svc.JobStart(owner, service.JobStartRequest{ProjectID: victim.ProjectID, Kind: testKind})
	if err != nil {
		t.Fatal(err)
	}

	e.timelineCarrying(holder.ProjectID, victim.ProjectID)

	if _, err := e.store.Productions().ProjectOfTimeline(context.Background(), victim.ProjectID); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("the project of a timeline that carries the id of a project = %v, want ErrNotFound", err)
	}

	key := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:limited", Scopes: service.AllScopes, OwnerUserID: user, ProjectID: holder.ProjectID,
	})

	if list, err := e.svc.ApprovalList(key, service.ApprovalListRequest{ProjectID: victim.ProjectID}); err == nil {
		t.Errorf("a key limited to the holder lists the requests of the victim project: %d", list.Total)
	}

	if _, err := e.svc.ApprovalGet(key, service.ApprovalRef{ID: job.ApprovalID}); err == nil {
		t.Error("a key limited to the holder reads a request of the victim project")
	}

	if _, err := e.svc.JobGet(key, service.JobGetRequest{JobID: job.JobID}); err == nil {
		t.Error("a key limited to the holder reads a job of the victim project")
	}

	if list, err := e.svc.ApprovalList(key, service.ApprovalListRequest{}); err != nil || list.Total != 0 {
		t.Errorf("the queue of the key = %+v, %v; want it empty", list, err)
	}

	// The owner keeps reaching both.
	if _, err := e.svc.ApprovalGet(owner, service.ApprovalRef{ID: job.ApprovalID}); err != nil {
		t.Errorf("the owner lost the request: %v", err)
	}
}
