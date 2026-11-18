package servicetest

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service"
)

// patchDeclaredSize rewrites the uncompressed size that the central directory of a zip declares for
// one entry, which is what a hostile archive does to slip past a size cap.
func patchDeclaredSize(t *testing.T, raw []byte, name string, size uint32) {
	t.Helper()

	const centralHeader, nameAt, sizeAt = "PK\x01\x02", 46, 24

	for i := 0; ; {
		j := bytes.Index(raw[i:], []byte(centralHeader))
		if j < 0 {
			t.Fatalf("no central directory record for %s", name)
		}

		i += j
		n := int(binary.LittleEndian.Uint16(raw[i+28:]))

		if string(raw[i+nameAt:i+nameAt+n]) == name {
			binary.LittleEndian.PutUint32(raw[i+sizeAt:], size)

			return
		}

		i += nameAt + n
	}
}

// SDD-007 T-20 (bundle half), SDD-005 section 16.3 step 1: a size that the archive declares is
// checked against the policy and against the bytes that are really there.
func TestSecurity_T20_BundleDeclaredSizes(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{})
	project, _ := src.bundleProject()
	good := src.exportBundle(project, nil).Result.Export.Path

	raw, err := os.ReadFile(good)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		entry string
		size  uint32
		code  string
	}{
		{"manifest declared over its cap", "manifest.json", 8 << 20, "E_LIMIT_EXCEEDED"},
		{"document declared over its cap", "document.json", 1 << 30, "E_LIMIT_EXCEEDED"},
		{"document declared at 4 GiB", "document.json", 1<<32 - 1, "E_LIMIT_EXCEEDED"},
		{"document declared smaller than it is", "document.json", 5, "E_VALIDATION"},
		{"manifest declared smaller than it is", "manifest.json", 5, "E_VALIDATION"},
	}

	dst := newMediaEnv(t, mediaCfg{})
	files := t.TempDir()

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			hostile := bytes.Clone(raw)
			patchDeclaredSize(t, hostile, c.entry, c.size)

			path := filepath.Join(files, "lying.vvbundle")
			if err := os.WriteFile(path, hostile, 0o600); err != nil {
				t.Fatal(err)
			}

			res, err := dst.importBundle(path, false)
			if err == nil {
				t.Fatalf("a bundle with a false size was accepted: %+v", res)
			}

			if !isCode(err, c.code) {
				t.Errorf("import = %v, want %s", err, c.code)
			}

			if listProjects(t, dst) != 0 || len(storeFiles(t, dst.ws)) != 0 {
				t.Error("a refused bundle left a project or files behind")
			}
		})
	}
}

// fontBundle is a bundle of a project with a user font, exported from src, and the font hash.
func fontBundle(t *testing.T, src *mediaEnv, font []byte) (path, hash string) {
	t.Helper()

	sum := sha256.Sum256(font)
	hash = hex.EncodeToString(sum[:])

	dir := filepath.Join(src.ws, "fonts")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	// Planted past the validation of the font store: the exporting side is not the one under test.
	if err := os.WriteFile(filepath.Join(dir, hash), font, 0o600); err != nil {
		t.Fatal(err)
	}

	job := src.exportBundle(src.brandProject(hash), nil)
	if job.State != service.JobSucceeded {
		t.Fatalf("export = %+v", job)
	}

	return job.Result.Export.Path, hash
}

// SDD-007 T-21, SDD-005 section 16.3 step 4: a crafted font inside a bundle refuses the whole bundle
// and nothing is stored, neither the font nor any media.
func TestSecurity_T21_BundleWithACraftedFontStoresNothing(t *testing.T) {
	anton := bundledFontFile(t, "Anton-Regular.ttf")

	manyTables := make([]byte, 64)
	copy(manyTables, "\x00\x01\x00\x00")
	binary.BigEndian.PutUint16(manyTables[4:], 65535)

	crafted := map[string][]byte{
		"truncated":       anton[:len(anton)/3],
		"numTables 65535": manyTables,
		"not a font":      []byte("this is not a font file at all"),
	}

	for name, font := range crafted {
		t.Run(name, func(t *testing.T) {
			src := newMediaEnv(t, mediaCfg{beta: true})
			path, hash := fontBundle(t, src, font)

			dst := newMediaEnv(t, mediaCfg{beta: true})

			res, err := dst.importBundle(path, false)
			if err == nil {
				t.Fatalf("a bundle with a crafted font was accepted: %+v", res)
			}

			if !isCode(err, "E_VALIDATION") {
				t.Errorf("import = %v, want E_VALIDATION", err)
			}

			if strings.Contains(err.Error(), hash) {
				t.Errorf("the refusal names the font hash: %v", err)
			}

			if listProjects(t, dst) != 0 || len(storeFiles(t, dst.ws)) != 0 {
				t.Error("a refused bundle left a project or media behind")
			}

			if entries, _ := os.ReadDir(filepath.Join(dst.ws, "fonts")); len(entries) != 0 {
				t.Errorf("%d font files stored for a crafted font", len(entries))
			}
		})
	}
}

