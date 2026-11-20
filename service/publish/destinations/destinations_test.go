package destinations

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/writing"
	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/connections"
)

var fixedNow = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

const (
	coverID = "ast_0123456789"
	oneID   = "ast_1111111111"
	twoID   = "ast_2222222222"
)

// image makes an image with some bytes and its digest.
func image(name, role, id string, size int) (Image, []byte) {
	data := bytes.Repeat([]byte{byte(len(name))}, size)
	sum := sha256.Sum256(data)

	return Image{
		Name: name, Role: role, AssetID: id, Hash: "sha256:" + id, ContentType: "image/png", Bytes: int64(size),
		SHA256: hex.EncodeToString(sum[:]),
	}, data
}

// testPost makes the package of a post with a cover and two images, the way the service builds one, and the files by
// image name.
func testPost(t *testing.T) (Package, map[string][]byte) {
	t.Helper()

	body := "---\ntitle: How I cut a video\nslug: how-i-cut-a-video\nsummary: A short one.\ntags: video, editing\n" +
		"cover: " + coverID + "\ncanonical: https://example.com/blog/how-i-cut-a-video\nlanguage: en\n---\n" +
		"Intro with *style*.\n\n![the desk](asset:" + oneID + ")\n\nMore text.\n\n![the cut](asset:" + twoID + ")\n"

	post, issues := writing.CheckPost(body, []string{coverID, oneID, twoID})
	if len(issues) != 0 {
		t.Fatalf("the fixture is not a good post: %v", issues)
	}

	cover, coverData := image("cover.png", "cover", coverID, 300)
	one, oneData := image("1-the-desk.png", "body", oneID, 500)
	two, twoData := image("2-the-cut.png", "body", twoID, 700)

	names := map[string]string{coverID: cover.Name, oneID: one.Name, twoID: two.Name}
	to := func(id string) (string, bool) { n, ok := names[id]; return "images/" + n, ok }

	md := writing.RewriteImages(post.Markdown, to)

	return Package{
		Slug: "how-i-cut-a-video", Status: StatusPublished, Meta: post.Meta, Markdown: md,
		HTML:   writing.RenderHTML(post.Markdown, func(id string) (string, bool) { return to(id) }),
		Images: []Image{cover, one, two},
	}, map[string][]byte{
		cover.Name: coverData, one.Name: oneData, two.Name: twoData,
	}
}

func opener(files map[string][]byte) func(Image) (io.ReadCloser, error) {
	return func(im Image) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(files[im.Name])), nil
	}
}

func cred(endpoint string, secret string, client connections.Doer) connections.Credential {
	return connections.Credential{
		Connection: connections.Connection{ID: "con_0123456789", Endpoint: endpoint, Hosts: []string{"localhost"}},
		Secret:     []byte(secret), Client: client,
	}
}

// gateLike is a client that follows no redirect, as the client of a connection does, and reaches loopback only.
func gateLike() connections.Doer { return outbound.LocalServer(5 * time.Second) }

func noSleep(context.Context, time.Duration) error { return nil }

func testConfig() Config { return Config{Now: func() time.Time { return fixedNow }, Sleep: noSleep} }

func contains(t *testing.T, got, want string) {
	t.Helper()

	if !strings.Contains(got, want) {
		t.Errorf("%q does not contain %q", got, want)
	}
}
