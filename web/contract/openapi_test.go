package contract

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// openAPIFile is the OpenAPI document of the HTTP API as the documentation site reads it
// (SDD-010 section 8). The warmup mode of vukem's openapi:export cannot see the routes that need
// the Service, so the document is taken from the running API of the contract environment.
const openAPIFile = "../../site/generated/openapi.json"

func TestOpenAPIDocumentIsCommitted(t *testing.T) {
	d, ok := newHTTP(t).(*httpDriver)
	if !ok {
		t.Fatal("newHTTP is not an httpDriver")
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, d.url+"/openapi.json", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	req.Header.Set("Authorization", "Bearer "+d.token)

	res, err := d.send(req)
	if err != nil {
		t.Fatal(err)
	}

	defer res.Body.Close()

	raw, err := io.ReadAll(res.Body)
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("openapi: %d %v", res.StatusCode, err)
	}

	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer

	enc := json.NewEncoder(&out)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)

	if err := enc.Encode(doc); err != nil {
		t.Fatal(err)
	}

	path := filepath.FromSlash(openAPIFile)

	if os.Getenv("UPDATE_OPENAPI") == "1" {
		if err := os.WriteFile(path, out.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}

		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (UPDATE_OPENAPI=1 go test ./web/contract/ -run OpenAPI records it)", err)
	}

	if !bytes.Equal(bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n")), out.Bytes()) {
		t.Errorf("site/generated/openapi.json is out of date: run UPDATE_OPENAPI=1 go test ./web/contract/ -run OpenAPI")
	}
}
