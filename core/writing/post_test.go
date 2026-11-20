package writing

import (
	"strings"
	"testing"
	"time"
)

const goodAsset = "ast_0123456789"

// bidi is a right-to-left override, which a title and an address may not hold.
var bidi = string(rune(0x202e))

func front(lines ...string) string { return "---\n" + strings.Join(lines, "\n") + "\n---\n" }

func issueCodes(issues []PostIssue) map[string]string {
	m := map[string]string{}
	for _, i := range issues {
		m[i.Path] = i.Code
	}

	return m
}

func TestPostParse(t *testing.T) {
	t.Parallel()

	body := front("title: \"A \\\"quoted\\\" title\"", "slug: a-title", "summary: one line", "tags: [go, \"web dev\"]",
		"cover: "+goodAsset, "canonical: https://example.com/a", "language: pt-BR") + "Body\r\nmore\n"

	p, issues := ParsePost(body)
	if len(issues) != 0 {
		t.Fatalf("issues = %v", issues)
	}

	want := PostMeta{
		Title: `A "quoted" title`, Slug: "a-title", Summary: "one line", Tags: []string{"go", "web dev"},
		Cover: goodAsset, Canonical: "https://example.com/a", Language: "pt-BR",
	}

	if p.Meta.Title != want.Title || p.Meta.Slug != want.Slug || p.Meta.Summary != want.Summary ||
		strings.Join(p.Meta.Tags, "|") != "go|web dev" || p.Meta.Cover != want.Cover ||
		p.Meta.Canonical != want.Canonical || p.Meta.Language != want.Language {
		t.Errorf("meta = %+v, want %+v", p.Meta, want)
	}

	if p.Markdown != "Body\nmore\n" || !p.HasFrontMatter {
		t.Errorf("markdown = %q front = %v", p.Markdown, p.HasFrontMatter)
	}
}

func TestPostParseProblems(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		body string
		path string
	}{
		"never closed":  {"---\ntitle: x\nbody", "front_matter"},
		"not key value": {front("title: x", "what is this"), "front_matter"},
		"unknown key":   {front("title: x", "author: me"), "front_matter.author"},
		"twice":         {front("title: x", "title: y"), "title"},
		"open quote":    {front(`title: "x`), "title"},
		"inner quote":   {front(`title: "x"y"`), "title"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, issues := ParsePost(c.body)
			if _, ok := issueCodes(issues)[c.path]; !ok {
				t.Errorf("no issue on %s: %v", c.path, issues)
			}
		})
	}

	if p, issues := ParsePost("just text"); p.HasFrontMatter || len(issues) != 0 || p.Markdown != "just text" {
		t.Errorf("no front matter: %+v %v", p, issues)
	}
}

