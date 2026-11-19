package servicetest

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// The library over the real store (SDD-018): the files are the truth, the index can be thrown away, an imported
// skill is not offered to an agent until a person reviews it.

func libPerson(_ *mediaEnv) context.Context {
	return service.WithCaller(
		context.Background(),
		service.Caller{Actor: "local:ui", Scopes: service.AllScopes, Browser: true},
	)
}

func libAgent(_ *mediaEnv) context.Context {
	return service.WithCaller(
		context.Background(),
		service.Caller{Actor: "local:claude-code-k3m9x2", Scopes: service.AllScopes},
	)
}

func libCode(err error) string {
	if se, ok := err.(*service.Error); ok { //nolint:errorlint // the service returns its own type
		return se.Code
	}

	return ""
}

const hookSkillMD = "---\nname: hook-writer\ndescription: Write the opening of a short video.\nlicense: MIT\n---\n# Hooks\n\nSee [patterns](references/patterns.md).\n"

// writeSkillFolder writes a skill folder under dir and returns its path.
func writeSkillFolder(t *testing.T, dir string, withScript bool) string {
	t.Helper()

	root := filepath.Join(dir, "hook-writer")
	for p, text := range map[string]string{"SKILL.md": hookSkillMD, "references/patterns.md": "Open with a question.\n"} {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(full, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if withScript {
		if err := os.MkdirAll(filepath.Join(root, "scripts"), 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(
			filepath.Join(root, "scripts", "run.sh"),
			[]byte("#!/bin/sh\necho hi\n"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
	}

	return root
}

func seedLibrary(t *testing.T, m *mediaEnv) {
	t.Helper()

	ctx := libPerson(m)

	if _, err := m.svc.MemoryWrite(ctx, service.MemoryWriteRequest{
		Name: "viewers-skip-intros", Description: "Viewers skip long intros", Tags: []string{"retention"},
		Body: "Cut the intro of a video to two seconds.\n",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := m.svc.PromptWrite(ctx, service.PromptWriteRequest{
		Name: "video-intro", Description: "An intro for a video", Body: "Write an intro about {{topic}}.\n",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := m.svc.SkillWrite(ctx, service.SkillWriteRequest{
		Name:        "hook-writer",
		Description: "Write the opening of a short video.",
		Body:        "# Hooks\n\nStart with the result.\n",
	}); err != nil {
		t.Fatal(err)
	}
}

func hitNames(res *service.LibrarySearchResult) []string {
	var out []string
	for i := range res.Hits {
		out = append(out, res.Hits[i].Kind+"/"+res.Hits[i].Name)
	}

	return out
}

// Acceptance criterion 1: a note, a skill and a prompt written through the service exist as Markdown files in the
// workspace and are found by LibrarySearch.
func TestLibraryWritesFilesInTheWorkspaceAndFindsThem(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	seedLibrary(t, m)

	for _, rel := range []string{"memory/viewers-skip-intros.md", "prompts/video-intro.md", "skills/hook-writer/SKILL.md"} {
		raw, err := os.ReadFile(filepath.Join(m.ws, "library", filepath.FromSlash(rel)))
		if err != nil || !strings.HasPrefix(string(raw), "---\nname: ") {
			t.Fatalf("%s: %v\n%s", rel, err, raw)
		}
	}

	for query, want := range map[string]string{
		"intro": "memory/viewers-skip-intros", "opening": "skill/hook-writer", "topic": "prompt/video-intro",
	} {
		res, err := m.svc.LibrarySearch(libAgent(m), service.LibrarySearchRequest{Query: query, AgentView: true})
		if err != nil {
			t.Fatal(err)
		}

		if got := strings.Join(hitNames(res), ","); !strings.Contains(got, want) {
			t.Errorf("%q: found %s, want %s", query, got, want)
		}
	}

	// the same note, written by an agent, is pending and not found by an agent
	if _, err := m.svc.MemoryWrite(libAgent(m), service.MemoryWriteRequest{
		Name: "agent-fact", Description: "The agent learned about zebras", Body: "x",
	}); err != nil {
		t.Fatal(err)
	}

	res, err := m.svc.LibrarySearch(libAgent(m), service.LibrarySearchRequest{Query: "zebras", AgentView: true})
	if err != nil || len(res.Hits) != 0 {
		t.Fatalf("a pending note is not offered to an agent: %v %v", err, hitNames(res))
	}

	res, err = m.svc.LibrarySearch(libPerson(m), service.LibrarySearchRequest{Query: "zebras", IncludeUnreviewed: true})
	if err != nil || len(res.Hits) != 1 || res.Hits[0].Kept {
		t.Fatalf("the person sees it, marked pending: %v %+v", err, res)
	}
}

// Acceptance criterion 2: deleting the index and running LibraryReindex restores every result.
func TestLibraryReindexRestoresEveryResultAfterTheIndexIsDeleted(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	seedLibrary(t, m)

	ctx := libPerson(m)
	queries := []string{"intro", "opening", "topic", "result", "retention"}
	before := map[string]string{}

	for _, q := range queries {
		res, err := m.svc.LibrarySearch(ctx, service.LibrarySearchRequest{Query: q})
		if err != nil {
			t.Fatal(err)
		}

		before[q] = strings.Join(hitNames(res), ",")
		if before[q] == "" {
			t.Fatalf("%q found nothing before", q)
		}
	}

	for _, stmt := range []string{"DELETE FROM library_entries", "DELETE FROM library_fts"} {
		if _, err := m.store.DB().ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}

	res, err := m.svc.LibraryReindex(ctx, service.LibraryReindexRequest{})
	if err != nil || res.Memory != 1 || res.Skills != 1 || res.Prompts != 1 || res.Invalid != 0 {
		t.Fatalf("%v %+v", err, res)
	}

	for _, q := range queries {
		got, err := m.svc.LibrarySearch(ctx, service.LibrarySearchRequest{Query: q})
		if err != nil {
			t.Fatal(err)
		}

		if s := strings.Join(hitNames(got), ","); s != before[q] {
			t.Errorf("%q: %s after the reindex, %s before", q, s, before[q])
		}
	}
}

// A file edited outside the product is picked up at the next call (RF-391).
func TestLibraryPicksUpAFileEditedOutside(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	seedLibrary(t, m)

	path := filepath.Join(m.ws, "library", "memory", "viewers-skip-intros.md")
	if err := os.WriteFile(
		path,
		[]byte("---\nname: viewers-skip-intros\ndescription: Edited by hand with a quokka\nkept: true\n---\nx\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	res, err := m.svc.LibrarySearch(libPerson(m), service.LibrarySearchRequest{Query: "quokka"})
	if err != nil || len(res.Hits) != 1 {
		t.Fatalf("%v %+v", err, res)
	}
}

// Acceptance criterion 3: an imported skill is unreviewed, flagged, and absent from the default agent listing
// until a person reviews it.
func TestLibraryImportedSkillIsUnreviewedUntilAPersonReviewsIt(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	folder := writeSkillFolder(t, m.files, true)

	res, err := m.svc.SkillImport(libAgent(m), service.SkillImportRequest{Path: folder, Source: "a friend"})
	if err != nil {
		t.Fatal(err)
	}

	if res.Entry.Reviewed || res.Entry.Origin != "imported" || !res.Entry.HasScripts || res.Entry.Warning == "" ||
		res.Files != 3 {
		t.Fatalf("%+v", res.Entry)
	}

	list, err := m.svc.SkillList(libAgent(m), service.SkillListRequest{AgentView: true})
	if err != nil || len(list.Entries) != 0 {
		t.Fatalf("absent from the default agent listing: %v %+v", err, list)
	}

	if _, err := m.svc.SkillGet(
		libAgent(m),
		service.LibraryRef{Name: "hook-writer", AgentView: true},
	); libCode(
		err,
	) != service.CodeForbidden {
		t.Fatalf("an agent surface may not read it: %v", err)
	}

	asked, err := m.svc.SkillList(libAgent(m), service.SkillListRequest{IncludeUnreviewed: true})
	if err != nil || len(asked.Entries) != 1 || asked.Entries[0].Reviewed {
		t.Fatalf("asked for, it is listed unreviewed: %v %+v", err, asked)
	}

	if _, err := m.svc.SkillMarkReviewed(
		libAgent(m),
		service.LibraryRef{Name: "hook-writer"},
	); libCode(
		err,
	) != service.CodeForbidden {
		t.Fatalf("an agent cannot review: %v", err)
	}

	reviewed, err := m.svc.SkillMarkReviewed(libPerson(m), service.LibraryRef{Name: "hook-writer"})
	if err != nil || !reviewed.Entry.Reviewed {
		t.Fatalf("%v %+v", err, reviewed)
	}

	list, err = m.svc.SkillList(libAgent(m), service.SkillListRequest{AgentView: true})
	if err != nil || len(list.Entries) != 1 {
		t.Fatalf("listed once reviewed: %v %+v", err, list)
	}

	// an edit made outside takes the review away
	if err := os.WriteFile(
		filepath.Join(m.ws, "library", "skills", "hook-writer", "references", "patterns.md"),
		[]byte("changed\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	list, err = m.svc.SkillList(libAgent(m), service.SkillListRequest{AgentView: true})
	if err != nil || len(list.Entries) != 0 {
		t.Fatalf("an edited skill is unreviewed again: %v %+v", err, list)
	}
}

func TestLibraryImportFromAnArchiveAndExportItAgain(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})

	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)
	for name, text := range map[string]string{
		"hook-writer/SKILL.md": hookSkillMD, "hook-writer/references/patterns.md": "Open with a question.\n",
	} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}

		_, _ = w.Write([]byte(text))
	}

	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(m.files, "pack.zip")
	if err := os.WriteFile(archive, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := m.svc.SkillImport(libPerson(m), service.SkillImportRequest{Path: archive}); err != nil {
		t.Fatal(err)
	}

	if _, err := m.svc.SkillMarkReviewed(libPerson(m), service.LibraryRef{Name: "hook-writer"}); err != nil {
		t.Fatal(err)
	}

	exp, err := m.svc.SkillExport(libPerson(m), service.SkillExportRequest{})
	if err != nil || len(exp.Names) != 1 || exp.Path != filepath.Join(m.ws, "exports", "library") {
		t.Fatalf("%v %+v", err, exp)
	}

	// the exported folder is in the workspace of this server: it can be imported again
	if _, err := m.svc.SkillImport(libPerson(m), service.SkillImportRequest{
		Path: filepath.Join(exp.Path, "hook-writer"), Replace: true,
	}); err != nil {
		t.Fatalf("an exported folder imports again: %v", err)
	}

	lint, err := m.svc.SkillLint(libPerson(m), service.LibraryRef{Name: "hook-writer"})
	if err != nil || lint.Errors != 0 {
		t.Fatalf("%v %+v", err, lint)
	}
}

func TestLibraryImportPathRules(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	ctx := libPerson(m)

	if err := os.MkdirAll(filepath.Join(m.ws, "library"), 0o700); err != nil {
		t.Fatal(err)
	}

	cases := map[string]string{
		"a URL":           "https://example.com/skill.zip",
		"the workspace":   filepath.Join(m.ws, "library"),
		"a relative path": "skill.zip",
	}

	for name, p := range cases {
		_, err := m.svc.SkillImport(ctx, service.SkillImportRequest{Path: p})
		if c := libCode(err); c != "E_PATH_NOT_ALLOWED" {
			t.Errorf("%s: %v, want E_PATH_NOT_ALLOWED", name, err)
		}
	}

	if _, err := m.svc.SkillImport(
		ctx,
		service.SkillImportRequest{Path: filepath.Join(m.files, "missing.zip")},
	); libCode(
		err,
	) != "E_FILE_NOT_FOUND" {
		t.Errorf("a missing file: %v", err)
	}

	if err := os.WriteFile(filepath.Join(m.files, "text.zip"), []byte("not an archive"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := m.svc.SkillImport(
		ctx,
		service.SkillImportRequest{Path: filepath.Join(m.files, "text.zip")},
	); libCode(
		err,
	) != "E_VALIDATION" {
		t.Errorf("a file that is not an archive: %v", err)
	}
}

func TestLibraryScopesAndProjectKeys(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	seedLibrary(t, m)

	reader := service.WithCaller(
		context.Background(),
		service.Caller{Actor: "key:key_0000000001", Scopes: []string{service.ScopeRead}},
	)

	if _, err := m.svc.MemoryList(reader, service.MemoryListRequest{IncludeUnreviewed: true}); err != nil {
		t.Fatal(err)
	}

	for name, call := range map[string]func() error{
		"MemoryWrite": func() error {
			_, err := m.svc.MemoryWrite(reader, service.MemoryWriteRequest{Description: "d"})
			return err
		},
		"PromptWrite": func() error {
			_, err := m.svc.PromptWrite(reader, service.PromptWriteRequest{Name: "a", Description: "d", Body: "x"})
			return err
		},
		"SkillImport": func() error {
			_, err := m.svc.SkillImport(reader, service.SkillImportRequest{Path: m.files})
			return err
		},
		"LibraryReindex": func() error {
			_, err := m.svc.LibraryReindex(reader, service.LibraryReindexRequest{})
			return err
		},
	} {
		if c := libCode(call()); c != service.CodeForbidden {
			t.Errorf("%s with the read scope: %q, want E_FORBIDDEN", name, c)
		}
	}

	limited := service.WithCaller(context.Background(), service.Caller{
		Actor: "key:key_0000000002", Scopes: service.AllScopes, ProjectID: "prj_0000000001",
	})

	if _, err := m.svc.MemoryWrite(
		limited,
		service.MemoryWriteRequest{Name: "w", Description: "d"},
	); libCode(
		err,
	) != service.CodeForbidden {
		t.Errorf("a key limited to a project writes no workspace note: %v", err)
	}

	if _, err := m.svc.MemoryWrite(
		limited,
		service.MemoryWriteRequest{Name: "p", Description: "d", Scope: "prj_0000000001"},
	); err != nil {
		t.Errorf("but one of its own project: %v", err)
	}

	if _, err := m.svc.SkillExport(limited, service.SkillExportRequest{}); libCode(err) != service.CodeForbidden {
		t.Errorf("%v", err)
	}
}

func TestLibraryAuditsWritesWithTheActor(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})
	seedLibrary(t, m)

	if _, err := m.svc.MemoryRemove(libPerson(m), service.LibraryRef{Name: "viewers-skip-intros"}); err != nil {
		t.Fatal(err)
	}

	res, err := m.svc.AuditList(libPerson(m), service.AuditListRequest{})
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]bool{}
	for _, row := range res.Entries {
		got[row.Action] = row.Actor == "local:ui"
	}

	for _, action := range []string{"library.memory.write", "library.prompt.write", "library.skill.write", "library.memory.remove"} {
		if !got[action] {
			t.Errorf("no audit row %s by the person: %v", action, got)
		}
	}
}

// A person edits the text exactly as it is; every other caller gets it cleaned and escaped.
func TestLibraryText(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})

	body := "Use <b>bold</b> & more" + zw + " text\n"
	if _, err := m.svc.PromptWrite(
		libPerson(m),
		service.PromptWriteRequest{Name: "raw", Description: "d", Body: body},
	); err != nil {
		t.Fatal(err)
	}

	own, err := m.svc.PromptGet(libPerson(m), service.LibraryRef{Name: "raw"})
	if err != nil || own.Body != body {
		t.Fatalf("a person gets the text as the file has it: %v %q", err, own.Body)
	}

	other, err := m.svc.PromptGet(libAgent(m), service.LibraryRef{Name: "raw"})
	if err != nil || strings.Contains(other.Body, "<") || strings.Contains(other.Body, ""+zw+"") ||
		other.UntrustedNotice == "" {
		t.Fatalf("another caller gets it as data: %v %q", err, other.Body)
	}

	if !strings.Contains(other.Text, `<untrusted_media_text source="library_text"`) ||
		strings.Contains(other.Text, "<b>") {
		t.Fatalf("the text part is a block: %s", other.Text)
	}
}

func TestLibraryNeedsAWorkspace(t *testing.T) {
	e := newEnv(t)

	_, err := e.svc.MemoryList(e.as(ui), service.MemoryListRequest{})
	if libCode(err) != "E_NOT_FOUND" {
		t.Fatalf("%v", err)
	}
}

// zw is a zero width space, which a person cannot see and a model reads.
var zw = string(rune(0x200b))
