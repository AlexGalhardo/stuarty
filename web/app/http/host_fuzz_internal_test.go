package httpapi

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// FuzzHostCheck (SDD-007 section 12): for any Host value and any path other than the health
// probe, hostAllowed accepts exactly the three loopback names with the listen port, compared
// case-insensitively, and nothing else.
func FuzzHostCheck(f *testing.F) {
	for _, seed := range []string{
		"127.0.0.1:7878", "localhost:7878", "[::1]:7878", "LOCALHOST:7878",
		"127.0.0.1", "127.1:7878", "localhost.:7878", "foo.localhost:7878",
		"[::ffff:127.0.0.1]:7878", "127.0.0.1:7879", "evil.example:7878", "",
		"127.0.0.1:7878\x00", " localhost:7878", "localhost:7878 ", "localhost:07878",
	} {
		f.Add(seed, "/api/v1/projects")
	}

	f.Add("evil.example", healthPath)

	f.Fuzz(func(t *testing.T, host, path string) {
		if path == healthPath || !strings.HasPrefix(path, "/") {
			return
		}

		r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		r.URL.Path = path
		r.Host = host

		local := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 7878}
		r = r.WithContext(context.WithValue(r.Context(), http.LocalAddrContextKey, net.Addr(local)))

		want := false

		// ASCII-only fold, independent of the Unicode tables the code under test uses.
		folded := []byte(host)
		for i, c := range folded {
			if c >= 'A' && c <= 'Z' {
				folded[i] = c + 'a' - 'A'
			}
		}

		switch string(folded) {
		case "127.0.0.1:7878", "localhost:7878", "[::1]:7878":
			want = true
		}

		if got := hostAllowed(r); got != want {
			t.Fatalf("hostAllowed(%q) = %v, want %v", host, got, want)
		}
	})
}

// FuzzSignInRedirect (SDD-007 section 3.6 item 4): of every request, only a top-level navigation by GET to
// exactly one of the two callback paths is the return of a consent screen; no other method, path (a prefix, a
// suffix, a different case, a dot segment) or fetch metadata is.
func FuzzSignInRedirect(f *testing.F) {
	f.Add("GET", "/social/callback", "navigate", "document")
	f.Add("GET", "/auth/callback", "navigate", "document")
	f.Add("POST", "/auth/callback", "navigate", "document")
	f.Add("GET", "/auth/callback/", "navigate", "document")
	f.Add("GET", "/auth/../social/callback", "cors", "empty")

	f.Fuzz(func(t *testing.T, method, path, mode, dest string) {
		if strings.ContainsAny(method, " \r\n\t") || method == "" {
			return
		}

		r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/", http.NoBody)
		r.Method = method
		r.URL.Path = path
		r.Header.Set("Sec-Fetch-Mode", mode)
		r.Header.Set("Sec-Fetch-Dest", dest)

		want := method == http.MethodGet && (path == "/social/callback" || path == "/auth/callback") &&
			mode == "navigate" && dest == "document"
		if got := signInRedirect(r); got != want {
			t.Fatalf("signInRedirect(%s %q, %q, %q) = %v, want %v", method, path, mode, dest, got, want)
		}
	})
}
