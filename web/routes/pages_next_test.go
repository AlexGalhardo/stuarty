package routes

import "testing"

func TestSafeNext(t *testing.T) {
	const fallback = "/projects"

	for in, want := range map[string]string{
		"//evil":                fallback,
		"//evil.example/x":      fallback,
		"/\t/evil.example":      fallback,
		"/\n/evil.example":      fallback,
		"/\x00/evil.example":    fallback,
		"/\x7f/evil.example":    fallback,
		"/%09/evil":             fallback,
		"/%2F/evil":             fallback,
		`/\evil`:                fallback,
		`/\\evil`:               fallback,
		"https://evil":          fallback,
		"http://evil/x":         fallback,
		"javascript:alert(1)":   fallback,
		"evil":                  fallback,
		"":                      fallback,
		"/login":                fallback,
		"/login?next=/projects": fallback,
		"/%zz":                  fallback,
		"/editor/prj_x?t=1":     "/editor/prj_x?t=1",
		"/projects?sort=name":   "/projects?sort=name",
		"/settings/keys#top":    "/settings/keys#top",
		"/projects/a%20b":       "/projects/a%20b",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}
