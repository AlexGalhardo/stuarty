package destinations

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/social"
)

func filesSecret(t *testing.T, folder string, extra map[string]any) string {
	t.Helper()

	m := map[string]any{"folder": folder}
	maps.Copy(m, extra)

	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}

	return string(b)
}

func sendFiles(t *testing.T, folder string, extra map[string]any, p Package, data map[string][]byte) (Result, error) {
	t.Helper()

	return newFiles(testConfig()).Send(context.Background(), &SendInput{
		Cred: cred("http://localhost", filesSecret(t, folder, extra), nil), Package: p, Open: opener(data),
		Delivery: "tgt_0123456789",
	})
}

func read(t *testing.T, path string) []byte {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return b
}

// A post with a cover and two images is written as a folder that a static site generator reads: the front matter
// ends where it should and every image path in the body and in the front matter is a file next to index.md.
func TestFilesRoundTrip(t *testing.T) {
	t.Parallel()

	folder := t.TempDir()
	p, data := testPost(t)

	res, err := sendFiles(t, folder, nil, p, data)
	if err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(folder, "how-i-cut-a-video")
	if res.RemoteID != "how-i-cut-a-video" || res.URL != dir || res.State != StatusPublished {
		t.Errorf("result = %+v", res)
	}

	index := string(read(t, filepath.Join(dir, "index.md")))

	fm, body, ok := strings.Cut(strings.TrimPrefix(index, "---\n"), "\n---\n")
	if !ok || !strings.HasPrefix(index, "---\n") {
		t.Fatalf("index.md is not front matter and body:\n%s", index)
	}

	for _, want := range []string{
		`title: "How I cut a video"`, `description: "A short one."`, `pubDate: 2026-11-20T12:00:00Z`,
		`heroImage: "images/cover.png"`, `tags: ["video", "editing"]`, `lang: "en"`, "draft: false",
	} {
		contains(t, fm, want)
	}

	// Every path the page mentions is a file with the bytes that were approved.
	refs := regexp.MustCompile(`images/[a-z0-9.-]+\.png`).FindAllString(fm+"\n"+body, -1)
	if len(refs) != 3 {
		t.Fatalf("the page mentions %d images, want 3:\n%s", len(refs), index)
	}

	for _, ref := range refs {
		got := read(t, filepath.Join(dir, filepath.FromSlash(ref)))
		if !bytes.Equal(got, data[filepath.Base(ref)]) {
			t.Errorf("%s differs from the approved file", ref)
		}
	}

	contains(t, body, "Intro with *style*.")
	contains(t, body, "![the desk](images/1-the-desk.png)")

	entries, _ := os.ReadDir(folder)
	if len(entries) != 1 {
		t.Errorf("the folder holds %d entries, want only the post: %v", len(entries), entries)
	}
}

func TestFilesTemplates(t *testing.T) {
	t.Parallel()

	p, data := testPost(t)
	p.Status = StatusDraft

	cases := map[string]struct {
		extra map[string]any
		want  []string
	}{
		"hugo": {
			map[string]any{"template": "hugo"},
			[]string{
				"draft: true",
				`slug: "how-i-cut-a-video"`,
				"cover:\n  image: \"images/cover.png\"",
				`canonicalURL: "https://example.com/blog/how-i-cut-a-video"`,
			},
		},
		"jekyll": {
			map[string]any{"template": "jekyll"},
			[]string{"layout: post", "published: false", `excerpt: "A short one."`},
		},
		"custom": {
			map[string]any{"template": "custom", "front_matter": "+++\nname = {{title}}\nimg = {{cover}}\n+++"},
			[]string{"+++\nname = \"How I cut a video\"\nimg = \"images/cover.png\"\n+++\n"},
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			folder := t.TempDir()
			if _, err := sendFiles(t, folder, c.extra, p, data); err != nil {
				t.Fatal(err)
			}

			index := string(read(t, filepath.Join(folder, p.Slug, "index.md")))
			for _, w := range c.want {
				contains(t, index, w)
			}
		})
	}
}

