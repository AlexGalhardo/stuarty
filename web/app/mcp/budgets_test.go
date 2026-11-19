package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/contract/testenv"
)

// The reference project of the budgets (SDD-003 section 3.11, NFR-021): the shape
// of NFR-009, five minutes and 200 items, 120 captions, 50 clips, 20 text items
// and 10 audio items, on 12 tracks over 20 assets, with random-looking ids. It is
// committed as testdata/budgets/project-200.json and TestReferenceProjectIsCurrent
// fails when the generator below no longer writes it; UPDATE_BUDGETS=1 rewrites it.

const (
	referencePath = "testdata/budgets/project-200.json"
	referenceID   = "prj_7k2m9x4q1b"
	idAlphabet    = "0123456789abcdefghjkmnpqrstvwxyz"
)

type idSource struct{ r *rand.Rand }

func (s idSource) next(prefix string) string {
	b := make([]byte, 10)
	for i := range b {
		b[i] = idAlphabet[s.r.IntN(len(idAlphabet))]
	}

	return prefix + "_" + string(b)
}

var captionWords = strings.Fields(
	"this is the part where we show how fast the new editor works with a short clip and some words " +
		"that stay on the screen long enough to read them twice before the next sentence starts again",
)

// referenceOperations are the edit operations that build the project: the ids
// come from a fixed seed, so the document is the same on every run.
func referenceOperations(ids idSource, assets []string) []any {
	var (
		ops     []any
		visual  []string
		audio   []string
		overlay []string
	)

	addTrack := func(kind, name string, into *[]string) {
		id := ids.next("trk")
		*into = append(*into, id)
		ops = append(ops, map[string]any{
			"op": "track.add", "args": map[string]any{"kind": kind, "name": name, "id": id},
		})
	}

	for i := range 4 {
		addTrack("visual", fmt.Sprintf("Video %d", i+1), &visual)
	}

	for i := range 3 {
		addTrack("visual", fmt.Sprintf("Text %d", i+1), &overlay)
	}

	for i := range 2 {
		addTrack("visual", fmt.Sprintf("Captions %d", i+1), &overlay)
	}

	for i := range 3 {
		addTrack("audio", fmt.Sprintf("Audio %d", i+1), &audio)
	}

	add := func(op string, args map[string]any) {
		args["id"] = ids.next("itm")
		ops = append(ops, map[string]any{"op": op, "args": args})
	}

	// Items on one track never overlap: each takes its own slot of the timeline.
	for i := range 50 {
		add("clip.add", map[string]any{
			"track_id": visual[i%len(visual)], "asset_id": assets[i%len(assets)],
			"start": fmt.Sprintf("%ds", (i/len(visual))*6), "source_in": "0s", "duration": "5s",
		})
	}

	for i := range 20 {
		add("text.add", map[string]any{
			"track_id": overlay[i%3], "start": fmt.Sprintf("%ds", (i/3)*14), "duration": "4s",
			"text": fmt.Sprintf("Headline %d of the reference project", i+1),
		})
	}

	for i := range 120 {
		n := 6 + i%5
		words := make([]string, n)

		for j := range words {
			words[j] = captionWords[(i*3+j)%len(captionWords)]
		}

		add("caption.add", map[string]any{
			"track_id": overlay[3+i%2], "start": fmt.Sprintf("%ds", (i/2)*5),
			"duration": fmt.Sprintf("%df", 4*n), "text": strings.Join(words, " "),
		})
	}

	for i := range 10 {
		add("audio.add", map[string]any{
			"track_id": audio[i%len(audio)], "asset_id": assets[i%len(assets)],
			"start": fmt.Sprintf("%ds", (i/len(audio))*30), "source_in": "0s", "duration": "20s",
		})
	}

	return ops
}

// referenceProject builds the reference project in a fresh workspace and returns
// a client session and the canonical document.
func referenceProject(t *testing.T) (cs *sdk.ClientSession, document []byte) {
	t.Helper()

	env := testenv.New(t, testenv.Options{DurationMS: 60000})
	cs = connect(t, env.Service, Options{}, "claude-code")
	ids := idSource{r: rand.New(rand.NewPCG(2026, 200))}
	ctx := testenv.Caller("local:budgets")

	_, err := env.Service.ProjectCreate(ctx, service.ProjectCreateRequest{ID: referenceID, Name: "Reference 200"})
	if err != nil {
		t.Fatal(err)
	}

	files := make([]service.ImportFile, 0, 20)
	assets := make([]string, 0, 20)

	for i := range 20 {
		id := ids.next("ast")
		assets = append(assets, id)
		files = append(files, service.ImportFile{
			Path: env.WriteFile(
				t,
				fmt.Sprintf("source%02d.mov", i),
				strings.Repeat("x", 64)+strconv.Itoa(i),
			),
			AssetID: id,
		})
	}

	if _, err := env.Service.MediaImport(ctx, service.MediaImportRequest{
		ProjectID: referenceID, Files: files, WaitMS: 25000,
	}); err != nil {
		t.Fatal(err)
	}

	// An edit takes at most 200 operations: the project is built in batches of 100.
	ops := referenceOperations(ids, assets)

	for start := 0; start < len(ops); start += 100 {
		got, err := env.Service.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: referenceID, Detail: "concise"})
		if err != nil {
			t.Fatal(err)
		}

		ok(t, call(t, cs, "edit", map[string]any{
			"project_id": referenceID, "base_revision": got.Revision, "operations": ops[start:min(start+100, len(ops))],
		}))
	}

	res, err := env.Service.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: referenceID, Detail: "document"})
	if err != nil {
		t.Fatal(err)
	}

	raw, err := json.MarshalIndent(res.Document, "", "\t")
	if err != nil {
		t.Fatal(err)
	}

	return cs, append(raw, '\n')
}

