package servicetest

import (
	"context"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// carried is the state a forged bundle could leave before its import refused it: the project of one user holds, as
// its first timeline, a timeline row whose id is the id of a project of another user who has no timeline yet. That
// other project keeps a job, a file and a key under that same id.
type carried struct {
	e                   *env
	ana, bob, anaKey    context.Context
	holder, victim, job string
}

const (
	carriedHash = "b3_carried0000000000000000000000000000000000000000000000000000000"
	carriedKey  = "key_carried001"
)

func newCarried(t *testing.T) *carried {
	t.Helper()

	e := newProjectWideEnv(t, &probe{estimate: usd(1)})
	ctx := context.Background()

	const anaUser, bobUser = "usr_0000000001", "usr_0000000002"

	ana, bob := networkPerson(anaUser, service.AllScopes, false), networkPerson(bobUser, service.AllScopes, false)

	victim, err := testproject.Create(bob, e.svc, "Bob project")
	if err != nil {
		t.Fatal(err)
	}

	holder, err := testproject.CreateWithTimeline(ana, e.svc, "Ana project")
	if err != nil {
		t.Fatal(err)
	}

	job, err := e.svc.JobStart(bob, service.JobStartRequest{ProjectID: victim.ProjectID, Kind: testKind})
	if err != nil {
		t.Fatal(err)
	}

	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO api_keys (id, token_id, user_id, project_id, name, created_ms)
 VALUES (?, 'tok_carried001', ?, ?, 'bob key', 1)`, []any{carriedKey, bobUser, victim.ProjectID}},
		{`INSERT INTO assets (id, kind, size_bytes, original_name, ingest_state, created_ms, updated_ms)
 VALUES (?, 'video', 1, 'bob.mp4', 'ready', 1, 1)`, []any{carriedHash}},
		{`INSERT INTO project_assets (project_id, asset_id, asset_hash, state, name, imported_by, imported_ms)
 VALUES (?, 'ast_carried001', ?, 'active', 'bob.mp4', 'user:bob', 1)`, []any{victim.ProjectID, carriedHash}},
		{`INSERT INTO timelines (id, name, state, revision, schema_version, document, doc_sha256, width, height, fps_num,
 fps_den, duration_frames, track_count, item_count, asset_count, resync_floor, created_ms, updated_ms, created_by,
 owner_user_id)
 SELECT ?, name, state, revision, schema_version, document, doc_sha256, width, height, fps_num, fps_den,
 duration_frames, track_count, item_count, asset_count, resync_floor, created_ms, updated_ms, created_by, owner_user_id
 FROM timelines WHERE id = ?`, []any{victim.ProjectID, holder.ProjectID}},
		{`INSERT INTO project_items (id, project_id, kind, title, position, state, ref, created_ms, updated_ms, created_by)
 VALUES ('pit_carried002', ?, 'timeline', 'carried', -1, 'active', ?, 1, 1, 'u')`,
			[]any{holder.ProjectID, victim.ProjectID}},
	} {
		if _, err := e.store.DB().ExecContext(ctx, q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}

	return &carried{
		e: e, ana: ana, bob: bob,
		anaKey: service.WithCaller(ctx, service.Caller{
			Actor: "key:ana", Scopes: service.AllScopes, OwnerUserID: anaUser, ProjectID: holder.ProjectID,
		}),
		holder: holder.ProjectID, victim: victim.ProjectID, job: job.JobID,
	}
}

func (c *carried) count(t *testing.T, query string, args ...any) int {
	t.Helper()

	var n int
	if err := c.e.store.DB().QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}

	return n
}

// victimIntact fails when what the other user keeps under the id was changed by what the holder did.
func (c *carried) victimIntact(t *testing.T) {
	t.Helper()

	if n := c.count(t, `SELECT count(*) FROM jobs WHERE id = ? AND state = 'waiting_approval'`, c.job); n != 1 {
		t.Error("the job of the other user was cancelled or deleted")
	}

	if n := c.count(t, `SELECT count(*) FROM project_assets WHERE project_id = ?`, c.victim); n != 1 {
		t.Error("the file of the other user was deleted")
	}

	if n := c.count(t, `SELECT count(*) FROM api_keys WHERE id = ?`, carriedKey); n != 1 {
		t.Error("the key of the other user was deleted")
	}
}

// VV-373: nothing the holder does through its own project reads, copies, stops or deletes what the other project
// keeps under the id its timeline row carries, for a key limited to the holder and for the holder's own session.
func TestSecurity_T06_ACarriedTimelineReachesNothingOfTheOtherProject(t *testing.T) {
	t.Run("the jobs of the project", func(t *testing.T) {
		c := newCarried(t)

		for name, who := range map[string]context.Context{"key": c.anaKey, "session": c.ana} {
			list, err := c.e.svc.JobList(who, service.JobListRequest{ProjectID: c.holder})
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}

			for i := range list.Jobs {
				if list.Jobs[i].JobID == c.job {
					t.Errorf("the %s of the holder lists the job of the other user", name)
				}
			}
		}
	})

	t.Run("the id of the other project, named directly", func(t *testing.T) {
		c := newCarried(t)

		for name, who := range map[string]context.Context{"key": c.anaKey, "session": c.ana} {
			if _, err := c.e.svc.JobGet(who, service.JobGetRequest{JobID: c.job}); err == nil {
				t.Errorf("the %s of the holder reads the job of the other user", name)
			}

			if list, err := c.e.svc.JobList(who, service.JobListRequest{ProjectID: c.victim}); err == nil {
				t.Errorf("the %s of the holder lists the jobs of the other project: %d", name, len(list.Jobs))
			}

			if _, err := c.e.svc.ProjectGet(who, service.ProjectGetRequest{ProjectID: c.victim}); err == nil {
				t.Errorf("the %s of the holder reads the other project", name)
			}
		}

		// The owner keeps their project: its jobs, a new job, and the project itself.
		if _, err := c.e.svc.JobGet(c.bob, service.JobGetRequest{JobID: c.job}); err != nil {
			t.Errorf("the owner lost their job: %v", err)
		}

		if _, err := c.e.svc.ProjectGet(c.bob, service.ProjectGetRequest{ProjectID: c.victim}); err != nil {
			t.Errorf("the owner lost their project: %v", err)
		}

		if _, err := c.e.svc.JobStart(c.bob, service.JobStartRequest{
			ProjectID: c.victim, Kind: testKind,
		}); err != nil {
			t.Errorf("the owner cannot start a job on their project: %v", err)
		}
	})

	t.Run("a copy of the project", func(t *testing.T) {
		c := newCarried(t)

		copied, err := c.e.svc.ProjectDuplicate(c.ana, service.ProjectDuplicateRequest{ProjectID: c.holder})
		if err != nil {
			t.Fatal(err)
		}

		if n := c.count(t, `SELECT count(*) FROM project_assets WHERE asset_hash = ? AND project_id <> ?`,
			carriedHash, c.victim); n != 0 {
			t.Errorf("the copy %s holds %d file(s) of the other user", copied.ProjectID, n)
		}
	})

	t.Run("the project in the trash", func(t *testing.T) {
		c := newCarried(t)

		if _, err := c.e.svc.ProjectTrash(c.ana, service.ProjectTrashRequest{ProjectID: c.holder}); err != nil {
			t.Fatal(err)
		}

		c.victimIntact(t)

		if _, err := c.e.svc.ProjectRestore(c.ana, service.ProjectTrashRequest{ProjectID: c.holder}); err != nil {
			t.Fatal(err)
		}

		c.victimIntact(t)
	})

	t.Run("emptying the trash", func(t *testing.T) {
		c := newCarried(t)

		if _, err := c.e.svc.ProjectTrash(c.ana, service.ProjectTrashRequest{ProjectID: c.holder}); err != nil {
			t.Fatal(err)
		}

		person := networkPerson("usr_0000000001", service.AllScopes, true)
		if _, err := c.e.svc.TrashEmpty(person, service.TrashEmptyRequest{ProjectID: c.holder}); err != nil {
			t.Fatal(err)
		}

		c.victimIntact(t)

		if n := c.count(t, `SELECT count(*) FROM timelines WHERE id = ?`, c.victim); n != 0 {
			t.Error("the row the holder carried outlived its project")
		}
	})

	t.Run("the listing of a limited key", func(t *testing.T) {
		c := newCarried(t)

		heads, _, err := c.e.store.Projects().List(context.Background(), service.ProjectQuery{ProjectID: c.holder})
		if err != nil {
			t.Fatal(err)
		}

		if slices.ContainsFunc(heads, func(h service.ProjectHead) bool { return h.ID == c.victim }) {
			t.Error("the listing limited to the holder names the id of the other project")
		}
	})

	t.Run("the other project still gets its first timeline", func(t *testing.T) {
		c := newCarried(t)

		added, err := c.e.svc.ProjectItemAdd(c.bob, service.ProjectItemAddRequest{
			ProjectID: c.victim, Kind: "timeline", Title: "Bob timeline",
		})
		if err != nil {
			t.Fatalf("the other user cannot add a timeline to their own project: %v", err)
		}

		tls, err := c.e.store.Productions().ActiveTimelines(context.Background(), c.victim)
		if err != nil || len(tls) != 1 {
			t.Fatalf("the timelines of the other project = %+v, %v (%+v)", tls, err, added)
		}

		if tls[0].TimelineID == c.victim {
			t.Errorf("the timeline of the other project took the id the holder's row carries")
		}

		if added.Item.TimelineID != tls[0].TimelineID {
			t.Errorf("the answer names the timeline %q, stored as %q", added.Item.TimelineID, tls[0].TimelineID)
		}

		if owner, err := c.e.store.Productions().ProjectOfTimeline(
			context.Background(), tls[0].TimelineID,
		); err != nil || owner != c.victim {
			t.Errorf("the project of the new timeline = %q, %v", owner, err)
		}

		if _, err := c.e.svc.ProjectGet(c.bob, service.ProjectGetRequest{
			ProjectID: c.victim, TimelineID: tls[0].TimelineID,
		}); err != nil {
			t.Errorf("the other user cannot read their new timeline: %v", err)
		}

		c.victimIntact(t)
	})

	t.Run("the first timeline of the holder", func(t *testing.T) {
		c := newCarried(t)

		tls, err := c.e.store.Productions().ActiveTimelines(context.Background(), c.holder)
		if err != nil {
			t.Fatal(err)
		}

		if len(tls) != 1 || tls[0].TimelineID != c.holder {
			t.Errorf("the timelines of the holder = %+v, want only its own", tls)
		}

		head, err := c.e.store.Productions().Get(context.Background(), c.holder)
		if err != nil {
			t.Fatal(err)
		}

		if head.Timeline.ID != c.holder || head.Timelines != 1 {
			t.Errorf("the head of the holder shows the timeline %s of %d", head.Timeline.ID, head.Timelines)
		}
	})
}

// A project whose first timeline carries the id of the project is the usual project, and of a workspace made before
// timelines had ids of their own: it keeps every timeline.
func TestCarriedTimeline_TheUsualProjectKeepsItsTimelines(t *testing.T) {
	e := newProjectWideEnv(t, &probe{estimate: usd(1)})
	ctx := e.as(ui)

	p, err := testproject.CreateWithTimeline(ctx, e.svc, "Usual")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := e.svc.ProjectItemAdd(ctx, service.ProjectItemAddRequest{
		ProjectID: p.ProjectID, Kind: "timeline", Title: "Second",
	}); err != nil {
		t.Fatal(err)
	}

	tls, err := e.store.Productions().ActiveTimelines(context.Background(), p.ProjectID)
	if err != nil {
		t.Fatal(err)
	}

	if len(tls) != 2 || tls[0].TimelineID != p.ProjectID {
		t.Fatalf("the timelines = %+v, want the first with the id of the project and a second", tls)
	}

	for i := range tls {
		owner, err := e.store.Productions().ProjectOfTimeline(context.Background(), tls[i].TimelineID)
		if err != nil || owner != p.ProjectID {
			t.Errorf("the project of %s = %q, %v", tls[i].TimelineID, owner, err)
		}
	}
}