func TestPostSlugify(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"How I cut a video in a day": "how-i-cut-a-video-in-a-day",
		"Café  com  Leite!":          "cafe-com-leite",
		"  --Hello--  ":              "hello",
		"日本語":                        "",
		"a/b\\c..d":                  "a-b-c-d",
		strings.Repeat("ab ", 60):    strings.TrimRight(strings.Repeat("ab-", 60)[:MaxPostSlug], "-"),
	}

	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPostCheck(t *testing.T) {
	t.Parallel()

	images := []string{goodAsset}
	ok := front("title: Hello", "tags: a, b", "cover: "+goodAsset, "language: en") +
		"Text ![pic](asset:" + goodAsset + ")\n"

	if _, issues := CheckPost(ok, images); len(issues) != 0 {
		t.Fatalf("a good post has issues: %v", issues)
	}

	cases := map[string]struct {
		body, path, code string
	}{
		"no front matter": {"text only", "title", CodeRequired},
		"no title":        {front("slug: x") + "t", "title", CodeRequired},
		"title too long":  {front("title: "+strings.Repeat("a", 201)) + "t", "title", CodeTooLong},
		"title bidi":      {front("title: a"+bidi+"b") + "t", "title", CodeInvalid},
		"slug caps":       {front("title: t", "slug: Bad_Slug") + "t", "slug", CodeInvalid},
		"slug hyphens":    {front("title: t", "slug: -a--b") + "t", "slug", CodeInvalid},
		"slug long":       {front("title: t", "slug: "+strings.Repeat("a", 81)) + "t", "slug", CodeTooLong},
		"slug empty":      {front("title: ---") + "t", "slug", CodeRequired},
		"summary long":    {front("title: t", "summary: "+strings.Repeat("a", 501)) + "t", "summary", CodeTooLong},
		"too many tags": {
			front("title: t", "tags: a1,a2,a3,a4,a5,a6,a7,a8,a9,b1,b2,b3,b4,b5,b6,b7,b8,b9,c1,c2,c3") + "t",
			"tags",
			CodeTooMany,
		},
		"tag twice":      {front("title: t", "tags: Go, go") + "t", "tags", CodeInvalid},
		"tag long":       {front("title: t", "tags: "+strings.Repeat("a", 41)) + "t", "tags", CodeTooLong},
		"cover shape":    {front("title: t", "cover: nope") + "t", "cover", CodeInvalid},
		"cover unknown":  {front("title: t", "cover: ast_abcdefghjk") + "t", "cover", CodeUnknownAsset},
		"canonical http": {front("title: t", "canonical: http://example.com") + "t", "canonical", CodeInvalid},
		"canonical user": {front("title: t", "canonical: https://u:p@example.com") + "t", "canonical", CodeInvalid},
		"language":       {front("title: t", "language: english_us") + "t", "language", CodeInvalid},
		"empty body":     {front("title: t"), "body", CodeRequired},
		"external image": {front("title: t") + "![x](https://evil.example/a.png)", "body.images", CodeExternalImage},
		"data image":     {front("title: t") + "![x](data:image/png;base64,AAAA)", "body.images", CodeExternalImage},
		"unknown image":  {front("title: t") + "![x](asset:ast_abcdefghjk)", "body.images", CodeUnknownAsset},
		"bad image id":   {front("title: t") + "![x](asset:../x)", "body.images", CodeInvalid},
		"too big":        {front("title: t") + strings.Repeat("a", MaxPostBytes), "body", CodeTooLong},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, issues := CheckPost(c.body, images)
			if got := issueCodes(issues)[c.path]; got != c.code {
				t.Errorf("%s = %q, want %q (all: %v)", c.path, got, c.code, issues)
			}
		})
	}

	var many strings.Builder

	many.WriteString(front("title: t"))

	for i := range 21 {
		many.WriteString("![x](asset:ast_" + strings.Repeat(string("abcdefghjkmnpqrstvwxyz"[i]), 10) + ")\n")
	}

	long := many.String()

	if _, issues := CheckPost(long, nil); issueCodes(issues)["body.images"] != CodeTooMany {
		t.Errorf("more than 20 images: %v", issues)
	}

	if _, issues := CheckPost("---\ntitle: t\n---\n![x](asset:"+goodAsset+")", nil); len(issues) != 0 {
		t.Errorf("an unchecked image id was refused: %v", issues)
	}
}

func TestPostImagesAreFoundOutsideFences(t *testing.T) {
	t.Parallel()

	md := "![a](asset:ast_aaaaaaaaaa)\n```\n![b](asset:ast_bbbbbbbbbb)\n```\n![c](asset:ast_cccccccccc) ![a](asset:ast_aaaaaaaaaa)"

	got := strings.Join(Images(md, "ast_zzzzzzzzzz"), " ")
	if got != "ast_zzzzzzzzzz ast_aaaaaaaaaa ast_cccccccccc" {
		t.Errorf("Images = %q", got)
	}

	out := RewriteImages(md, func(id string) (string, bool) { return "images/" + id + ".png", id != "ast_cccccccccc" })
	if !strings.Contains(out, "![a](images/ast_aaaaaaaaaa.png)") ||
		!strings.Contains(out, "![b](asset:ast_bbbbbbbbbb)") ||
		!strings.Contains(out, "![c](asset:ast_cccccccccc)") {
		t.Errorf("RewriteImages = %q", out)
	}
}

