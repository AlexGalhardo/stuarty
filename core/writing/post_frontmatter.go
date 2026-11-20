package writing

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The presets of the front matter of a post written as files (SDD-020 section 44).
const (
	TemplateAstro  = "astro"
	TemplateHugo   = "hugo"
	TemplateJekyll = "jekyll"
	TemplateCustom = "custom"
	// MaxTemplate is the longest custom template, in bytes.
	MaxTemplate = 4096
)

var presets = map[string]string{
	TemplateAstro: "---\ntitle: {{title}}\ndescription: {{summary}}\npubDate: {{date}}\nheroImage: {{cover}}\n" +
		"tags: {{tags}}\nlang: {{language}}\ndraft: {{draft}}\n---\n",
	TemplateHugo: "---\ntitle: {{title}}\ndate: {{date}}\ndraft: {{draft}}\nslug: {{slug}}\nsummary: {{summary}}\n" +
		"tags: {{tags}}\ncover:\n  image: {{cover}}\ncanonicalURL: {{canonical}}\nlanguageCode: {{language}}\n---\n",
	TemplateJekyll: "---\nlayout: post\ntitle: {{title}}\ndate: {{date}}\nexcerpt: {{summary}}\ntags: {{tags}}\n" +
		"lang: {{language}}\ncanonical_url: {{canonical}}\nimage: {{cover}}\npublished: {{published}}\n---\n",
}

var placeholder = regexp.MustCompile(`\{\{\s*([a-z_]+)\s*\}\}`)

// TemplateNames lists the presets.
func TemplateNames() []string {
	return []string{TemplateAstro, TemplateHugo, TemplateJekyll, TemplateCustom}
}

// CheckTemplate validates the template of a connection: a preset name, or custom with a text whose placeholders are
// all known.
func CheckTemplate(name, custom string) error {
	switch name {
	case TemplateAstro, TemplateHugo, TemplateJekyll:
		return nil
	case TemplateCustom:
	default:
		return fmt.Errorf("the template %q is not one of astro, hugo, jekyll, custom", clip(name))
	}

	if strings.TrimSpace(custom) == "" || len(custom) > MaxTemplate {
		return fmt.Errorf("a custom template is 1 to %d bytes", MaxTemplate)
	}

	for _, m := range placeholder.FindAllStringSubmatch(custom, -1) {
		if _, ok := placeholderValue(m[1], &PostMeta{}, FrontValues{}); !ok {
			return fmt.Errorf("the placeholder {{%s}} is not known", clip(m[1]))
		}
	}

	return nil
}

func clip(s string) string {
	r := []rune(s)
	if len(r) > 30 {
		r = r[:30]
	}

	return strings.Map(func(c rune) rune {
		if c < ' ' {
			return '?'
		}

		return c
	}, string(r))
}

// FrontValues are what a front matter needs besides the metadata of the post.
type FrontValues struct {
	// Cover is the path of the cover image as written in the file (images/cover.png), empty with none.
	Cover string
	// Date is the time of the send.
	Date time.Time
	// Draft is true when the visibility is private.
	Draft bool
}

// yamlQuote writes s as a YAML double-quoted scalar. strconv.Quote escapes the backslash, the quote, the line break
// and every control character with escapes YAML reads the same way, so a value can never end its line or add a key.
func yamlQuote(s string) string { return strconv.Quote(s) }

func placeholderValue(name string, m *PostMeta, v FrontValues) (string, bool) {
	switch name {
	case "title":
		return yamlQuote(m.Title), true
	case "slug":
		return yamlQuote(m.EffectiveSlug()), true
	case "summary":
		return yamlQuote(m.Summary), true
	case "cover":
		return yamlQuote(v.Cover), true
	case "canonical":
		return yamlQuote(m.Canonical), true
	case "language":
		return yamlQuote(m.Language), true
	case "date":
		return v.Date.UTC().Format(time.RFC3339), true
	case "draft":
		return strconv.FormatBool(v.Draft), true
	case "published":
		return strconv.FormatBool(!v.Draft), true
	case "tags":
		q := make([]string, len(m.Tags))
		for i, t := range m.Tags {
			q[i] = yamlQuote(t)
		}

		return "[" + strings.Join(q, ", ") + "]", true
	}

	return "", false
}

// FrontMatter renders the front matter of a post from a preset, or from a custom template. Every value is a double-quoted
// YAML scalar, a boolean or an RFC 3339 time written by this function, so no value can close the block or add a key.
func FrontMatter(name, custom string, m *PostMeta, v FrontValues) (string, error) {
	if err := CheckTemplate(name, custom); err != nil {
		return "", err
	}

	tpl := presets[name]
	if name == TemplateCustom {
		tpl = custom
	}

	out := placeholder.ReplaceAllStringFunc(tpl, func(p string) string {
		val, _ := placeholderValue(placeholder.FindStringSubmatch(p)[1], m, v)

		return val
	})

	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}

	return out, nil
}
