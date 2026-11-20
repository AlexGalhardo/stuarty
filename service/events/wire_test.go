package events_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/events"
)

// encode writes a frame the way SDD-006 section 3 says the stream does: id
// (batch.committed only), event, one data line, a blank line.
func encode(f events.Frame) string {
	var b strings.Builder

	if f.ID != "" {
		b.WriteString("id: " + f.ID + "\n")
	}

	b.WriteString("event: " + f.Event + "\ndata: " + f.Data + "\n\n")

	return b.String()
}

// Check 1 of SDD-006 section 11: the exact bytes of each event, one golden file
// per event. UPDATE_GOLDEN=1 rewrites them.
func TestGoldenBytesOfEachEvent(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 11, 14, 12, 3, 10, 204_000_000, time.UTC)
	row := service.OperationRow{
		ProjectID: "prj_7k2m9x4q1b", Revision: 42, BaseRevision: 41, Kind: service.OpEdit, Actor: "local:claude-code",
		AtMs: at.UnixMilli(), Diff: json.RawMessage(`[{"kind":"item.added","id":"itm_s6t7v8w9x0"}]`),
	}

	batch, err := events.BatchFrame(&row, "+ item itm_s6t7v8w9x0")
	if err != nil {
		t.Fatal(err)
	}

	moved := time.Date(2026, 11, 14, 12, 20, 31, 870_000_000, time.UTC)
	job := service.Event{
		ProjectID: "prj_7k2m9x4q1b", JobID: "job_4f8h2k9m3q", Kind: "export", State: "running", Progress: 0.31,
		Message: "Encoding frame 280 of 900", At: moved,
	}
	asset := service.Event{
		ProjectID: "prj_7k2m9x4q1b", AssetID: "ast_3c5v8n2w6d", State: "ingesting", Progress: 0.42,
		Message: "Normalizing video", At: moved,
	}

	cases := map[string]events.Frame{
		"batch.committed": batch,
		"project.updated": events.ProjectFrame(
			&service.Event{ProjectID: "prj_7k2m9x4q1b", State: "trashed", At: moved},
		),
		"nested.updated": events.NestedFrame(&service.Event{
			ProjectID: "prj_7k2m9x4q1b", TimelineID: "prj_a1b2c3d4e5", Revision: 12, At: moved,
		}),
		"asset.updated":   events.AssetFrame(&asset),
		"job.updated":     events.JobFrame(&job),
		"resync.required": events.ResyncFrame("prj_7k2m9x4q1b", 1500, events.ReasonGapTooLarge),
	}

	for name, f := range cases {
		path := filepath.Join("testdata", name+".golden")
		got := encode(f)

		if os.Getenv("UPDATE_GOLDEN") != "" {
			if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
				t.Fatal(err)
			}
		}

		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		if got != strings.ReplaceAll(string(want), "\r\n", "\n") {
			t.Errorf("%s bytes:\n got %q\nwant %q", name, got, want)
		}

		if (f.ID != "") != (name == "batch.committed") {
			t.Errorf("%s: id %q is set only for batch.committed", name, f.ID)
		}

		if strings.Count(got, "data:") != 1 || strings.Contains(f.Data, "\n") {
			t.Errorf("%s: data is not one line", name)
		}
	}
}

var exampleBlock = regexp.MustCompile("(?s)```text\n(.*?)```")

// Check 2: every example block of the SDD is extracted and validated against the
// JSON Schema of its event, so the document cannot drift from the contract.
func TestSDDExamplesMatchTheSchemas(t *testing.T) {
	t.Parallel()

	md, err := os.ReadFile(filepath.Join("..", "..", "docs", "sdds", "006-real-time-protocol.md"))
	if err != nil {
		t.Fatal(err)
	}

	schemas := map[string]*jsonschema.Schema{}
	c := jsonschema.NewCompiler()

	for _, name := range []string{
		events.BatchCommitted, events.ProjectUpdated, events.NestedUpdated, events.AssetUpdated, events.JobUpdated,
		events.ResyncRequired,
	} {
		f, err := os.Open(filepath.Join("testdata", "schema", name+".json"))
		if err != nil {
			t.Fatal(err)
		}

		doc, err := jsonschema.UnmarshalJSON(f)
		_ = f.Close()

		if err != nil {
			t.Fatal(err)
		}

		if err := c.AddResource("mem:///"+name+".json", doc); err != nil {
			t.Fatal(err)
		}

		if schemas[name], err = c.Compile("mem:///" + name + ".json"); err != nil {
			t.Fatal(err)
		}
	}

	seen := map[string]int{}
	text := strings.ReplaceAll(string(md), "\r\n", "\n")

	for _, m := range exampleBlock.FindAllStringSubmatch(text, -1) {
		var event, data, id string

		for line := range strings.SplitSeq(m[1], "\n") {
			switch {
			case strings.HasPrefix(line, "event: "):
				event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: {"):
				data = strings.TrimPrefix(line, "data: ")
			case strings.HasPrefix(line, "id: "):
				id = strings.TrimPrefix(line, "id: ")
			}
		}

		sch, ok := schemas[event]
		if !ok {
			continue // the preamble, the heartbeat and the layout block carry no event payload
		}

		seen[event]++

		inst, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(data)))
		if err != nil {
			t.Errorf("%s example is not JSON: %v\n%s", event, err, data)

			continue
		}

		if err := sch.Validate(inst); err != nil {
			t.Errorf("%s example does not match its schema: %v\n%s", event, err, data)
		}

		if (id != "") != (event == events.BatchCommitted) {
			t.Errorf("%s example has id %q", event, id)
		}
	}

	for name := range schemas {
		if seen[name] == 0 {
			t.Errorf("the SDD has no example of %s", name)
		}
	}
}