func TestPostRenderHTML(t *testing.T) {
	t.Parallel()

	src := func(id string) (string, bool) { return "images/" + id + ".png", id == goodAsset }

	cases := map[string]struct{ in, want string }{
		"paragraph": {
			"Hello *there* **you** `x<y`",
			"<p>Hello <em>there</em> <strong>you</strong> <code>x&lt;y</code></p>\n",
		},
		"heading": {"## Title ##", "<h2>Title</h2>\n"},
		"hr":      {"---", "<hr>\n"},
		"lists": {
			"- a\n- b\n\n1. one\n2. two",
			"<ul>\n<li>a</li>\n<li>b</li>\n</ul>\n<ol>\n<li>one</li>\n<li>two</li>\n</ol>\n",
		},
		"quote": {"> q1\n> q2\n>\n> q3", "<blockquote>\n<p>q1\nq2</p>\n<p>q3</p>\n</blockquote>\n"},
		"fence": {"```go\n<b>x</b>\n```", "<pre><code>&lt;b&gt;x&lt;/b&gt;</code></pre>\n"},
		"link": {
			"[a](https://example.com/?a=1&b=2)",
			`<p><a href="https://example.com/?a=1&amp;b=2" rel="noopener noreferrer">a</a></p>` + "\n",
		},
		"image": {
			"![the \"desk\"](asset:" + goodAsset + ")",
			`<p><img src="images/` + goodAsset + `.png" alt="the &#34;desk&#34;"></p>` + "\n",
		},
		"image gone":          {"![alt](asset:ast_abcdefghjk)", "<p>alt</p>\n"},
		"escape":              {`\*not em\* \<b\>`, "<p>*not em* &lt;b&gt;</p>\n"},
		"escaped closer":      {`*a\*b*`, "<p><em>a*b</em></p>\n"},
		"space before closer": {"*a *b*", "<p><em>a *b</em></p>\n"},
		"unclosed marks":      {"[a *b [c](x", "<p>[a *b [c](x</p>\n"},
		"nested link": {
			"[[a](https://a.example)](https://b.example)",
			`<p><a href="https://b.example" rel="noopener noreferrer"><a href="https://a.example" rel="noopener noreferrer">a</a></a></p>` + "\n",
		},
		"links past the depth stay text": {
			"[[[[[a](/1)](/2)](/3)](/4)](/5)",
			`<p><a href="/5" rel="noopener noreferrer"><a href="/4" rel="noopener noreferrer"><a href="/3" rel="noopener noreferrer"><a href="/2" rel="noopener noreferrer">[a](/1)</a></a></a></a></p>` + "\n",
		},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := RenderHTML(c.in, src); got != c.want {
				t.Errorf("got  %q\nwant %q", got, c.want)
			}
		})
	}
}

// A hostile post never becomes markup: every character is escaped and the only tags are the renderer's own.
func TestSecurity_T88_HostilePostRendersAsText(t *testing.T) {
	t.Parallel()

	hostile := []string{
		"<script>alert(1)</script>",
		`<img src=x onerror=alert(1)>`,
		`[x](javascript:alert(1))`,
		`[x](JaVaScRiPt:alert(1))`,
		"[x](java\tscript:alert(1))",
		"[x](java\nscript:alert(1))",
		`[x](data:text/html;base64,PHNjcmlwdD4=)`,
		`[x](vbscript:msgbox)`,
		`[x](//evil.example/x)`,
		`[x]( javascript:alert(1))`,
		`[x](https://a.example/" onmouseover="alert(1))`,
		`![x](https://tracker.example/p.png)`,
		`![x](javascript:alert(1))`,
		"**<img src=x onerror=1>**",
		"[<b>x</b>](https://a.example)",
		"```\n</code></pre><script>1</script>\n```",
		"> <iframe src=//evil>",
		"- <a href=//evil>x</a>",
		"# <svg onload=1>",
		"&lt;script&gt; &#60;script&#62;",
		"<!-- --><style>*{}</style>",
		`[x](/\evil.example)`,
		`[x](javascript&#58;alert(1))`,
		`[x](&#106;avascript:alert(1))`,
	}

	for _, in := range hostile {
		out := RenderHTML(in, func(string) (string, bool) { return "images/x.png", true })

		stripped := out
		for _, tag := range []string{"p", "h1", "h2", "h3", "li", "ul", "ol", "em", "strong", "code", "pre", "blockquote"} {
			stripped = strings.ReplaceAll(stripped, "<"+tag+">", "")
			stripped = strings.ReplaceAll(stripped, "</"+tag+">", "")
		}

		stripped = strings.ReplaceAll(stripped, "<hr>", "")

		for strings.Contains(stripped, `<a href="`) {
			i := strings.Index(stripped, `<a href="`)
			j := strings.Index(stripped[i:], `" rel="noopener noreferrer">`)

			if j < 0 {
				t.Fatalf("a link of %q is not well formed: %s", in, out)
			}

			href := stripped[i+9 : i+j]
			if strings.ContainsAny(href, `"<>'`) {
				t.Errorf("the href of %q holds a quote or a bracket: %s", in, out)
			}

			if !SafeHref(htmlUnescape(href)) {
				t.Errorf("the link of %q has an unsafe address: %s", in, out)
			}

			stripped = stripped[:i] + stripped[i+j+len(`" rel="noopener noreferrer">`):]
			stripped = strings.Replace(stripped, "</a>", "", 1)
		}

		if strings.ContainsAny(stripped, "<>") {
			t.Errorf("markup the renderer did not write in the output of %q: %s", in, out)
		}

		if strings.Contains(strings.ToLower(out), "javascript:") && strings.Contains(out, "href=\"javascript") {
			t.Errorf("a javascript link in the output of %q: %s", in, out)
		}
	}
}

