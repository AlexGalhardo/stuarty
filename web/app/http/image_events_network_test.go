package httpapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
)

// TestImageEventsInNetworkModeAreTheOwners: in network mode the stream of an image opens for its owner only. Another
// member gets the answer of an image that does not exist, and a key limited to a project is refused, before the
// response becomes a stream (SDD-006 section 12, SDD-021 section 10).
func TestImageEventsInNetworkModeAreTheOwners(t *testing.T) {
	h := newNetworkHarness(t, resolved(), true)
	h.user(t, "ana@example.com", service.RoleMember)
	h.user(t, "bob@example.com", service.RoleMember)
	ana := h.session(t, "ana@example.com")
	bob := h.session(t, "bob@example.com")

	made := h.send(t, http.MethodPost, "/api/v1/images", `{"name":"Mine","preset":"square_post"}`, ana, nil)
	if made.status != http.StatusOK {
		t.Fatalf("ImageCreate = %d %s", made.status, made.body)
	}

	var img struct {
		ImageID string `json:"image_id"`
	}

	if err := json.Unmarshal(made.body, &img); err != nil || img.ImageID == "" {
		t.Fatalf("%v %s", err, made.body)
	}

	path := "/api/v1/images/" + img.ImageID + "/events"

	// the owner gets a stream: read the head of the answer and leave
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	hr, err := http.NewRequestWithContext(ctx, http.MethodGet, h.srv.URL+path, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}

	hr.Host = netHost
	hr.Header.Set("Origin", netOrigin)
	hr.AddCookie(&http.Cookie{Name: cookieName, Value: ana})

	res, err := h.srv.Client().Do(hr)
	if err != nil {
		t.Fatal(err)
	}

	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("the owner = %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}

	cancel()
	_ = res.Body.Close()

	// another member: as unknown as an image that does not exist
	if other := h.send(t, http.MethodGet, path, "", bob, nil); other.status != http.StatusNotFound ||
		other.code() != "E_NOT_FOUND" {
		t.Errorf("another member = %d %s", other.status, other.body)
	}

	// a key of the owner limited to one project reaches no image
	project := h.send(t, http.MethodPost, "/api/v1/projects", projectJSON("p"), ana, nil)
	if project.status != http.StatusCreated {
		t.Fatalf("project = %d %s", project.status, project.body)
	}

	var p struct {
		ProjectID string `json:"project_id"`
	}

	_ = json.Unmarshal(project.body, &p)

	addFirstTimeline(t, h, ana, p.ProjectID)

	key := h.makeKey(t, ana, `{"name":"a","scopes":["read","edit"],"project_id":"`+p.ProjectID+`"}`)
	if limited := h.asKey(t, key.Secret, http.MethodGet, path, ""); limited.status != http.StatusForbidden ||
		limited.code() != service.CodeForbidden {
		t.Errorf("a project-limited key = %d %s", limited.status, limited.body)
	}
}
