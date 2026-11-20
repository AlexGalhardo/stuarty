package writing

import (
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Limits of a post (SDD-019 section 13).
const (
	MaxPostTitle     = 200
	MaxPostSlug      = 80
	MaxPostSummary   = 500
	MaxPostTags      = 20
	MaxPostTag       = 40
	MaxPostCanonical = 2048
	MaxPostImages    = 20
	// MaxPostBytes is the limit of every text (project.MaxText), front matter included.
	MaxPostBytes = 256 << 10
)

// The codes of a post issue.
const (
	CodeRequired      = "required"
	CodeTooLong       = "too_long"
	CodeTooMany       = "too_many"
	CodeInvalid       = "invalid"
	CodeUnknownAsset  = "unknown_asset"
	CodeExternalImage = "external_image"
)

var (
	assetIDPattern = regexp.MustCompile(`^ast_[0123456789abcdefghjkmnpqrstvwxyz]{10}$`)
	slugPattern    = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	langPattern    = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)
)

// PostMeta is the front matter of a post.
type PostMeta struct {
	Title     string   `json:"title"     untrusted:"true"`
	Slug      string   `json:"slug"`
	Summary   string   `json:"summary"   untrusted:"true"`
	Tags      []string `json:"tags"      untrusted:"true"`
	Cover     string   `json:"cover"`
	Canonical string   `json:"canonical" untrusted:"true"`
	Language  string   `json:"language"`
}

// PostIssue is one problem of a post: where, which rule, and a sentence that names the value and the limit.
type PostIssue struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Post is a post read from the body of a text.
type Post struct {
	Meta PostMeta
	// Markdown is the body after the front matter.
	Markdown string
	// HasFrontMatter is false when the body has no front matter at all.
	HasFrontMatter bool
}

// frontMatterKeys are the keys of the front matter.
var frontMatterKeys = []string{"title", "slug", "summary", "tags", "cover", "canonical", "language"}

// ParsePost reads the front matter and the body of a post. It never fails: what is wrong comes back as issues, so a
// person sees every problem at once.
func ParsePost(body string) (Post, []PostIssue) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	p := Post{Markdown: body}

	lines := strings.Split(body, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], " \t") != "---" {
		return p, nil
	}

	end := -1

	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], " \t") == "---" {
			end = i

			break
		}
	}

	if end < 0 {
		return p, []PostIssue{
			{
				Path:    "front_matter",
				Code:    CodeInvalid,
				Message: "the front matter starts with --- and never ends: close it with a line that is only ---.",
			},
		}
	}

	p.HasFrontMatter = true
	p.Markdown = strings.TrimPrefix(strings.Join(lines[end+1:], "\n"), "\n")

	var issues []PostIssue

	seen := map[string]bool{}

	for n, line := range lines[1:end] {
		if strings.TrimSpace(line) == "" {
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		key = strings.ToLower(strings.TrimSpace(key))

		if !ok || key == "" {
			issues = append(issues, PostIssue{
				Path: "front_matter", Code: CodeInvalid,
				Message: "line " + strconv.Itoa(n+2) + " of the front matter is not key: value.",
			})

			continue
		}

		if !slices.Contains(frontMatterKeys, key) {
			issues = append(issues, PostIssue{
				Path: "front_matter." + sanitizeKey(key), Code: CodeInvalid,
				Message: "the key " + quoteShort(key) + " is not one of " + strings.Join(frontMatterKeys, ", ") + ".",
			})

			continue
		}

		if seen[key] {
			issues = append(
				issues,
				PostIssue{Path: key, Code: CodeInvalid, Message: "the key " + key + " is written twice."},
			)

			continue
		}

		seen[key] = true

		v, verr := unquote(strings.TrimSpace(value))
		if verr != "" {
			issues = append(issues, PostIssue{Path: key, Code: CodeInvalid, Message: verr})

			continue
		}

		switch key {
		case "title":
			p.Meta.Title = v
		case "slug":
			p.Meta.Slug = v
		case "summary":
			p.Meta.Summary = v
		case "tags":
			p.Meta.Tags = splitTags(v)
		case "cover":
			p.Meta.Cover = v
		case "canonical":
			p.Meta.Canonical = v
		case "language":
			p.Meta.Language = v
		}
	}

	return p, issues
}

// quoteShort writes a key the person wrote in a message, cut and cleaned of control characters.
func quoteShort(s string) string { return `"` + sanitizeKey(s) + `"` }

