package writing

import (
	"html"
	"regexp"
	"strconv"
	"strings"
)

// ImageSrc gives the address an image of an asset is written with in the HTML; false when the asset is not known,
// and then the image is written as its alt text.
type ImageSrc func(assetID string) (string, bool)

const maxInlineDepth = 4

var (
	headingLine = regexp.MustCompile(`^(#{1,6})[ \t]+(.*?)[ \t]*#*[ \t]*$`)
	ruleLine    = regexp.MustCompile(`^ {0,3}(?:(?:-[ \t]*){3,}|(?:\*[ \t]*){3,}|(?:_[ \t]*){3,})$`)
	bulletLine  = regexp.MustCompile(`^ {0,3}[-*+][ \t]+(.*)$`)
	orderedLine = regexp.MustCompile(`^ {0,3}\d{1,9}[.)][ \t]+(.*)$`)
)

// RenderHTML renders the Markdown of a post to HTML for the destinations (SDD-019 section 13). It writes only its own
// fixed tags and escapes every character of the text, so no text of a post can become a tag, an attribute or a script;
// raw HTML is shown as text. The output is deterministic.
func RenderHTML(md string, src ImageSrc) string {
	var out strings.Builder

	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")

	for i := 0; i < len(lines); {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		switch {
		case trimmed == "":
			i++
		case strings.HasPrefix(trimmed, "```"):
			i = renderFence(&out, lines, i)
		case headingLine.MatchString(strings.TrimLeft(line, " ")) && len(line)-len(strings.TrimLeft(line, " ")) < 4:
			m := headingLine.FindStringSubmatch(strings.TrimLeft(line, " "))
			tag := "h" + strconv.Itoa(len(m[1]))
			out.WriteString("<" + tag + ">" + inline(m[2], src, 0) + "</" + tag + ">\n")

			i++
		case ruleLine.MatchString(line):
			out.WriteString("<hr>\n")

			i++
		case strings.HasPrefix(trimmed, ">"):
			i = renderQuote(&out, lines, i, src)
		case bulletLine.MatchString(line):
			i = renderList(&out, lines, i, "ul", bulletLine, src)
		case orderedLine.MatchString(line):
			i = renderList(&out, lines, i, "ol", orderedLine, src)
		default:
			i = renderParagraph(&out, lines, i, src)
		}
	}

	return out.String()
}

func renderFence(out *strings.Builder, lines []string, i int) int {
	out.WriteString("<pre><code>")

	i++

	var body []string

	for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
		body = append(body, lines[i])
		i++
	}

	out.WriteString(html.EscapeString(strings.Join(body, "\n")))
	out.WriteString("</code></pre>\n")

	return min(i+1, len(lines))
}

func startsBlock(line string) bool {
	t := strings.TrimSpace(line)

	return t == "" || strings.HasPrefix(t, "```") || strings.HasPrefix(t, ">") || strings.HasPrefix(t, "#") ||
		bulletLine.MatchString(line) || orderedLine.MatchString(line) || ruleLine.MatchString(line)
}

func renderQuote(out *strings.Builder, lines []string, i int, src ImageSrc) int {
	var inner []string

	for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), ">") {
		t := strings.TrimPrefix(strings.TrimSpace(lines[i]), ">")
		inner = append(inner, strings.TrimPrefix(t, " "))
		i++
	}

	out.WriteString("<blockquote>\n")

	var para []string

	flush := func() {
		if len(para) > 0 {
			out.WriteString("<p>" + inline(strings.Join(para, "\n"), src, 0) + "</p>\n")

			para = nil
		}
	}

	for _, l := range inner {
		if strings.TrimSpace(l) == "" {
			flush()

			continue
		}

		para = append(para, l)
	}

	flush()
	out.WriteString("</blockquote>\n")

	return i
}

func renderList(out *strings.Builder, lines []string, i int, tag string, re *regexp.Regexp, src ImageSrc) int {
	out.WriteString("<" + tag + ">\n")

	for i < len(lines) {
		m := re.FindStringSubmatch(lines[i])
		if m == nil {
			break
		}

		out.WriteString("<li>" + inline(m[1], src, 0) + "</li>\n")

		i++
	}

	out.WriteString("</" + tag + ">\n")

	return i
}

func renderParagraph(out *strings.Builder, lines []string, i int, src ImageSrc) int {
	var para []string

	for i < len(lines) && (len(para) == 0 || !startsBlock(lines[i])) {
		para = append(para, strings.TrimSpace(lines[i]))
		i++
	}

	out.WriteString("<p>" + inline(strings.Join(para, "\n"), src, 0) + "</p>\n")

	return i
}

