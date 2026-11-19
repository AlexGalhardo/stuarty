package servicetest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/store"
)

// invariants are those of SDD-005 section 9.2, checked after every crash.
func invariants(t *testing.T, st *store.Store, project string) {
	t.Helper()

	ctx := context.Background()

	row, err := st.Projects().Get(ctx, project)
	if err != nil {
		t.Fatal(err)
	}

	rows, _, err := st.Operations().Page(ctx, project, service.HistoryQuery{Limit: 200})
	if err != nil {
		t.Fatal(err)
	}

	// projects.revision is the highest operations.revision, and revisions are contiguous from 1.
	if int64(len(rows)) != row.Revision {
		t.Fatalf("%d log rows for revision %d", len(rows), row.Revision)
	}

	for i := range rows {
		if rows[i].Revision != row.Revision-int64(i) {
			t.Fatalf("log not contiguous: row %d is r%d under r%d", i, rows[i].Revision, row.Revision)
		}
	}

	// sha256(document) equals doc_sha256 equals the row's at that revision.
	sum := sha256.Sum256(row.Document)
	if got := hex.EncodeToString(sum[:]); got != row.DocSHA256 {
		t.Errorf("sha256(document) = %s, doc_sha256 = %s", got, row.DocSHA256)
	}

	if len(rows) > 0 && rows[0].DocSHA256 != row.DocSHA256 {
		t.Errorf("log row doc_sha256 %s differs from the project's %s", rows[0].DocSHA256, row.DocSHA256)
	}

	// The summary columns equal the summary recomputed from the document.
	doc, err := timeline.Decode(row.Document)
	if err != nil {
		t.Fatal(err)
	}

	if row.Summary.Items != len(doc.Items) || row.Summary.Tracks != len(doc.Tracks) ||
		row.Summary.Assets != len(doc.Assets) ||
		row.Summary.Width != doc.Settings.Width ||
		row.Summary.DurationFrames != endOf(doc) {
		t.Errorf("summary %+v does not match the document", row.Summary)
	}

	if !bytes.Equal(timeline.Canonical(doc), row.Document) {
		t.Error("the stored document is not canonical")
	}

	var integrity string
	if err := st.DB().QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Errorf("integrity_check = %q, %v", integrity, err)
	}
}

func endOf(doc *timeline.Document) int64 {
	var end int64

	for id := range doc.Items {
		it := doc.Items[id]
		end = max(end, int64(it.Start+it.Duration))
	}

	return end
}