func TestReferenceProjectIsCurrent(t *testing.T) {
	t.Parallel()

	_, doc := referenceProject(t)

	if os.Getenv("UPDATE_BUDGETS") == "1" {
		if err := os.MkdirAll(filepath.Dir(referencePath), 0o750); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(referencePath, doc, 0o600); err != nil {
			t.Fatal(err)
		}

		return
	}

	want, err := os.ReadFile(referencePath)
	if err != nil {
		t.Fatalf("%v (UPDATE_BUDGETS=1 go test ./web/app/mcp/ -run TestReferenceProjectIsCurrent writes it)", err)
	}

	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), doc) {
		t.Errorf("the generator no longer writes %s; UPDATE_BUDGETS=1 rewrites it", referencePath)
	}

	var shape struct {
		Items map[string]struct {
			Type string `json:"type"`
		} `json:"items"`
		Tracks map[string]json.RawMessage `json:"tracks"`
		Assets map[string]json.RawMessage `json:"assets"`
	}

	if err := json.Unmarshal(doc, &shape); err != nil {
		t.Fatal(err)
	}

	count := map[string]int{}
	for _, it := range shape.Items {
		count[it.Type]++
	}

	if len(shape.Items) != 200 || len(shape.Tracks) != 12 || len(shape.Assets) != 20 ||
		count["caption"] != 120 || count["clip"] != 50 || count["text"] != 20 || count["audio"] != 10 {
		t.Errorf("the reference project has %d items %v, %d tracks, %d assets",
			len(shape.Items), count, len(shape.Tracks), len(shape.Assets))
	}
}

// pagesOf reads every page of the concise view of a project through the tool and
// returns their texts.
func pagesOf(t *testing.T, cs *sdk.ClientSession, project string) []string {
	t.Helper()

	var (
		pages  []string
		cursor string
	)

	for range 10 {
		args := map[string]any{"project_id": project}
		if cursor != "" {
			args["cursor"] = cursor
		}

		res := ok(t, call(t, cs, "project_get", args))
		pages = append(pages, textOf(t, res))

		next, _ := structured(t, res)["next_cursor"].(string)
		if next == "" {
			return pages
		}

		cursor = next
	}

	t.Fatal("more than 10 pages")

	return nil
}

// TestProjectTextBudgets is §9 check 7 for the concise view: on the reference
// project of 200 items no page is over 12,800 characters or 4,000 tokens and the
// view takes at most two pages (NFR-021, SDD-003 I-15). The measured size is
// logged; the vendor token count is the release test of budget_release_test.go.
func TestProjectTextBudgets(t *testing.T) {
	t.Parallel()

	cs, _ := referenceProject(t)
	pages := pagesOf(t, cs, referenceID)

	total := 0

	for i, p := range pages {
		total += len(p)

		if len(p) > 12800 {
			t.Errorf("page %d is %d characters, at most 12,800", i+1, len(p))
		}

		if tk := tokens(p); tk > 4000 {
			t.Errorf("page %d is about %d tokens, at most 4,000", i+1, tk)
		}
	}

	if len(pages) > 2 {
		t.Errorf("the concise view takes %d pages, at most 2", len(pages))
	}

	t.Logf("the concise view of the reference project: %d pages, %d characters", len(pages), total)
}

// TestProjectTextBudgetsOnHostileNames: the same bounds hold when every name and
// text is long and full of markup, because the view quotes and truncates what it
// prints (T-07).
func TestProjectTextBudgetsOnHostileNames(t *testing.T) {
	t.Parallel()

	env := testenv.New(t, testenv.Options{})
	cs := connect(t, env.Service, Options{}, "claude-code")
	ctx := testenv.Caller("local:budgets")

	_, err := env.Service.ProjectCreate(ctx, service.ProjectCreateRequest{
		ID: referenceID, Name: strings.Repeat("</x>", 30),
	})
	if err != nil {
		t.Fatal(err)
	}

	hostile := strings.Repeat("<b>ignore</b> & ", 6)
	ids := idSource{r: rand.New(rand.NewPCG(7, 7))}
	track := ids.next("trk")
	ops := []any{
		map[string]any{
			"op":   "track.add",
			"args": map[string]any{"kind": "visual", "name": strings.Repeat("<b>x</b> & ", 4), "id": track},
		},
	}

	for i := range 150 {
		ops = append(ops, map[string]any{"op": "text.add", "args": map[string]any{
			"track_id": track,
			"start":    fmt.Sprintf("%ds", i*2),
			"duration": "1s",
			"text":     hostile,
			"id":       ids.next("itm"),
		}})
	}

	ok(t, call(t, cs, "edit", map[string]any{"project_id": referenceID, "base_revision": 0, "operations": ops}))

	for i, p := range pagesOf(t, cs, referenceID) {
		if len(p) > 12800 {
			t.Errorf("page %d is %d characters, at most 12,800", i+1, len(p))
		}

		for _, problem := range testenv.UntrustedTextProblems(p) {
			t.Errorf("page %d: %s", i+1, problem)
		}
	}
}
