package testpublish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
)

// SeedExport writes a succeeded export job of the project and the file it made under dir (the workspace),
// and returns the id of the job. actor is who the job is recorded for; now is the clock of the claim.
func SeedExport(
	t Tester, st service.Store, dir, actor, project, id, ext string, data []byte, now time.Time,
) string {
	t.Helper()

	exports := filepath.Join(dir, "exports", project)
	if err := os.MkdirAll(exports, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(exports, id+ext), data, 0o600); err != nil {
		t.Fatal(err)
	}

	// The digest is recorded as the exporter records it (service/exporter), so a run checks it.
	sum := sha256.Sum256(data)

	res, merr := json.Marshal(service.JobResult{Export: &service.ExportResult{
		Format: strings.TrimPrefix(ext, "."), Bytes: int64(len(data)), SHA256: hex.EncodeToString(sum[:]),
	}})
	if merr != nil {
		t.Fatal(merr)
	}

	werr := st.Write(context.Background(), func(ctx context.Context, tx service.Tx) error {
		row := service.JobRow{
			ID:        id,
			Kind:      service.JobExport,
			ProjectID: &project,
			Actor:     actor,
			State:     service.JobQueued,
			Input:     json.RawMessage(`{}`),
		}
		if err := tx.Jobs().CreateLimited(ctx, row, nil); err != nil {
			return err
		}

		if _, err := tx.Jobs().Claim(ctx, id, now.UnixMilli()); err != nil {
			return err
		}

		_, err := tx.Jobs().Finish(ctx, id, service.JobSucceeded, res, nil, now.UnixMilli())

		return err
	})
	if werr != nil {
		t.Fatal(werr)
	}

	return id
}
