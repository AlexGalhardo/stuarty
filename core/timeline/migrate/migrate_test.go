package migrate_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/core/timeline/migrate"
)

const versionsDir = "../testdata/versions"

func TestFixturesMigrateToCurrentAndValidate(t *testing.T) {
	for v := 1; v <= migrate.Current; v++ {
		dir := filepath.Join(versionsDir, "v"+strconv.Itoa(v))
		fixtures, err := filepath.Glob(filepath.Join(dir, "*.json"))
		if err != nil || len(fixtures) == 0 {
			t.Fatalf("%s must exist and hold fixtures: %v", dir, err)
		}
		for _, f := range fixtures {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := timeline.DecodeRaw(data)
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			migrated, err := timeline.Migrate(raw)
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			doc, err := timeline.FromRaw(migrated)
			if err != nil {
				t.Fatalf("%s: %v", f, err)
			}
			if doc.SchemaVersion != migrate.Current {
				t.Errorf("%s: schema_version %d after migration", f, doc.SchemaVersion)
			}
			if vs := timeline.Validate(doc); len(vs) != 0 {
				t.Errorf("%s: %v", f, vs)
			}
		}
	}
}

func TestCurrentVersionsAgree(t *testing.T) {
	if timeline.CurrentSchemaVersion != migrate.Current {
		t.Fatalf("timeline %d, migrate %d", timeline.CurrentSchemaVersion, migrate.Current)
	}
}

// TestFixturesNeverDeleted compares the files under versions/ with MANIFEST.txt:
// a fixture is frozen history and is never removed (SDD-001 §6 item 4).
func TestFixturesNeverDeleted(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(versionsDir, "MANIFEST.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var have []string
	err = filepath.WalkDir(versionsDir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == "MANIFEST.txt" {
			return err
		}
		rel, _ := filepath.Rel(versionsDir, p)
		have = append(have, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(have)
	for want := range strings.FieldsSeq(string(data)) {
		if !slices.Contains(have, want) {
			t.Errorf("fixture %s was deleted; versions/ files are never removed", want)
		}
	}
	for _, f := range have {
		if !strings.Contains("\n"+string(data)+"\n", "\n"+f+"\n") && !strings.Contains(string(data), f) {
			t.Errorf("fixture %s is not listed in MANIFEST.txt", f)
		}
	}
}

func TestMigrateRefusesNewer(t *testing.T) {
	doc := migrate.Doc{"schema_version": json.Number(strconv.Itoa(migrate.Current + 1))}
	_, err := migrate.Migrate(doc)
	me, ok := errors.AsType[*migrate.Error](err)
	if !ok || me.Code != migrate.CodeSchemaVersion {
		t.Fatalf("got %v", err)
	}
	for _, n := range []int{migrate.Current, migrate.Current + 1} {
		if !strings.Contains(me.Message, strconv.Itoa(n)) {
			t.Errorf("message %q lacks %d", me.Message, n)
		}
	}
	if !strings.Contains(me.Message, "upgrade") {
		t.Errorf("message %q lacks the instruction to upgrade", me.Message)
	}
	if _, wrote := doc["title"]; wrote || len(doc) != 1 {
		t.Errorf("the document was written: %v", doc)
	}
}

func TestSyntheticV0MigratesToV1AndValidates(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(versionsDir, "v1", "launch-teaser.json"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := timeline.DecodeRaw(data)
	if err != nil {
		t.Fatal(err)
	}
	// A pretend v0 called the project "title"; v0to1 renames it to "name".
	raw["title"] = raw["name"]
	delete(raw, "name")
	raw["schema_version"] = json.Number("0")
	v0to1 := func(d migrate.Doc) (migrate.Doc, error) {
		d["name"] = d["title"]
		delete(d, "title")
		return d, nil
	}
	out, err := migrate.Run(raw, 1, migrate.Steps{0: v0to1})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := timeline.FromRaw(out)
	if err != nil {
		t.Fatal(err)
	}
	if vs := timeline.Validate(doc); len(vs) != 0 {
		t.Errorf("%v", vs)
	}
}

func TestRunChainsAndReportsErrors(t *testing.T) {
	steps := migrate.Steps{
		1: func(d migrate.Doc) (migrate.Doc, error) { d["a"] = true; return d, nil },
		2: func(d migrate.Doc) (migrate.Doc, error) { d["b"] = true; return d, nil },
	}
	out, err := migrate.Run(migrate.Doc{"schema_version": json.Number("1")}, 3, steps)
	if err != nil || out["a"] != true || out["b"] != true || out["schema_version"] != json.Number("3") {
		t.Errorf("chain: %v %v", out, err)
	}
	same, err := migrate.Run(migrate.Doc{"schema_version": json.Number("3")}, 3, steps)
	if err != nil || len(same) != 1 {
		t.Errorf("current: %v %v", same, err)
	}
	if _, err = migrate.Run(migrate.Doc{"schema_version": json.Number("1")}, 2, nil); err == nil ||
		!strings.Contains(err.Error(), "no migration from schema_version 1") {
		t.Errorf("missing step: %v", err)
	}
	boom := errors.New("boom")
	failing := migrate.Steps{1: func(migrate.Doc) (migrate.Doc, error) { return nil, boom }}
	if _, err = migrate.Run(migrate.Doc{"schema_version": json.Number("1")}, 2, failing); !errors.Is(err, boom) {
		t.Errorf("failing step: %v", err)
	}
	for src, code := range map[string]string{"": migrate.CodeShape, "x": migrate.CodeSchemaVersion, "-1": migrate.CodeSchemaVersion} {
		doc := migrate.Doc{}
		if src != "" {
			doc["schema_version"] = json.Number(src)
		}
		_, err := migrate.Run(doc, 1, nil)
		if me, ok := errors.AsType[*migrate.Error](err); !ok || me.Code != code {
			t.Errorf("version %q: %v", src, err)
		}
	}
}

func TestEveryVersionHasAStep(t *testing.T) {
	if err := migrate.Check(migrate.Current, nil); err != nil {
		t.Fatal(err)
	}
	noop := func(d migrate.Doc) (migrate.Doc, error) { return d, nil }
	if err := migrate.Check(3, migrate.Steps{1: nil, 2: noop}); err == nil {
		t.Error("a missing step went unnoticed")
	}
}

const starterDir = "../../../service/templates/starter"

// TestStarterTemplatesFrozen fails when a starter template has no copy with the same
// canonical bytes in the v1 fixtures, so every future version migrates it (P2-SCH-004).
func TestStarterTemplatesFrozen(t *testing.T) {
	starters, err := filepath.Glob(filepath.Join(starterDir, "*.template.json"))
	if err != nil || len(starters) != 3 {
		t.Fatalf("want 3 starter templates in %s, got %d: %v", starterDir, len(starters), err)
	}
	var fixtures [][]byte
	files, _ := filepath.Glob(filepath.Join(versionsDir, "v1", "*.json"))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if doc, err := timeline.Decode(data); err == nil {
			fixtures = append(fixtures, timeline.Canonical(doc))
		}
	}
	for _, s := range starters {
		data, err := os.ReadFile(s)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := timeline.Decode(data)
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		want := timeline.Canonical(doc)
		if !slices.ContainsFunc(fixtures, func(b []byte) bool { return bytes.Equal(b, want) }) {
			t.Errorf("%s has no copy with the same canonical bytes in testdata/versions/v1/", filepath.Base(s))
		}
	}
}
