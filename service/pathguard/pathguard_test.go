package pathguard

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

func code(err error) string {
	if pe, ok := errors.AsType[*Error](err); ok {
		return pe.Code
	}

	return ""
}

// TestSecurity_T11_URLAndUNCPathsRefused: no scheme, UNC or device path reaches
// the operating system, whatever the host (SDD-007 T-11, section 7.2).
func TestSecurity_T11_URLAndUNCPathsRefused(t *testing.T) {
	cases := []string{
		"http://169.254.169.254/latest/meta-data",
		"https://example.com/a.mp4",
		"file:///etc/passwd",
		"smb://server/share/a.mp4",
		"ftp://host/a.mp4",
		"data:video/mp4;base64,AAAA",
		"concat:a.mp4|b.mp4",
		"subfile,,start,0,end,0,,:/etc/passwd",
		"pipe:0",
		"rtmp://host/app",
		"tcp://127.0.0.1:80",
		`\\evil\share\a.mp4`,
		`\\?\C:\x.mp4`,
		`\\?\UNC\evil\share\a.mp4`,
		`\\.\pipe\x`,
		`\\?\GLOBALROOT\Device\x`,
		`\??\C:\x.mp4`,
	}

	for _, goos := range []string{"windows", "linux"} {
		for _, p := range cases {
			if _, err := Clean(p, goos); code(err) != CodePathNotAllowed {
				t.Errorf("%s: Clean(%q) = %v, want E_PATH_NOT_ALLOWED", goos, p, err)
			}
		}
	}

	// On Unix "//host/share" is an ordinary path with a doubled separator.
	if _, err := Clean("//evil/share/a.mp4", "windows"); code(err) != CodePathNotAllowed {
		t.Errorf("windows: a // prefix was accepted: %v", err)
	}

	g, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range cases {
		f, err := g.OpenImport(p)
		if err == nil {
			_ = f.Close()
			t.Errorf("OpenImport(%q) opened a file", p)
		}
	}
}

// TestSecurity_T12_ImportPathCanonicalization holds the syntactic table of
// SDD-007 section 7.1 steps 1 to 6 and the symlink and junction cases.
func TestSecurity_T12_ImportPathCanonicalization(t *testing.T) {
	type tc struct{ goos, in, want string } // want "" means refused

	table := []tc{
		{"linux", "/media/a.mp4", "/media/a.mp4"},
		{"linux", "/media//./a.mp4", "/media/a.mp4"},
		{"linux", "/media/a\\b.mp4", "/media/a\\b.mp4"},
		{"linux", "/~/a.mp4", "/~/a.mp4"},
		{"linux", "~/a.mp4", ""},
		{"linux", "a.mp4", ""},
		{"linux", "./a.mp4", ""},
		{"linux", "/media/../etc/passwd", ""},
		{"linux", "/media/..", ""},
		{"linux", "/a\x00b", ""},
		{"linux", "/a\nb", ""},
		{"linux", "/a\x7fb", ""},
		{"linux", "", ""},
		{"linux", "/" + strings.Repeat("a", 4096), ""},
		{"linux", "/\xff\xfe", ""},
		{"windows", `C:\Media\a.mp4`, `C:\Media\a.mp4`},
		{"windows", `c:/Media//./a.mp4`, `C:\Media\a.mp4`},
		{"windows", `C:\Media\..\a.mp4`, ""},
		{"windows", `C:a.mp4`, ""},
		{"windows", `C:\a.mp4:secret`, ""},
		{"windows", `C:\a.mp4::$DATA`, ""},
		{"windows", `C:\a<b.mp4`, ""},
		{"windows", `C:\a>b.mp4`, ""},
		{"windows", `C:\a"b.mp4`, ""},
		{"windows", `C:\a|b.mp4`, ""},
		{"windows", `C:\a?b.mp4`, ""},
		{"windows", `C:\a*b.mp4`, ""},
		{"windows", `C:\dir.\a.mp4`, ""},
		{"windows", `C:\dir \a.mp4`, ""},
		{"windows", `C:\a.mp4.`, ""},
		{"windows", `C:\a.mp4 `, ""},
		{"windows", `C:\NUL.mp4`, ""},
		{"windows", `C:\nul`, ""},
		{"windows", `C:\dir\Con.txt`, ""},
		{"windows", `C:\COM1\a.mp4`, ""},
		{"windows", `C:\lpt9.x.y`, ""},
		{"windows", "C:\\COM\u00b9.mp4", ""},
		{"windows", `C:\CONIN$`, ""},
		{"windows", `C:\CONOUT$.mp4`, ""},
		{"windows", `C:\NUL .mp4`, ""},
		{"windows", `\Media\a.mp4`, ""},
		{"windows", `Media\a.mp4`, ""},
		{"windows", `/Media/a.mp4`, ""},
		{"windows", `C:\COM10.mp4`, `C:\COM10.mp4`},
		{"windows", `C:\console.mp4`, `C:\console.mp4`},
	}

	for _, c := range table {
		got, err := Clean(c.in, c.goos)
		switch {
		case c.want == "" && err == nil:
			t.Errorf("%s: Clean(%q) = %q, want a refusal", c.goos, c.in, got)
		case c.want != "" && (err != nil || got != c.want):
			t.Errorf("%s: Clean(%q) = %q, %v; want %q", c.goos, c.in, got, err, c.want)
		case err != nil && code(err) != CodePathNotAllowed:
			t.Errorf("%s: Clean(%q) code %s", c.goos, c.in, code(err))
		}

		if err != nil && strings.Contains(err.Error(), "secret") {
			t.Errorf("the refusal for %q names the path", c.in)
		}
	}
}

