package httpapi_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// SDD-003 section 6.8: the commands of session C, run with the real curl against
// a real listener. The token goes through a private header file (curl -H @file),
// never through a command line. The steps that need media, a frame or an export
// join when those operations exist.
func TestSessionCWithCurl(t *testing.T) {
	t.Parallel()

	curl, err := exec.LookPath("curl")
	if err != nil {
		t.Skip("curl is not installed")
	}

	h := newHarness(t)

	headers := filepath.Join(t.TempDir(), "headers")
	content := "Authorization: Bearer " + canary + "\nStuarty-Client: curl-demo\nContent-Type: application/json\n"

	if err := os.WriteFile(headers, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	run := func(args ...string) string {
		t.Helper()

		ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
		defer cancel()

		out, err := exec.CommandContext(ctx, curl, append([]string{"-sS", "-H", "@" + headers}, args...)...).
			CombinedOutput()
		if err != nil {
			t.Fatalf("curl %v: %v\n%s", args, err, out)
		}

		return string(out)
	}

	api := h.srv.URL + "/api/v1"

	created := run("-i", "-d", `{"name":"Launch teaser"}`, api+"/projects")
	if !strings.HasPrefix(created, "HTTP/1.1 201") || !strings.Contains(created, "Location: /api/v1/projects/prj_") {
		t.Fatalf("create:\n%s", created)
	}

	var body struct {
		ProjectID string `json:"project_id"`
	}

	_, payload, _ := strings.Cut(created, "\r\n\r\n")
	if err := json.Unmarshal([]byte(payload), &body); err != nil || body.ProjectID == "" {
		t.Fatalf("create body: %v\n%s", err, created)
	}

	project := api + "/projects/" + body.ProjectID

	edit1 := run(
		"-d",
		`{"base_revision":0,"operations":[{"op":"track.add","args":{"kind":"visual","name":"Video","id":"trk_f6g7h8j9k0"}},`+
			`{"op":"track.add","args":{"kind":"visual","name":"Captions","id":"trk_a1b2c3d4e5"}}]}`,
		project+"/edit",
	)
	if !strings.Contains(edit1, `"revision":1`) || !strings.Contains(edit1, `"actor":"local:curl-demo"`) {
		t.Fatalf("edit 1: %s", edit1)
	}

	caption := `{"base_revision":1,"operations":[{"op":"text.add","args":{"track_id":"trk_a1b2c3d4e5","start":"0s","duration":"2s","text":"hello"}}]}`

	first := run("-H", "Idempotency-Key: caption-0001", "-d", caption, project+"/edit")
	if !strings.Contains(first, `"revision":2`) || !strings.Contains(first, `"replayed":false`) {
		t.Fatalf("keyed edit: %s", first)
	}

	again := run("-H", "Idempotency-Key: caption-0001", "-d", caption, project+"/edit")
	if !strings.Contains(again, `"revision":2`) || !strings.Contains(again, `"replayed":true`) {
		t.Errorf("replay: %s", again)
	}

	lint := run(project + "/lint")
	if !strings.Contains(lint, `"revision":2`) || !strings.Contains(lint, `"errors":0`) {
		t.Errorf("lint: %s", lint)
	}
}
