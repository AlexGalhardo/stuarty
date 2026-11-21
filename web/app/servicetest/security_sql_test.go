package servicetest

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/testproject"
)

// sqlPayloads are the classic injection strings, plus a NUL, a lookalike quote
// and a long one. File-name safe on every platform where a file name is used:
// the file test below strips what Windows refuses.
var sqlPayloads = []string{
	`'); DROP TABLE projects; --`,
	`' OR '1'='1`,
	`x'; DELETE FROM operations; --`,
	`" OR ""="`,
	`%' UNION SELECT name, sql FROM sqlite_master --`,
	`Robert'); DROP TABLE assets;--`,
	"a\x00'; DROP TABLE audit; --",
	"ʼ; DROP TABLE jobs; --",
	`\'; DROP TABLE templates; --`,
	`1; ATTACH DATABASE 'x' AS y; --`,
	strings.Repeat("' OR 1=1 --", 40),
}

// tableCounts is the row count of every table of the store.
func tableCounts(t *testing.T, e *env) map[string]int {
	t.Helper()

	ctx := context.Background()
	db := e.store.DB()

	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}

	var names []string

	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}

		names = append(names, n)
	}

	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	_ = rows.Close()

	out := map[string]int{}

	for _, n := range names {
		var c int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM "`+strings.ReplaceAll(n, `"`, `""`)+`"`).
			Scan(&c); err != nil {
			t.Fatalf("count %s: %v", n, err)
		}

		out[n] = c
	}

	return out
}

// T-23: a payload in every free-text input of the Service leaves every table in
// place and no row removed (SDD-007 section 15 check 24). The tables of the
// idempotency keys, the queue and the jobs may legitimately shrink, so the
// check is on the tables that hold the user's work.
func TestSecurity_T23_SQLInjectionPayloads(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := m.as(ui)

	sentinel := m.newProject("sentinel")
	sentinelDoc := string(m.document(sentinel))

	project, track := m.withTrack()
	base := m.head(project).Revision

	before := tableCounts(t, m.env)

	var created []string

	for i, p := range sqlPayloads {
		name := fmt.Sprintf("%d%s", i, p)
		if len(name) > 120 {
			name = name[:120]
		}

		// A project name cannot hold a payload since epic 56; its description and the title of a timeline can.
		req := testproject.Request("sql payload")
		req.Description = name + " " + testproject.Description

		if made, cerr := m.svc.ProjectCreate(ctx, req); cerr == nil {
			created = append(created, made.ProjectID)

			_, _ = m.svc.ProjectItemAdd(
				ctx,
				service.ProjectItemAddRequest{ProjectID: made.ProjectID, Kind: "timeline", Title: name},
			)
		}

		_, _ = m.svc.ProjectList(ctx, service.ProjectListRequest{State: p, Cursor: p})
		_, _ = m.svc.ProjectList(ctx, service.ProjectListRequest{Cursor: p})
		_, _ = m.svc.HistoryList(
			ctx,
			service.HistoryListRequest{ProjectID: project, Actor: p, Cursor: p},
		)
		_, _ = m.svc.JobList(ctx, service.JobListRequest{ProjectID: project, State: p, Cursor: p})
		_, _ = m.svc.MediaList(ctx, service.MediaListRequest{ProjectID: project, Cursor: p})
		_, _ = m.svc.TemplateList(ctx, service.TemplateListRequest{Cursor: p})
		_, _ = m.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: p})
		_, _ = m.svc.TemplateSave(ctx, service.TemplateSaveRequest{ProjectID: project, Name: name, Description: p})

		quoted, jerr := json.Marshal(p)
		if jerr != nil {
			t.Fatal(jerr)
		}

		ops := []command.Operation{
			op("track.add", fmt.Sprintf(`{"kind":"visual","name":%s}`, trimJSON(string(quoted), 80))),
			op("text.add", fmt.Sprintf(`{"track_id":%q,"start":"%ds","duration":"1s","text":%s}`, track, i*2, quoted)),
			op(
				"caption.add",
				fmt.Sprintf(`{"track_id":%q,"start":"%ds","duration":"1s","text":%s}`, track, i*2+30, quoted),
			),
		}

		for _, o := range ops {
			head := m.head(project).Revision

			_, _ = m.svc.Edit(
				ctx,
				service.EditRequest{ProjectID: project, BaseRevision: head, Operations: []command.Operation{o}},
			)
		}

		// The idempotency key has a strict charset: the payload is refused, never run.
		_, ierr := m.svc.Edit(ctx, service.EditRequest{
			ProjectID: project, BaseRevision: m.head(project).Revision, IdempotencyKey: p,
			Operations: []command.Operation{op("track.add", `{"kind":"audio"}`)},
		})
		if ierr == nil {
			t.Errorf("payload %q was accepted as an idempotency key", p)
		}

		// The actor label of a caller is stored with each operation.
		hostile := service.WithCaller(context.Background(), service.Caller{Actor: p, Scopes: service.AllScopes})
		_, _ = m.svc.Edit(hostile, service.EditRequest{
			ProjectID: project, BaseRevision: m.head(project).Revision,
			Operations: []command.Operation{op("track.add", `{"kind":"audio"}`)},
		})
	}

	if _, err := m.svc.MediaImport(ctx, service.MediaImportRequest{
		ProjectID: project,
		Files:     []service.ImportFile{{Path: m.write("o'neil'); DROP TABLE assets;--.mp4", clipBytes)}},
	}); err != nil {
		t.Fatalf("import of a file with SQL in its name: %v", err)
	}

	// The payloads were stored as data: the proof that they reached the database.
	stored := 0

	for _, id := range created {
		got, gerr := m.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: id})
		if gerr != nil {
			t.Fatal(gerr)
		}

		if strings.Contains(got.Description, "DROP TABLE projects") {
			stored++
		}
	}

	if stored == 0 {
		t.Error("no project kept its hostile description: the payload never reached a query")
	}

	if !strings.Contains(string(m.document(project)), "DROP TABLE") {
		t.Error("no item text kept its hostile content: the payload never reached a query")
	}

	after := tableCounts(t, m.env)

	if !slices.Equal(slices.Sorted(maps.Keys(before)), slices.Sorted(maps.Keys(after))) {
		t.Fatalf("the set of tables changed:\nbefore %v\nafter  %v", before, after)
	}

	for table, was := range before {
		if table == "idempotency_keys" || strings.HasPrefix(table, "queue") || table == "jobs" {
			continue
		}

		if after[table] < was {
			t.Errorf("table %s lost rows: %d to %d", table, was, after[table])
		}
	}

	if got := string(m.document(sentinel)); got != sentinelDoc {
		t.Error("the sentinel project changed")
	}

	if got := m.head(project).Revision; got < base {
		t.Errorf("the revision went back from %d to %d", base, got)
	}

	var integrity string
	if err := m.store.DB().
		QueryRowContext(context.Background(), `PRAGMA integrity_check`).
		Scan(&integrity); err != nil ||
		integrity != "ok" {
		t.Errorf("integrity_check = %q, %v", integrity, err)
	}
}

// trimJSON shortens a JSON string literal to n characters of content and
// keeps it a valid literal.
func trimJSON(lit string, n int) string {
	var s string
	if err := json.Unmarshal([]byte(lit), &s); err != nil {
		return `"x"`
	}

	if len(s) > n {
		s = s[:n]
	}

	b, err := json.Marshal(s)
	if err != nil {
		return `"x"`
	}

	return string(b)
}