func TestSecurity_T12_ImportOpensAndRefuses(t *testing.T) {
	dir := realTemp(t)
	file := writeFile(t, filepath.Join(dir, "a.mp4"))

	g, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}

	f, err := g.OpenImport(file)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	_ = f.Close()

	if _, err := g.OpenImport(filepath.Join(dir, "missing.mp4")); code(err) != CodeFileNotFound {
		t.Errorf("missing file: %v, want E_FILE_NOT_FOUND in local mode", err)
	}

	if _, err := g.OpenImport(dir); code(err) != CodePathNotAllowed {
		t.Errorf("directory: %v, want E_PATH_NOT_ALLOWED", err)
	}

	if _, err := g.OpenImport(filepath.Join("relative", "a.mp4")); code(err) != CodePathNotAllowed {
		t.Errorf("relative: %v", err)
	}

	link := filepath.Join(dir, "link.mp4")
	if os.Symlink(file, link) == nil {
		f, err := g.OpenImport(link)
		if err != nil {
			t.Fatalf("symlink to a regular file is resolved in local mode: %v", err)
		}

		_ = f.Close()
	}
}

// TestSecurity_T12_ImportConfinedToRoots: with roots set a symlink or a junction
// that leaves the root is refused, a case variant of the root is accepted where
// the file system ignores case, and /media2 is not inside /media.
func TestSecurity_T12_ImportConfinedToRoots(t *testing.T) {
	base := realTemp(t)
	root := filepath.Join(base, "media")
	other := filepath.Join(base, "media2")
	outside := filepath.Join(base, "secret")

	for _, d := range []string{root, other, outside} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	inside := writeFile(t, filepath.Join(root, "ok.mp4"))
	writeFile(t, filepath.Join(other, "no.mp4"))
	secret := writeFile(t, filepath.Join(outside, "pw.mp4"))

	for _, network := range []bool{false, true} {
		g, err := New(Config{Roots: []string{root}, Network: network})
		if err != nil {
			t.Fatal(err)
		}

		f, err := g.OpenImport(inside)
		if err != nil {
			t.Fatalf("network=%v: inside the root: %v", network, err)
		}

		_ = f.Close()

		for name, p := range map[string]string{
			"sibling with a common prefix": filepath.Join(other, "no.mp4"),
			"outside":                      secret,
			"dotdot":                       root + string(filepath.Separator) + ".." + string(filepath.Separator) + "secret" + string(filepath.Separator) + "pw.mp4",
		} {
			if f, err := g.OpenImport(p); err == nil {
				_ = f.Close()
				t.Errorf("network=%v %s: opened", network, name)
			} else if network && code(err) != CodePathNotAllowed {
				t.Errorf("network=%v %s: %v, want E_PATH_NOT_ALLOWED", network, name, err)
			}
		}

		link := filepath.Join(root, "escape.mp4")
		if err := os.Symlink(secret, link); err == nil {
			if f, err := g.OpenImport(link); err == nil {
				_ = f.Close()
				t.Errorf("network=%v: a symlink out of the root was opened", network)
			}
		}

		dirLink := filepath.Join(root, "escdir")
		if makeDirLink(t, dirLink, outside) {
			if f, err := g.OpenImport(filepath.Join(dirLink, "pw.mp4")); err == nil {
				_ = f.Close()
				t.Errorf("network=%v: a directory link or junction out of the root was opened", network)
			}
		}

		_ = os.Remove(link)
		_ = os.Remove(dirLink)
	}

	// The case rule only applies on a case-insensitive volume. A macOS runner may
	// mount a case-sensitive one, where the variant names another path.
	if caseInsensitive(root, inside) && (runtime.GOOS == "windows" || runtime.GOOS == "darwin") {
		g, err := New(Config{Roots: []string{strings.ToUpper(root)}})
		if err != nil {
			t.Fatal(err)
		}

		f, err := g.OpenImport(strings.ToLower(inside))
		if err != nil {
			t.Fatalf("a case variant of the root: %v", err)
		}

		_ = f.Close()
	}
}

