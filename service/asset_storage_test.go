package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
	"github.com/AlexGalhardo/stuarty/service/assets"
)

const storedHex = "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"

// stored puts a file where the content store keeps the original of storedHex and returns the workspace.
func stored(t *testing.T, body string) string {
	t.Helper()

	ws := t.TempDir()

	p, err := enginestore.New(ws).Path(storedHex)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	return ws
}

func sized(s *svc, state IngestState, rowSize int64, check bool) assets.Held {
	h := assets.Held{Hash: "sha256:" + storedHex}
	s.heldSize(&h, &AssetRow{IngestState: state, SizeBytes: rowSize}, check)

	return h
}

func TestAssetLibraryASizeOfAReadyFileIsTheRowAndOverFourGiBIsKept(t *testing.T) {
	t.Parallel()

	s := &svc{workspace: stored(t, "x")}
	over := int64(5) << 30 // 5 GiB: the row says so, the test file is one byte

	h := sized(s, IngestReady, over, true)
	if h.State != assets.StateReady || h.Bytes == nil || *h.Bytes != over {
		t.Errorf("held = %+v, bytes %v", h, h.Bytes)
	}
}

func TestAssetLibraryAFileStillImportingHasNoSizeYet(t *testing.T) {
	t.Parallel()

	s := &svc{workspace: stored(t, "x")}

	for _, state := range []IngestState{IngestPending, IngestIngesting} {
		if h := sized(s, state, 1234, true); h.State != assets.StateImporting || h.Bytes != nil {
			t.Errorf("%s: state %q, bytes %v; want importing and no size", state, h.State, h.Bytes)
		}
	}
}

func TestAssetLibraryAFileMissingOnDiskIsMissingAndNeverZero(t *testing.T) {
	t.Parallel()

	s := &svc{workspace: t.TempDir()} // nothing in the store

	h := sized(s, IngestReady, 1234, true)
	if h.State != assets.StateMissing || h.Bytes != nil {
		t.Errorf("state %q, bytes %v; want missing and no size", h.State, h.Bytes)
	}

	// Without the check the row is trusted (a lookup by hash for a write does not stat every file).
	if h := sized(s, IngestReady, 1234, false); h.State != assets.StateReady || h.Bytes == nil {
		t.Errorf("unchecked held = %+v", h)
	}
}

func TestAssetLibraryAFolderWhereTheFileShouldBeIsMissing(t *testing.T) {
	t.Parallel()

	ws := t.TempDir()

	p, err := enginestore.New(ws).Path(storedHex)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(p, 0o700); err != nil {
		t.Fatal(err)
	}

	if h := sized(&svc{workspace: ws}, IngestReady, 10, true); h.State != assets.StateMissing {
		t.Errorf("state %q, want missing", h.State)
	}
}

func TestAssetLibraryAFailedIngestKeepsItsSizeAndSaysFailed(t *testing.T) {
	t.Parallel()

	s := &svc{workspace: stored(t, "x")}

	if h := sized(s, IngestFailed, 77, true); h.State != assets.StateFailed || h.Bytes == nil || *h.Bytes != 77 {
		t.Errorf("held = %+v bytes %v", h, h.Bytes)
	}
}

func TestAssetLibraryAServiceWithNoWorkspaceTrustsTheRow(t *testing.T) {
	t.Parallel()

	if h := sized(&svc{}, IngestReady, 9, true); h.State != assets.StateReady || h.Bytes == nil || *h.Bytes != 9 {
		t.Errorf("held = %+v", h)
	}
}

func TestAssetLibraryStoredOriginalIsOnlyForAValidHash(t *testing.T) {
	t.Parallel()

	s := &svc{workspace: t.TempDir()}

	for _, bad := range []string{"", "sha256:zz", "sha256:" + storedHex + "0", "../../etc/passwd", "sha256:" + strings.ToUpper(storedHex)} {
		if p, ok := s.storedOriginal(bad); ok {
			t.Errorf("storedOriginal(%q) = %q, want refusal", bad, p)
		}
	}

	if _, ok := s.storedOriginal("sha256:" + storedHex); !ok {
		t.Error("a valid hash has a stored path")
	}
}