func sanitizeKey(s string) string {
	r := []rune(s)
	if len(r) > 30 {
		r = r[:30]
	}

	for i, c := range r {
		if unicode.IsControl(c) || c == '"' {
			r[i] = '?'
		}
	}

	return string(r)
}

// unquote reads a value that may be in double quotes with \" and \\ as escapes.
func unquote(v string) (string, string) {
	if !strings.HasPrefix(v, `"`) {
		return v, ""
	}

	if len(v) < 2 || !strings.HasSuffix(v, `"`) {
		return "", "a value that starts with a double quote must end with one."
	}

	var b strings.Builder

	inner := v[1 : len(v)-1]
	for i := 0; i < len(inner); i++ {
		c := inner[i]

		switch {
		case c == '\\' && i+1 < len(inner) && (inner[i+1] == '"' || inner[i+1] == '\\'):
			b.WriteByte(inner[i+1])

			i++
		case c == '"':
			return "", "a double quote inside a quoted value is written \\\"."
		default:
			b.WriteByte(c)
		}
	}

	return b.String(), ""
}

func splitTags(v string) []string {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "[") && strings.HasSuffix(v, "]") {
		v = v[1 : len(v)-1]
	}

	var out []string

	for t := range strings.SplitSeq(v, ",") {
		if t = strings.Trim(strings.TrimSpace(t), `"`); t != "" {
			out = append(out, t)
		}
	}

	return out
}

// Slugify makes a slug from a title: lower case, accents removed, every other run of characters a single hyphen.
func Slugify(title string) string {
	var b strings.Builder

	hyphen := true

	for _, r := range norm.NFKD.String(strings.ToLower(title)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)

			hyphen = false
		case unicode.Is(unicode.Mn, r):
		default:
			if !hyphen {
				b.WriteByte('-')

				hyphen = true
			}
		}
	}

	s := strings.TrimRight(b.String(), "-")
	if len(s) > MaxPostSlug {
		s = strings.TrimRight(s[:MaxPostSlug], "-")
	}

	return s
}

// EffectiveSlug is the slug of the front matter, or the one made from the title.
func (m *PostMeta) EffectiveSlug() string {
	if m.Slug != "" {
		return m.Slug
	}

	return Slugify(m.Title)
}

// unsafeRune says whether a title or a tag may not hold r: a control character, or an invisible or
// direction-changing one (the rule of publish.Invisible, written here because core/writing does not import it).
func unsafeRune(r rune) bool {
	switch {
	case unicode.IsControl(r):
		return true
	case r == 0x200B, r == 0x200C, r == 0xFEFF, r == 0x2060, r == 0x00AD:
		return true
	case r >= 0x200E && r <= 0x200F, r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069:
		return true
	case r >= 0xE0000 && r <= 0xE007F:
		return true
	}

	return false
}

