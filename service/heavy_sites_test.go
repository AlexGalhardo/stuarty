package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The heavy-call limit is spent by seven operations, and the documentation pages and the OpenAPI
// document list them (VV-115). Adding or removing a call site fails here until those lists change.
func TestHeavyLimitCallSites(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}

	sites := 0

	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}

		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}

		sites += strings.Count(string(src), "s.heavyLimit(ctx, c)")
	}

	if sites != 7 {
		t.Errorf(
			"%d call sites of heavyLimit, want 7: update the heavyOperations of web/app/http, the pages api-keys and jobs of the site and SDD-000",
			sites,
		)
	}
}
