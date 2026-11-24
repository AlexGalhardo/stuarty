package aihost

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The pins of .config/aihelper.env (read by scripts/install-aihelper-runtime.sh) and the embedded catalogue
// are the same files: a change of one without the other fails here.
func TestPinFileAgreesWithTheManifest(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".config", "aihelper.env"))
	if err != nil {
		t.Fatal(err)
	}

	pins := map[string]string{}

	for line := range strings.SplitSeq(string(data), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(line), "="); ok && !strings.HasPrefix(k, "#") {
			pins[k] = strings.Trim(v, `"`)
		}
	}

	m, err := DefaultManifest()
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range m.Entries {
		var prefix string

		switch e.Name {
		case ToolRuntime:
			prefix = "ORT"
		case ToolModel:
			prefix = "YUNET"
		default:
			continue
		}

		if pins[prefix+"_URL"] != e.URL || pins[prefix+"_SHA256"] != e.SHA256 ||
			pins[prefix+"_BYTES"] != itoa(e.Bytes) {
			t.Errorf("%s: .config/aihelper.env says %q %q %q, the manifest says %q %q %d", e.Name,
				pins[prefix+"_URL"], pins[prefix+"_SHA256"], pins[prefix+"_BYTES"], e.URL, e.SHA256, e.Bytes)
		}
	}

	if pins["ORT_VERSION"] == "" {
		t.Error("ORT_VERSION is not pinned")
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
