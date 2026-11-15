package shell

import "testing"

func TestAllowed(t *testing.T) {
	const port = 3101
	tests := []struct {
		url  string
		want bool
	}{
		{"http://127.0.0.1:3101/projects", true},
		{"http://127.0.0.1:3101", true},
		{"http://localhost:3101/editor/p_1?x=1#y", true},
		{"https://example.com", false},
		{"https://example.com/", false},
		{"https://127.0.0.1:3101/", false},
		{"http://127.0.0.1:3102/", false},
		{"http://127.0.0.1/", false},
		{"http://127.0.0.2:3101/", false},
		{"http://[::1]:3101/", false},
		{"http://0.0.0.0:3101/", false},
		{"http://localhost.evil.com:3101/", false},
		{"http://127.0.0.1:3101@evil.com/", false},
		{"http://evil.com@127.0.0.1:3101/", false},
		{"http://127.0.0.1:3101.evil.com/", false},
		{"file:///etc/passwd", false},
		{"file://127.0.0.1:3101/x", false},
		{"data:text/html,<p>x</p>", false},
		{"blob:http://127.0.0.1:3101/abc", false},
		{"javascript:alert(1)", false},
		{"ws://127.0.0.1:3101/", false},
		{"about:blank", false},
		{"//127.0.0.1:3101/", false},
		{"/projects", false},
		{"", false},
		{"http://", false},
		{"http://127.0.0.1:3101:80/", false},
	}
	for _, tc := range tests {
		if got := Allowed(tc.url, port); got != tc.want {
			t.Errorf("Allowed(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

func TestAllowedRefusesAnInvalidPort(t *testing.T) {
	if Allowed("http://127.0.0.1:0/", 0) || Allowed("http://127.0.0.1:3101/", -1) {
		t.Error("a non-positive server port must allow nothing")
	}
}
