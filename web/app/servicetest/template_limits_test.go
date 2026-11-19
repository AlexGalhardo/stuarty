package servicetest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
)

// The Service reads a template file of up to 64 MiB from disk, whatever the
// HTTP body cap is (SDD-007 section 7.6 item 1); one byte more is refused for
// its size, and neither is accepted as a template.
func TestTemplateImportReadsUpTo64MiBFromDisk(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{})

	const limit = 64 << 20

	write := func(name string, size int) string {
		p := filepath.Join(m.files, name)

		f, err := os.Create(p)
		if err != nil {
			t.Fatal(err)
		}

		if err := f.Truncate(int64(size)); err != nil {
			t.Fatal(err)
		}

		if err := f.Close(); err != nil {
			t.Fatal(err)
		}

		return p
	}

	// Written one at a time and removed at once: a 64 MiB file is real disk on some systems.
	for _, c := range []struct {
		name string
		size int
	}{{"at.zip", limit}, {"over.zip", limit + 1}} {
		name, size := c.name, c.size

		_, err := m.svc.TemplateImport(m.as(ui), service.TemplateImportRequest{Path: write(name, size)})
		if code(err) != command.CodeValidation {
			t.Fatalf("%s: %v, want E_VALIDATION", name, err)
		}

		if err := os.Remove(filepath.Join(m.files, name)); err != nil {
			t.Fatal(err)
		}

		over := strings.Contains(serviceError(t, err).Message, "over 64 MiB")
		if over != (size > limit) {
			t.Errorf(
				"%s (%d bytes): message %q, over-the-limit refusal = %v",
				name,
				size,
				serviceError(t, err).Message,
				over,
			)
		}
	}
}