func TestSecurity_T12_NetworkModeWithoutRootsRefusesEverything(t *testing.T) {
	dir := realTemp(t)
	file := writeFile(t, filepath.Join(dir, "a.mp4"))

	g, err := New(Config{Network: true})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := g.OpenImport(file); code(err) != CodePathNotAllowed {
		t.Fatalf("got %v", err)
	}
}

func TestSecurity_T12_NetworkModeAnswersAreAlike(t *testing.T) {
	base := realTemp(t)
	root := filepath.Join(base, "media")

	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}

	g, err := New(Config{Roots: []string{root}, Network: true})
	if err != nil {
		t.Fatal(err)
	}

	_, missing := g.OpenImport(filepath.Join(root, "missing.mp4"))
	_, dir := g.OpenImport(root)
	_, out := g.OpenImport(base)

	if missing == nil || dir == nil || out == nil ||
		missing.Error() != dir.Error() || dir.Error() != out.Error() {
		t.Fatalf("answers differ: %v | %v | %v", missing, dir, out)
	}
}

func TestRootInsideWorkspaceIsRefused(t *testing.T) {
	ws := realTemp(t)
	sub := filepath.Join(ws, "store")

	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := New(Config{Workspace: ws, Roots: []string{sub}}); err == nil {
		t.Fatal("a root inside the workspace was accepted")
	}

	file := writeFile(t, filepath.Join(sub, "x.mp4"))

	g, err := New(Config{Workspace: ws})
	if err != nil {
		t.Fatal(err)
	}

	_, err = g.OpenImport(file)
	if code(err) != CodePathNotAllowed {
		t.Fatalf("a file inside the workspace: %v", err)
	}

	var pe *Error
	if !errors.As(err, &pe) || !strings.Contains(pe.Hint, "outside the workspace") {
		t.Fatalf("the hint does not say to copy the file out: %v", err)
	}
}

