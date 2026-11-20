package library

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"strings"
	"testing"

	core "github.com/AlexGalhardo/stuarty/core/library"
)

// seedArchive is a .zip, or a .tar.gz when zipped is false, of the named files.
func seedArchive(zipped bool, files map[string]string) []byte {
	var buf bytes.Buffer

	if zipped {
		zw := zip.NewWriter(&buf)
		for name, data := range files {
			w, _ := zw.Create(name)
			_, _ = w.Write([]byte(data))
		}

		_ = zw.Close()

		return buf.Bytes()
	}

	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)

	for name, data := range files {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(data)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(data))
	}

	_ = tw.Close()
	_ = gz.Close()

	return buf.Bytes()
}

// FuzzSkillArchive feeds any bytes as a skill archive, read as a .zip and as a .tar.gz (SDD-008 section 13.2):
// no panic, and what an archive yields is within the limits of SDD-018: at most maxArchiveEntries files, every
// path a plain relative one, no two names that fold to the same, no file over twice MaxSkillFile, and no more
// than maxInflated bytes in all.
func FuzzSkillArchive(f *testing.F) {
	good := map[string]string{"SKILL.md": goodSkillMD, "references/patterns.md": "Open with a question.\n"}
	hostile := map[string]string{"../escape.md": "x", "SKILL.md": goodSkillMD, "skill.md": "twin"}

	for _, zipped := range []bool{true, false} {
		f.Add(seedArchive(zipped, good))
		f.Add(seedArchive(zipped, hostile))
	}

	f.Add([]byte("PK\x03\x04"))
	f.Add([]byte{0x1f, 0x8b, 0x08, 0x00})

	f.Fuzz(func(t *testing.T, raw []byte) {
		if files, err := readZip(bytes.NewReader(raw), int64(len(raw))); err == nil {
			checkArchiveFiles(t, "zip", files)
		}

		if files, err := readTarGz(bytes.NewReader(raw)); err == nil {
			checkArchiveFiles(t, "tar.gz", files)
		}
	})
}

func checkArchiveFiles(t *testing.T, kind string, files []core.File) {
	t.Helper()

	if len(files) > maxArchiveEntries {
		t.Fatalf("%s: %d files, more than %d", kind, len(files), maxArchiveEntries)
	}

	seen := map[string]bool{}

	var total int64

	for _, file := range files {
		if err := core.ValidFilePath(file.Path); err != nil {
			t.Fatalf("%s: the path %q is not a plain relative one: %v", kind, file.Path, err)
		}

		folded := strings.ToLower(file.Path)
		if seen[folded] {
			t.Fatalf("%s: two files fold to %q", kind, folded)
		}

		seen[folded] = true

		if len(file.Data) > core.MaxSkillFile*2 {
			t.Fatalf("%s: %q is %d bytes, over the limit", kind, file.Path, len(file.Data))
		}

		total += int64(len(file.Data))
	}

	if total > maxInflated {
		t.Fatalf("%s: %d bytes inflated, over %d", kind, total, maxInflated)
	}
}
