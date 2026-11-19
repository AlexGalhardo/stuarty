package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/events"
)

// importSpy records what the route hands to TemplateImport.
type importSpy struct {
	service.Service

	mu    sync.Mutex
	paths []string
}

func (s *importSpy) TemplateImport(
	_ context.Context,
	req service.TemplateImportRequest,
) (*service.TemplateResult, error) {
	s.mu.Lock()
	s.paths = append(s.paths, req.Path)
	s.mu.Unlock()

	return &service.TemplateResult{}, nil
}

// POST /templates/import takes the path of a file on the server, not the file
// (SDD-003 section 6): the 64 MiB limit of a template is read from disk by the
// Service, and the 1 MiB body cap of the route never meets it. A long path
// passes, and a body of 2 MiB is still refused with E_PAYLOAD_TOO_LARGE, so no
// route has a body cap other than 1 MiB.
func TestTemplateImportRouteKeepsTheGlobalBodyCap(t *testing.T) {
	t.Parallel()

	spy := &importSpy{}
	h := newHarnessWrapped(t, events.Options{}, func(s service.Service) service.Service {
		spy.Service = s

		return spy
	})

	long := "/srv/media/" + strings.Repeat("a/", 1500) + "t.stuarty-template.zip"

	res := h.do(req{method: http.MethodPost, path: "/api/v1/templates/import", body: `{"path":"` + long + `"}`})
	if res.status != http.StatusCreated {
		t.Fatalf("a long path = %d %s, want 201", res.status, res.body)
	}

	spy.mu.Lock()
	got := append([]string(nil), spy.paths...)
	spy.mu.Unlock()

	if len(got) != 1 || got[0] != long {
		t.Errorf("the Service got %d paths, first %q", len(got), strings.Join(got, ","))
	}

	big := `{"path":"/x","dry_run":false,"pad":"` + strings.Repeat("x", 2<<20) + `"}`

	res = h.do(req{method: http.MethodPost, path: "/api/v1/templates/import", body: big})
	if res.status != http.StatusRequestEntityTooLarge || res.json(t)["code"] != "E_PAYLOAD_TOO_LARGE" {
		t.Errorf("a 2 MiB body = %d %s, want 413 E_PAYLOAD_TOO_LARGE", res.status, res.body)
	}

	spy.mu.Lock()
	defer spy.mu.Unlock()

	if len(spy.paths) != 1 {
		t.Errorf("an oversized body reached the Service: %v", spy.paths)
	}
}