// A body of the largest size made of marks that never close renders in linear time: before the scan tables, each [ or
// * searched to the end of the block, and 200 KiB of them took from 3 to 34 seconds of the server's CPU.
func TestPostRenderIsLinearOnUnclosedMarks(t *testing.T) {
	t.Parallel()

	nested := strings.Repeat("[", MaxPostBytes/8) + "a" + strings.Repeat("](/x)", MaxPostBytes/8)

	for name, unit := range map[string]string{
		"brackets": "[", "bracket text": "[a", "images": "![", "stars": "*a ", "double stars": "**a ",
		"parens": "[a](", "nested": nested,
	} {
		body := unit
		if name != "nested" {
			body = strings.Repeat(unit, MaxPostBytes/len(unit))
		}

		start := time.Now()
		out := RenderHTML(body, nil)

		if took := time.Since(start); took > 5*time.Second {
			t.Errorf("%s: %d bytes rendered in %v", name, len(body), took)
		}

		if strings.Count(out, "<a ") > 2*maxInlineDepth {
			t.Errorf("%s: links nested past the depth limit", name)
		}
	}
}

func htmlUnescape(s string) string {
	return strings.NewReplacer("&amp;", "&", "&#34;", `"`, "&#39;", "'", "&lt;", "<", "&gt;", ">").Replace(s)
}

func TestPostSafeHref(t *testing.T) {
	t.Parallel()

	for _, ok := range []string{"https://a.example", "HTTP://a.example/x?y=1", "mailto:a@b.example", "/path", "#top"} {
		if !SafeHref(ok) {
			t.Errorf("%q is refused", ok)
		}
	}

	for _, bad := range []string{"", "javascript:1", "//a.example", `/\a.example`, `https://a.example\@b.example`, "ftp://a", "data:x", "a b", "https://a\x00", "relative/path", "https://a" + bidi + ".example"} {
		if SafeHref(bad) {
			t.Errorf("%q is allowed", bad)
		}
	}
}

func TestPostFrontMatter(t *testing.T) {
	t.Parallel()

	m := &PostMeta{
		Title: "Say \"hi\"\nnew: key", Slug: "say-hi", Summary: "s", Tags: []string{"a", "b c"},
		Canonical: "https://example.com/x", Language: "en",
	}
	v := FrontValues{Cover: "images/cover.png", Date: time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC), Draft: true}

	for _, name := range []string{TemplateAstro, TemplateHugo, TemplateJekyll} {
		out, err := FrontMatter(name, "", m, v)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		if !strings.HasPrefix(out, "---\n") || !strings.HasSuffix(out, "---\n") {
			t.Errorf("%s: not a block: %q", name, out)
		}

		if strings.Count(out, "\n---") != 1 || strings.Contains(out, "\nnew: key") {
			t.Errorf("%s: a value added a line or closed the block: %q", name, out)
		}

		if !strings.Contains(out, `"Say \"hi\"\nnew: key"`) || !strings.Contains(out, `["a", "b c"]`) ||
			!strings.Contains(out, "2026-11-20T12:00:00Z") || !strings.Contains(out, "images/cover.png") {
			t.Errorf("%s: %q", name, out)
		}
	}

	astro, _ := FrontMatter(TemplateAstro, "", m, v)
	if !strings.Contains(astro, "draft: true") {
		t.Errorf("astro draft: %q", astro)
	}

	jekyll, _ := FrontMatter(TemplateJekyll, "", m, v)
	if !strings.Contains(jekyll, "published: false") || !strings.Contains(jekyll, "layout: post") {
		t.Errorf("jekyll: %q", jekyll)
	}

	custom, err := FrontMatter(
		TemplateCustom,
		"+++\nt = {{ title }}\nagain = {{title}}\n+++",
		&PostMeta{Title: "{{date}}"},
		v,
	)
	if err != nil || custom != "+++\nt = \"{{date}}\"\nagain = \"{{date}}\"\n+++\n" {
		t.Errorf("custom = %q, %v", custom, err)
	}

	for name, c := range map[string][2]string{
		"unknown preset":      {"jekyl", ""},
		"unknown placeholder": {TemplateCustom, "{{body}}"},
		"empty custom":        {TemplateCustom, " "},
		"huge custom":         {TemplateCustom, strings.Repeat("x", MaxTemplate+1)},
	} {
		if err := CheckTemplate(c[0], c[1]); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestPostKind(t *testing.T) {
	t.Parallel()

	if k, err := CheckKind("post"); err != nil || k != KindPost {
		t.Errorf("CheckKind(post) = %v, %v", k, err)
	}

	if _, err := CheckKind("poem"); err == nil {
		t.Error("poem is a kind")
	}
}