// SDD-007 T-21, SDD-005 section 16.3 step 4: in network mode a bundle cannot link a font it does not
// carry, and the refusal is the same whether the font store holds that hash or not.
func TestSecurity_T21_BundleCannotNameAnotherUsersFont(t *testing.T) {
	src := newMediaEnv(t, mediaCfg{beta: true})
	path, hash := fontBundle(t, src, bundledFontFile(t, "Anton-Regular.ttf"))

	var kept []testEntry

	for _, e := range bundleEntries(t, path) {
		if !strings.HasPrefix(e.name, "font.") {
			kept = append(kept, e)
		}
	}

	withManifest(t, kept, func(m *service.BundleManifest) { m.Fonts = []service.BundleFontFile{} })

	stripped := writeBundleEntries(t, t.TempDir(), "stripped.vvbundle", kept)
	req := service.ProjectBundleImportRequest{Path: stripped, AsNew: true}
	bob := service.Caller{Actor: "user:bob", Scopes: service.RoleScopes(service.RoleMember), OwnerUserID: "bob"}

	known, errKnown := src.svc.(bundleImporter).ProjectBundleImport(service.WithCaller(context.Background(), bob), req)
	fresh := newMediaEnv(t, mediaCfg{beta: true})
	_, errUnknown := fresh.svc.(bundleImporter).ProjectBundleImport(service.WithCaller(context.Background(), bob), req)

	if known != nil || !isCode(errKnown, "E_VALIDATION") || !isCode(errUnknown, "E_VALIDATION") {
		t.Fatalf("known = %+v, %v; unknown = %v; want both refused", known, errKnown, errUnknown)
	}

	if errKnown.Error() != errUnknown.Error() {
		t.Errorf("the refusals differ, which tells whether a font exists:\n%v\n%v", errKnown, errUnknown)
	}

	for _, err := range []error{errKnown, errUnknown} {
		if strings.Contains(err.Error(), hash) {
			t.Errorf("the refusal names the font hash: %v", err)
		}
	}

	if listProjects(t, fresh) != 0 || len(storeFiles(t, fresh.ws)) != 0 {
		t.Error("a refused bundle left something behind")
	}

	// The local owner's font store is their own: the same bundle imports.
	if _, err := src.importBundle(stripped, true); err != nil {
		t.Errorf("local import of a bundle that relies on the font store: %v", err)
	}
}

func fuzzBundleZip(entries map[string]string) []byte {
	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	for _, name := range slices.Sorted(maps.Keys(entries)) {
		w, _ := zw.Create(name)
		_, _ = w.Write([]byte(entries[name]))
	}

	_ = zw.Close()

	return buf.Bytes()
}

// FuzzBundleArchive: whatever the bytes of a .vvbundle, the import returns or refuses; it never
// panics, and a refusal leaves no project and no file in the stores (SDD-007 T-20, T-21).
func FuzzBundleArchive(f *testing.F) {
	hexName := strings.Repeat("ab", 32)
	manifest := `{"format":"vukem-video-bundle","bundle_version":1,"project":{"id":"prj_0000000000"}}`

	f.Add(fuzzBundleZip(map[string]string{"manifest.json": manifest, "document.json": "{}"}))
	f.Add(fuzzBundleZip(map[string]string{
		"manifest.json": manifest, "document.json": "{}",
		"media." + hexName: "media", "font." + hexName: "\x00\x01\x00\x00",
	}))
	f.Add(fuzzBundleZip(map[string]string{"../manifest.json": manifest, "/etc/passwd": "x"}))
	f.Add([]byte("PK\x03\x04"))
	f.Add([]byte{})

	f.Fuzz(func(t *testing.T, data []byte) {
		tb := plainTempTB{t}
		dst := newMediaEnv(tb, mediaCfg{beta: true})

		path := filepath.Join(tb.TempDir(), "fuzz.vvbundle")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}

		if _, err := dst.importBundle(path, false); err == nil {
			return
		}

		if listProjects(t, dst) != 0 || len(storeFiles(t, dst.ws)) != 0 {
			t.Fatal("a refused bundle left a project or media behind")
		}

		if entries, _ := os.ReadDir(filepath.Join(dst.ws, "fonts")); len(entries) != 0 {
			t.Fatalf("a refused bundle left %d fonts behind", len(entries))
		}
	})
}

// plainTempTB gives temporary folders without the "#" that the name of a fuzz seed puts in
// testing.T.TempDir, which the workspace path rules refuse.
type plainTempTB struct{ testing.TB }

func (p plainTempTB) TempDir() string {
	p.Helper()

	dir, err := os.MkdirTemp("", "vvfuzz")
	if err != nil {
		p.Fatal(err)
	}

	p.Cleanup(func() { _ = os.RemoveAll(dir) })

	return dir
}