// A second send replaces the first one: nothing of the old version stays, and no work folder is left.
func TestFilesSecondSendReplaces(t *testing.T) {
	t.Parallel()

	folder := t.TempDir()
	p, data := testPost(t)

	if _, err := sendFiles(t, folder, nil, p, data); err != nil {
		t.Fatal(err)
	}

	// The second version has one image less.
	p.Images = p.Images[:2]
	p.Markdown = "New text.\n"

	if _, err := sendFiles(t, folder, nil, p, data); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(folder, p.Slug)
	if _, err := os.Stat(filepath.Join(dir, "images", "2-the-cut.png")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("an image of the old version stayed: %v", err)
	}

	contains(t, string(read(t, filepath.Join(dir, "index.md"))), "New text.")

	if entries, _ := os.ReadDir(folder); len(entries) != 1 {
		t.Errorf("work folders were left: %v", entries)
	}
}

func TestFilesZip(t *testing.T) {
	t.Parallel()

	folder := t.TempDir()
	p, data := testPost(t)

	res, err := sendFiles(t, folder, map[string]any{"zip": true}, p, data)
	if err != nil {
		t.Fatal(err)
	}

	if res.URL != filepath.Join(folder, p.Slug+".zip") {
		t.Errorf("url = %q", res.URL)
	}

	zr, err := zip.OpenReader(res.URL)
	if err != nil {
		t.Fatal(err)
	}

	got := map[string][]byte{}

	for _, f := range zr.File {
		rc, oerr := f.Open()
		if oerr != nil {
			t.Fatal(oerr)
		}

		got[f.Name], _ = io.ReadAll(rc)
		_ = rc.Close()
	}

	// Windows cannot replace a file that is open.
	_ = zr.Close()

	if len(got) != 4 || !bytes.Equal(got["how-i-cut-a-video/images/cover.png"], data["cover.png"]) ||
		!strings.Contains(string(got["how-i-cut-a-video/index.md"]), "heroImage") {
		t.Errorf("the zip holds %d entries: %v", len(got), keys(got))
	}

	if entries, _ := os.ReadDir(folder); len(entries) != 1 {
		t.Errorf("the folder holds more than the zip: %v", entries)
	}

	// Sent again, the zip is replaced.
	if _, err := sendFiles(t, folder, map[string]any{"zip": true}, p, data); err != nil {
		t.Fatalf("second send: %v", err)
	}
}

func keys[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}

	return out
}

func TestFilesRefusals(t *testing.T) {
	t.Parallel()

	p, data := testPost(t)

	kindOf := func(err error) string {
		se, ok := social.AsError(err)
		if !ok {
			return "not a social error: " + errString(err)
		}

		return se.Kind + "/" + se.Reason
	}

	t.Run("folder missing", func(t *testing.T) {
		t.Parallel()

		_, err := sendFiles(t, filepath.Join(t.TempDir(), "nope"), nil, p, data)
		if got := kindOf(err); got != "invalid/folder_missing" {
			t.Errorf("got %s", got)
		}
	})

	t.Run("relative folder", func(t *testing.T) {
		t.Parallel()

		if _, err := sendFiles(t, "relative/path", nil, p, data); kindOf(err) != "invalid/bad_settings" {
			t.Errorf("got %s", kindOf(err))
		}
	})

	t.Run("unknown member", func(t *testing.T) {
		t.Parallel()

		if _, err := sendFiles(
			t,
			t.TempDir(),
			map[string]any{"chmod": 777},
			p,
			data,
		); kindOf(
			err,
		) != "invalid/bad_settings" {
			t.Errorf("got %s", kindOf(err))
		}
	})

	t.Run("a file where the post goes", func(t *testing.T) {
		t.Parallel()

		folder := t.TempDir()
		if err := os.WriteFile(filepath.Join(folder, p.Slug), []byte("mine"), 0o600); err != nil {
			t.Fatal(err)
		}

		if _, err := sendFiles(t, folder, nil, p, data); kindOf(err) != "invalid/not_a_plain_target" {
			t.Errorf("got %s", kindOf(err))
		}

		if string(read(t, filepath.Join(folder, p.Slug))) != "mine" {
			t.Error("the file was replaced")
		}
	})

	t.Run("an image that changed keeps the old version", func(t *testing.T) {
		t.Parallel()

		folder := t.TempDir()
		if _, err := sendFiles(t, folder, nil, p, data); err != nil {
			t.Fatal(err)
		}

		bad := map[string][]byte{}
		for k, v := range data {
			bad[k] = append([]byte(nil), v...)
		}

		bad["2-the-cut.png"][0] ^= 0xff

		p2 := p
		p2.Markdown = "changed\n"

		if _, err := sendFiles(t, folder, nil, p2, bad); kindOf(err) != "invalid/write_failed" {
			t.Errorf("got %s", kindOf(err))
		}

		contains(t, string(read(t, filepath.Join(folder, p.Slug, "index.md"))), "Intro with")

		if entries, _ := os.ReadDir(folder); len(entries) != 1 {
			t.Errorf("work folders were left: %v", entries)
		}
	})

	t.Run("a bad template", func(t *testing.T) {
		t.Parallel()

		_, err := sendFiles(t, t.TempDir(), map[string]any{"template": "custom", "front_matter": "{{body}}"}, p, data)
		if kindOf(err) != "invalid/bad_settings" {
			t.Errorf("got %s", kindOf(err))
		}
	})
}

