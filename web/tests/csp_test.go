package tests

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/AlexGalhardo/vukem/vukemtest"

	"github.com/AlexGalhardo/vukem-video/web/bootstrap"
)

// TestCSPMatchesSDD007 pins the served policy to SDD-007 section 8.5.5.
func TestCSPMatchesSDD007(t *testing.T) {
	t.Parallel()

	app := vukemtest.New(t, bootstrap.New)

	resp := app.Client(t).Get("/up")
	csp := resp.Header(http.CanonicalHeaderKey("Content-Security-Policy"))
	nonce := regexp.MustCompile(`'nonce-[^']+'`)
	got := nonce.ReplaceAllString(csp, "'nonce-<n>'")

	want := "default-src 'self'; script-src 'self' 'nonce-<n>'; worker-src 'self'; style-src 'self' 'nonce-<n>'; " +
		"img-src 'self' data:; media-src 'self'; connect-src 'self'; font-src 'self'; " +
		"frame-ancestors 'none'; base-uri 'none'; object-src 'none'; form-action 'self'"
	if got != want {
		t.Fatalf("CSP =\n%s\nwant\n%s", got, want)
	}

	for _, banned := range []string{"unsafe-inline", "unsafe-eval", "blob:"} {
		if strings.Contains(csp, banned) {
			t.Errorf("CSP contains %s", banned)
		}
	}
}