func FuzzCanonicalizeImportPath(f *testing.F) {
	for _, s := range []string{"/a/b", `C:\a\b`, "http://x", `\\a\b`, "/a/../b", "C:/a/./b", "", "/", "C:\\", "\x00"} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, in string) {
		for _, goos := range []string{"windows", "linux"} {
			out, err := Clean(in, goos)
			if err != nil {
				continue
			}

			if !utf8.ValidString(out) || strings.ContainsAny(out, "\x00\n\r\t") {
				t.Fatalf("%s: %q accepted as %q", goos, in, out)
			}

			for _, c := range components(out, goos) {
				if c == ".." || c == "." {
					t.Fatalf("%s: %q accepted with %q component", goos, in, c)
				}

				if goos == "windows" && (strings.ContainsAny(c, `:<>"|?*`) || isReserved(c) ||
					strings.HasSuffix(c, ".") || strings.HasSuffix(c, " ")) {
					t.Fatalf("windows: %q accepted with component %q", in, c)
				}
			}

			again, err := Clean(out, goos)
			if err != nil || again != out {
				t.Fatalf("%s: not idempotent: %q -> %q -> %q, %v", goos, in, out, again, err)
			}
		}
	})
}

func components(p, goos string) []string {
	if goos == "windows" {
		return strings.Split(p[3:], `\`)
	}

	return strings.Split(strings.TrimPrefix(p, "/"), "/")
}

func realTemp(t *testing.T) string {
	t.Helper()

	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	return d
}

func writeFile(t *testing.T, path string) string {
	t.Helper()

	if err := os.WriteFile(path, []byte("0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}

	return path
}

// makeDirLink makes link point at target: a symbolic link, or a junction on
// Windows where a symbolic link needs a privilege. It reports false when the
// platform refused both, which skips that case.
func makeDirLink(t *testing.T, link, target string) bool {
	t.Helper()

	if runtime.GOOS == "windows" {
		out, err := exec.CommandContext(t.Context(), "cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
		if err != nil {
			t.Logf("mklink /J: %v: %s", err, out)

			return false
		}

		return true
	}

	return os.Symlink(target, link) == nil
}

// caseInsensitive tells whether the volume of root resolves case variants of
// both paths to the same files.
func caseInsensitive(root, inside string) bool {
	_, e1 := os.Stat(strings.ToUpper(root))
	_, e2 := os.Stat(strings.ToLower(inside))

	return e1 == nil && e2 == nil
}

// The guard never puts a server path in an answer; only Roots lets the service show them to an administrator.
func TestNetworkRefusalNamesNoPath(t *testing.T) {
	base := realTemp(t)
	root := filepath.Join(base, "media")

	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}

	g, err := New(Config{Roots: []string{root}, Network: true})
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{filepath.Join(base, "other.mp4"), filepath.Join(root, "missing.mp4")} {
		_, err := g.OpenImport(path)

		if pe, ok := errors.AsType[*Error](err); !ok || strings.Contains(pe.Hint+pe.Message, base) {
			t.Errorf("answer %v names a server path", err)
		}
	}

	if got := g.Roots(); len(got) != 1 || !strings.EqualFold(got[0], root) {
		t.Errorf("Roots = %v, want [%s]", got, root)
	}
}

// VV-081: the files the server writes under exports/ and templates/ can be imported back without a copy,
// and nothing else of the workspace can.
func TestOpenExportedAcceptsOnlyTheFolderOfTheFilesTheServerWrote(t *testing.T) {
	ws := t.TempDir()
	write := func(parts ...string) string {
		p := filepath.Join(append([]string{ws}, parts...)...)
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}

		return p
	}

	bundle := write("exports", "prj_1", "bundle.vvbundle")
	tmpl := write("templates", "t.vvtemplate")
	secret := write("token")

	g, err := New(Config{Workspace: ws})
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{bundle, tmpl} {
		f, err := g.OpenExported(p)
		if err != nil {
			t.Fatalf("OpenExported(%s) = %v", p, err)
		}

		_ = f.Close()

		if _, err := g.OpenImport(p); code(err) != CodePathNotAllowed {
			t.Errorf("OpenImport(%s) = %v, a medium must still be refused", p, err)
		}
	}

	if _, err := g.OpenExported(secret); code(err) != CodePathNotAllowed {
		t.Errorf("OpenExported(token) = %v, want E_PATH_NOT_ALLOWED", err)
	}

	net, err := New(Config{Workspace: ws, Network: true, Roots: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := net.OpenExported(bundle); code(err) != CodePathNotAllowed {
		t.Errorf("network mode: %v, want E_PATH_NOT_ALLOWED", err)
	}
}