func errString(err error) string {
	if err == nil {
		return "nil"
	}

	return err.Error()
}

func TestFilesConfig(t *testing.T) {
	t.Parallel()

	f := newFiles(testConfig())
	dir := t.TempDir()

	if err := f.ValidateConfig([]byte(filesSecret(t, dir, nil)), "http://localhost"); err != nil {
		t.Errorf("a good folder: %v", err)
	}

	for name, secret := range map[string]string{
		"missing":  filesSecret(t, filepath.Join(dir, "nope"), nil),
		"relative": `{"folder":"x"}`,
		"unknown":  `{"folder":"` + strings.ReplaceAll(dir, `\`, `\\`) + `","x":1}`,
		"text":     `not json`,
		"trailing": `{"folder":"x"} {}`,
	} {
		if err := f.ValidateConfig([]byte(secret), ""); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	res, err := f.Test(context.Background(), cred("http://localhost", filesSecret(t, dir, nil), nil))
	if err != nil || !res.OK {
		t.Errorf("test = %+v, %v", res, err)
	}

	res, _ = f.Test(
		context.Background(),
		cred("http://localhost", filesSecret(t, filepath.Join(dir, "nope"), nil), nil),
	)
	if res.OK {
		t.Error("a missing folder tested ok")
	}
}

// T-99: nothing a post names can make the destination write outside the folder of its connection.
func TestSecurity_T99_FilesDestinationStaysInItsFolder(t *testing.T) {
	t.Parallel()

	p, data := testPost(t)
	outside := t.TempDir()
	folder := filepath.Join(t.TempDir(), "site")

	if err := os.Mkdir(folder, 0o750); err != nil {
		t.Fatal(err)
	}

	hostile := []string{
		"..", "../" + filepath.Base(outside), "a/b", `a\b`, "..\\x", "", ".", "A", "-a", "a--b", "a b", "con:x",
		strings.Repeat("a", 81), "a\x00b", "/abs",
	}

	for _, slug := range hostile {
		q := p
		q.Slug = slug

		if _, err := sendFiles(t, folder, nil, q, data); err == nil {
			t.Errorf("slug %q was written", slug)
		}
	}

	for _, name := range []string{"../evil.png", "a/b.png", "..", "x.exe", "a.png.exe", "", "A.png", ".hidden.png", `a\b.png`} {
		q := p
		q.Images = append([]Image(nil), p.Images...)
		q.Images[1].Name = name

		if _, err := sendFiles(t, folder, nil, q, data); err == nil {
			t.Errorf("image name %q was written", name)
		}
	}

	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("something was written outside the folder: %v", entries)
	}

	if entries, _ := os.ReadDir(folder); len(entries) != 0 {
		t.Errorf("something was left in the folder after refusals: %v", entries)
	}

	// A slug folder that is a symbolic link to another place is refused and the other place is not touched.
	if err := os.Symlink(outside, filepath.Join(folder, p.Slug)); err != nil {
		// skip-env: symlink-privilege
		t.Skipf("no symlink privilege on this machine; the link case is not run: %v", err)
	}

	if _, err := sendFiles(t, folder, nil, p, data); err == nil {
		t.Error("a post was written through a symbolic link")
	}

	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("a symbolic link let the post out: %v", entries)
	}
}
