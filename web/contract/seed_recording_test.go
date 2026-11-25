package contract

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/recording"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/recordings"
)

// seedRecording is the scenario step "SeedRecording": it gives the workspace of a driver a recording that a person
// added, whose screen track is an asset of the project, with a pointer event file. No capture helper runs in these
// environments, and the commands that finish a screen take (SDD-002 C.93 to C.95) read what such a recording holds,
// so this is how a scenario holds one. Every driver gets the same row and the same file.
func seedRecording(t *testing.T, d driver, s step) {
	t.Helper()

	var a struct {
		ProjectID string `json:"project_id"`
		AssetID   string `json:"asset_id"`
		Events    string `json:"events"`
	}

	if err := json.Unmarshal(s.Args, &a); err != nil {
		t.Fatalf("%s: SeedRecording: %v", d.name(), err)
	}

	pg, err := d.service().
		ProjectGet(callerCtx(), service.ProjectGetRequest{ProjectID: a.ProjectID, Detail: "document"})
	if err != nil {
		t.Fatalf("%s: SeedRecording: %v", d.name(), err)
	}

	asset, ok := pg.Document.Assets[a.AssetID]
	if !ok {
		t.Fatalf("%s: SeedRecording: %s is not an asset of %s", d.name(), a.AssetID, a.ProjectID)
	}

	row := recordings.Row{
		ID:          "rec_0seed00001",
		ProjectID:   a.ProjectID,
		State:       string(recording.Ready),
		Review:      string(recording.ReviewAdded),
		SourceKind:  "display",
		Width:       1920,
		Height:      1080,
		FPS:         30,
		AssetHash:   asset.Hash,
		AssetID:     asset.ID,
		Actor:       "seed",
		CreatedMs:   fixedNow.UnixMilli(),
		Tracks:      "[]",
		Pauses:      "[]",
		EventsCount: 2,
	}

	env := d.env()

	if err := env.Store.Write(context.Background(), func(ctx context.Context, tx service.Tx) error {
		return tx.Recordings().Insert(ctx, row)
	}); err != nil {
		t.Fatalf("%s: SeedRecording: %v", d.name(), err)
	}

	folder := filepath.Join(string(env.Dir), "recordings", row.ID)
	if err := os.MkdirAll(folder, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(folder, recording.EventsFile), []byte(a.Events), 0o600); err != nil {
		t.Fatal(err)
	}
}