// CheckPost validates a post: the syntax of the front matter, the rules of every value and of the images. images are
// the ids of the assets of the project that are images; nil means the caller did not look, and then an image id is
// not checked against it.
func CheckPost(body string, images []string) (Post, []PostIssue) {
	p, issues := ParsePost(body)
	add := func(path, code, msg string) { issues = append(issues, PostIssue{Path: path, Code: code, Message: msg}) }
	m := &p.Meta

	if !utf8.ValidString(body) {
		add("body", CodeInvalid, "the text is not valid UTF-8.")
	}

	if len(body) > MaxPostBytes {
		add("body", CodeTooLong, "the text is over 256 KiB.")
	}

	switch n := utf8.RuneCountInString(m.Title); {
	case strings.TrimSpace(m.Title) == "":
		add("title", CodeRequired, "the post needs a title: write title: ... in the front matter.")
	case n > MaxPostTitle:
		add("title", CodeTooLong, "the title has "+strconv.Itoa(n)+" characters; the limit is 200.")
	case strings.ContainsFunc(m.Title, unsafeRune):
		add("title", CodeInvalid, "the title has a control or an invisible character.")
	}

	checkSlug(m, add)

	if n := utf8.RuneCountInString(m.Summary); n > MaxPostSummary {
		add("summary", CodeTooLong, "the summary has "+strconv.Itoa(n)+" characters; the limit is 500.")
	} else if strings.ContainsFunc(m.Summary, unsafeRune) {
		add("summary", CodeInvalid, "the summary has a control or an invisible character, or more than one line.")
	}

	checkTags(m.Tags, add)

	if m.Cover != "" {
		switch {
		case !assetIDPattern.MatchString(m.Cover):
			add("cover", CodeInvalid, "the cover is the id of an image of the project (ast_ and 10 characters).")
		case images != nil && !slices.Contains(images, m.Cover):
			add("cover", CodeUnknownAsset, "the cover is not an image of this project.")
		}
	}

	if m.Canonical != "" {
		if u, err := url.Parse(m.Canonical); err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil ||
			len(m.Canonical) > MaxPostCanonical || strings.ContainsFunc(m.Canonical, unsafeRune) {
			add(
				"canonical",
				CodeInvalid,
				"the canonical address is an https URL with a host, no user and at most 2,048 characters.",
			)
		}
	}

	if m.Language != "" && !langPattern.MatchString(m.Language) {
		add("language", CodeInvalid, "the language is a tag such as en, pt-BR or zh-Hant-TW.")
	}

	if strings.TrimSpace(p.Markdown) == "" {
		add("body", CodeRequired, "the post has no text after the front matter.")
	}

	refs, external := scanImages(p.Markdown)
	if external > 0 {
		add(
			"body.images",
			CodeExternalImage,
			"an image of the body is not an asset of the project: use ![alt](asset:<id>); a picture from another address is never fetched.",
		)
	}

	if n := len(Images(p.Markdown, m.Cover)); n > MaxPostImages {
		add("body.images", CodeTooMany, "the post uses "+strconv.Itoa(n)+" images; the limit is 20.")
	}

	for _, id := range refs {
		if !assetIDPattern.MatchString(id) {
			add("body.images", CodeInvalid, "an image names "+quoteShort(id)+", which is not an asset id.")
		} else if images != nil && !slices.Contains(images, id) {
			add("body.images", CodeUnknownAsset, "the image "+id+" is not an image of this project.")
		}
	}

	return p, issues
}

func checkSlug(m *PostMeta, add func(path, code, msg string)) {
	slug := m.EffectiveSlug()

	switch {
	case slug == "" && m.Title != "":
		add("slug", CodeRequired, "the title has nothing a slug can be made of: write slug: ... in the front matter.")
	case len(slug) > MaxPostSlug:
		add("slug", CodeTooLong, "the slug has "+strconv.Itoa(len(slug))+" characters; the limit is 80.")
	case slug != "" && !slugPattern.MatchString(slug):
		add("slug", CodeInvalid, "the slug uses a-z, 0-9 and single hyphens, with none at the ends.")
	}
}

func checkTags(tags []string, add func(path, code, msg string)) {
	if len(tags) > MaxPostTags {
		add("tags", CodeTooMany, "the post has "+strconv.Itoa(len(tags))+" tags; the limit is 20.")
	}

	seen := map[string]bool{}

	for _, t := range tags {
		switch low := strings.ToLower(t); {
		case utf8.RuneCountInString(t) > MaxPostTag:
			add("tags", CodeTooLong, "a tag is over 40 characters.")
		case strings.ContainsFunc(t, unsafeRune):
			add("tags", CodeInvalid, "a tag has a control or an invisible character.")
		case seen[low]:
			add("tags", CodeInvalid, "the tag "+quoteShort(t)+" is written twice.")
		default:
			seen[low] = true
		}
	}
}

var imageRef = regexp.MustCompile(`!\[[^\]\n]*\]\(([^)\n]*)\)`)

// scanImages lists the asset ids of the images of a body, in order, and counts the images that are not assets.
func scanImages(md string) (ids []string, external int) {
	for _, m := range imageRef.FindAllStringSubmatch(stripFences(md), -1) {
		addr := strings.TrimSpace(m[1])
		if id, ok := strings.CutPrefix(addr, "asset:"); ok {
			ids = append(ids, id)
		} else {
			external++
		}
	}

	return ids, external
}

// stripFences removes fenced code blocks, where an image reference is only text.
func stripFences(md string) string {
	var out []string

	in := false

	for line := range strings.SplitSeq(md, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			in = !in

			continue
		}

		if !in {
			out = append(out, line)
		}
	}

	return strings.Join(out, "\n")
}

// Images lists the ids of the assets a post uses: the cover first, then the images of the body in order, each once.
func Images(md, cover string) []string {
	var out []string

	if cover != "" {
		out = append(out, cover)
	}

	ids, _ := scanImages(md)
	for _, id := range ids {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}

	return out
}
