// Package security_test holds the UI side of threat T-22 (SDD-007 section 8.5,
// ROADMAP P2-SEC-005): stored text is shown as text, never as HTML.
package security_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
)

// rawHTMLSinks are the ways to put a string into the DOM as markup.
var rawHTMLSinks = regexp.MustCompile(`innerHTML|outerHTML|insertAdjacentHTML|document\.write|dangerouslySetInnerHTML`)

// sinkAllowlist names the files (slash paths relative to web/resources/js)
// that may mention a sink. It is empty: the editor draws text as React text
// children, and a new entry needs a review of SDD-007 section 8.5.
var sinkAllowlist = map[string]bool{}

func webDir(t *testing.T) string {
	t.Helper()

	dir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestSecurity_T22_NoRawHTMLSinks(t *testing.T) {
	root := filepath.Join(webDir(t), "resources", "js")
	scanned := 0

	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		switch filepath.Ext(path) {
		case ".ts", ".tsx", ".js", ".jsx", ".mjs":
		default:
			return nil
		}

		if entry.IsDir() {
			return nil
		}

		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		scanned++

		if sinkAllowlist[rel] {
			return nil
		}

		for i, line := range strings.Split(string(body), "\n") {
			if m := rawHTMLSinks.FindString(line); m != "" {
				t.Errorf("%s:%d uses the raw HTML sink %q", rel, i+1, m)
			}
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if scanned < 50 {
		t.Fatalf("scanned %d files under %s: the scan does not see the sources", scanned, root)
	}
}

func buildAndStart(t *testing.T) (base, token string) {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "app")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	build := exec.CommandContext(ctx, "go", "build", "-o", bin, "github.com/AlexGalhardo/stuarty/web/cmd/app")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	addr := listener.Addr().String()
	_ = listener.Close()

	home := filepath.Join(t.TempDir(), "ws")

	cmd := exec.CommandContext(t.Context(), bin, "serve")
	cmd.Env = append(os.Environ(),
		"STUARTY_HOME="+home, "STUARTY_MIN_FREE_BYTES=1",
		"HOST=127.0.0.1", "PORT="+strings.TrimPrefix(addr, "127.0.0.1:"),
		"APP_KEY=base64:"+base64.StdEncoding.EncodeToString(bytes.Repeat([]byte("k"), 32)),
	)

	var out bytes.Buffer

	cmd.Stdout, cmd.Stderr = &out, &out

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})

	go func() { _ = cmd.Wait(); close(done) }()

	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-done
	})

	base = "http://" + addr

	for deadline := time.Now().Add(60 * time.Second); ; time.Sleep(200 * time.Millisecond) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/up", http.NoBody)
		if err != nil {
			t.Fatal(err)
		}

		resp, err := outbound.LocalServer(5 * time.Second).Do(req)
		if err == nil {
			_ = resp.Body.Close()

			break
		}

		if time.Now().After(deadline) {
			t.Fatalf("the server never answered: %v", err)
		}
	}

	raw, err := os.ReadFile(filepath.Join(home, "token"))
	if err != nil {
		t.Fatal(err)
	}

	return base, strings.TrimSpace(string(raw))
}

var bannedSources = map[string]bool{"'unsafe-inline'": true, "'unsafe-eval'": true, "blob:": true, "*": true}

var nonceOf = regexp.MustCompile(`'nonce-([A-Za-z0-9_-]{16,})'`)

func TestSecurity_T22_CSPNonceOnly(t *testing.T) {
	base, token := buildAndStart(t)

	get := func(path string, auth bool) (http.Header, string) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, base+path, http.NoBody)
		if err != nil {
			t.Fatal(err)
		}

		if auth {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err := outbound.LocalServer(10 * time.Second).Do(req)
		if err != nil {
			t.Fatal(err)
		}

		defer func() { _ = resp.Body.Close() }()

		body, _ := io.ReadAll(resp.Body)

		return resp.Header, string(body)
	}

	nonces := map[string]bool{}

	for _, c := range []struct {
		path string
		auth bool
	}{{"/projects", true}, {"/projects", false}, {"/up", false}} {
		header, body := get(c.path, c.auth)
		policy := header.Get("Content-Security-Policy")

		if policy == "" {
			t.Fatalf("%s (auth %v) carries no Content-Security-Policy", c.path, c.auth)
		}

		for directive := range strings.SplitSeq(policy, ";") {
			name, value, _ := strings.Cut(strings.TrimSpace(directive), " ")
			if name != "script-src" && name != "style-src" && name != "default-src" {
				continue
			}

			for source := range strings.FieldsSeq(value) {
				if bannedSources[source] {
					t.Errorf("%s: %s allows %s: %s", c.path, name, source, policy)
				}
			}
		}

		var script string

		for directive := range strings.SplitSeq(policy, ";") {
			if strings.HasPrefix(strings.TrimSpace(directive), "script-src ") {
				script = strings.TrimSpace(directive)
			}
		}

		match := nonceOf.FindStringSubmatch(script)
		if len(match) < 2 || script != "script-src 'self' 'nonce-"+match[1]+"'" {
			t.Fatalf("%s: script-src is not 'self' plus one nonce: %q", c.path, script)
		}

		if !strings.Contains("; "+policy+";", "; worker-src 'self';") {
			t.Errorf("%s: worker-src is not exactly 'self': %s", c.path, policy)
		}

		nonces[match[1]] = true

		// Every script the document carries bears the nonce of its own response.
		for _, tag := range regexp.MustCompile(`<script\b[^>]*>`).FindAllString(body, -1) {
			if strings.Contains(tag, `type="application/json"`) {
				continue
			}

			if !strings.Contains(tag, `nonce="`+match[1]+`"`) {
				t.Errorf("%s: script tag without the nonce of the response: %s", c.path, tag)
			}
		}
	}

	if len(nonces) != 3 {
		t.Errorf("the nonce is not fresh per request: %d distinct in 3 responses", len(nonces))
	}
}

// TestSecurity_T22_UIRendersTextNotHTML runs the Playwright spec
// web/e2e/specs/security-ui.spec.ts in headless Chromium. It skips, with the reason,
// where bun or the browser is not installed.
func TestSecurity_T22_UIRendersTextNotHTML(t *testing.T) {
	if testing.Short() {
		// skip-env: short-mode
		t.Skip("the browser test is not run with -short")
	}

	bun, err := exec.LookPath("bun")
	if err != nil {
		// skip-env: browser
		t.Skip("bun is not on PATH: the Playwright spec security-ui.spec.ts did not run")
	}

	dir := webDir(t)
	if _, err := os.Stat(filepath.Join(dir, "node_modules", "@playwright", "test")); err != nil {
		// skip-env: browser
		t.Skip(
			"Playwright is not installed in web/node_modules (run bun install): the spec security-ui.spec.ts did not run",
		)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, bun, "run", "e2e", "--", "security-ui")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "STUARTY_MIN_FREE_BYTES=1")

	out, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "Executable doesn't exist") {
			// skip-env: browser
			t.Skip("the Playwright browser is not installed (bunx playwright install chromium): the spec did not run")
		}

		t.Fatalf("security-ui.spec.ts failed: %v\n%s", err, out)
	}
}