// inline renders the inline Markdown of one block.
func inline(s string, src ImageSrc, depth int) string {
	var b strings.Builder

	r := []rune(s)

	for i := 0; i < len(r); {
		c := r[i]

		switch {
		case c == '\\' && i+1 < len(r) && strings.ContainsRune("\\`*[]()!#_{}<>.-+", r[i+1]):
			b.WriteString(html.EscapeString(string(r[i+1])))

			i += 2
		case c == '`':
			if end := indexRune(r, '`', i+1); end > i+1 {
				b.WriteString("<code>" + html.EscapeString(string(r[i+1:end])) + "</code>")

				i = end + 1

				continue
			}

			b.WriteRune(c)

			i++
		case c == '!' && i+1 < len(r) && r[i+1] == '[':
			if text, addr, next, ok := linkAt(r, i+1); ok {
				b.WriteString(renderImage(text, addr, src))

				i = next

				continue
			}

			b.WriteRune(c)

			i++
		case c == '[':
			if text, addr, next, ok := linkAt(r, i); ok {
				b.WriteString(renderLink(text, addr, src, depth))

				i = next

				continue
			}

			b.WriteRune(c)

			i++
		case c == '*' && depth < maxInlineDepth:
			if out, next, ok := emphasisAt(r, i, src, depth); ok {
				b.WriteString(out)

				i = next

				continue
			}

			b.WriteRune(c)

			i++
		default:
			b.WriteString(html.EscapeString(string(c)))

			i++
		}
	}

	return b.String()
}

func indexRune(r []rune, c rune, from int) int {
	for i := from; i < len(r); i++ {
		if r[i] == c {
			return i
		}
	}

	return -1
}

// linkAt reads [text](address) starting at the [ at r[i]. next is the index after the closing parenthesis.
func linkAt(r []rune, i int) (text, addr string, next int, ok bool) {
	depth := 0

	for j := i; j < len(r); j++ {
		switch r[j] {
		case '[':
			depth++
		case ']':
			depth--

			if depth == 0 {
				if j+1 >= len(r) || r[j+1] != '(' {
					return "", "", 0, false
				}

				end := indexRune(r, ')', j+2)
				if end < 0 {
					return "", "", 0, false
				}

				return string(r[i+1 : j]), strings.TrimSpace(string(r[j+2 : end])), end + 1, true
			}
		case '\n':
			if depth == 0 {
				return "", "", 0, false
			}
		}
	}

	return "", "", 0, false
}

func renderImage(alt, addr string, src ImageSrc) string {
	plain := html.EscapeString(stripMarks(alt))

	id, isAsset := strings.CutPrefix(addr, "asset:")
	if !isAsset || src == nil {
		return plain
	}

	s, ok := src(id)
	if !ok {
		return plain
	}

	return `<img src="` + html.EscapeString(s) + `" alt="` + plain + `">`
}

func stripMarks(s string) string {
	return strings.NewReplacer("*", "", "`", "", "\\", "").Replace(s)
}

func renderLink(text, addr string, src ImageSrc, depth int) string {
	label := inline(text, src, depth+1)
	if !SafeHref(addr) {
		return label
	}

	return `<a href="` + html.EscapeString(addr) + `" rel="noopener noreferrer">` + label + `</a>`
}

// SafeHref says whether an address may be written as a link: http, https and mailto, a path that starts with one
// slash, or an anchor. Anything with white space or a control character is refused, since a browser ignores some of
// them inside a scheme (java\tscript:).
func SafeHref(addr string) bool {
	if addr == "" || strings.ContainsFunc(addr, func(r rune) bool { return r <= ' ' || r == 0x7f || unsafeRune(r) }) {
		return false
	}

	low := strings.ToLower(addr)

	switch {
	case strings.HasPrefix(low, "http://"), strings.HasPrefix(low, "https://"), strings.HasPrefix(low, "mailto:"):
		return true
	case strings.HasPrefix(addr, "#"):
		return true
	case strings.HasPrefix(addr, "/") && !strings.HasPrefix(addr, "//"):
		return true
	}

	return false
}

func emphasisAt(r []rune, i int, src ImageSrc, depth int) (string, int, bool) {
	marks := 1
	if i+1 < len(r) && r[i+1] == '*' {
		marks = 2
	}

	start := i + marks
	if start >= len(r) || r[start] == ' ' || r[start] == '\n' {
		return "", 0, false
	}

	for j := start; j+marks <= len(r); j++ {
		if r[j] == '\\' {
			j++

			continue
		}

		if r[j] != '*' || r[j-1] == ' ' || j == start {
			continue
		}

		if marks == 2 && (j+1 >= len(r) || r[j+1] != '*') {
			continue
		}

		if marks == 1 && j+1 < len(r) && r[j+1] == '*' {
			continue
		}

		tag := "em"
		if marks == 2 {
			tag = "strong"
		}

		return "<" + tag + ">" + inline(string(r[start:j]), src, depth+1) + "</" + tag + ">", j + marks, true
	}

	return "", 0, false
}

// RewriteImages replaces, in the Markdown, every ![alt](asset:<id>) with the path to gives, outside fenced code. An id
// to does not know is left as it is.
func RewriteImages(md string, to func(assetID string) (string, bool)) string {
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	in := false

	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			in = !in

			continue
		}

		if in {
			continue
		}

		lines[i] = imageRef.ReplaceAllStringFunc(line, func(m string) string {
			sub := imageRef.FindStringSubmatch(m)
			addr := strings.TrimSpace(sub[1])

			id, ok := strings.CutPrefix(addr, "asset:")
			if !ok {
				return m
			}

			path, known := to(id)
			if !known {
				return m
			}

			return strings.Replace(m, "("+sub[1]+")", "("+path+")", 1)
		})
	}

	return strings.Join(lines, "\n")
}
