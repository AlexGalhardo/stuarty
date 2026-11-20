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

// scan holds, for the runes of one inline run, where each bracket closes and where the next closing parenthesis and
// the next closing emphasis marks are. It is built in one pass from the right, so finding a link or an emphasis is a
// lookup and not a search to the end of the text: a hostile body of unclosed [ or * stays linear.
type scan struct {
	r []rune
	// match is the index of the ] that closes the [ at i, or -1.
	match []int
	// paren is the index of the first ) at or after i, or -1.
	paren []int
	// single and double are the first index at or after i where * or ** may close an emphasis, or -1.
	single, double []int
}

func newScan(r []rune) *scan {
	n := len(r)
	sc := &scan{r: r, match: make([]int, n), paren: make([]int, n+1), single: make([]int, n+1), double: make([]int, n+1)}

	var open []int

	for i, c := range r {
		sc.match[i] = -1

		switch c {
		case '[':
			open = append(open, i)
		case ']':
			if len(open) > 0 {
				sc.match[open[len(open)-1]] = i
				open = open[:len(open)-1]
			}
		}
	}

	// escaped[i] is true when an odd run of backslashes stands right before r[i], which is what the left-to-right
	// skip of an escape in an emphasis makes of it.
	escaped := make([]bool, n)
	run := 0

	for i, c := range r {
		escaped[i] = run%2 == 1

		if c == '\\' {
			run++
		} else {
			run = 0
		}
	}

	sc.paren[n], sc.single[n], sc.double[n] = -1, -1, -1

	for i := n - 1; i >= 0; i-- {
		sc.paren[i], sc.single[i], sc.double[i] = sc.paren[i+1], sc.single[i+1], sc.double[i+1]

		if r[i] == ')' {
			sc.paren[i] = i
		}

		if r[i] != '*' || escaped[i] || i == 0 || r[i-1] == ' ' {
			continue
		}

		if i+1 < n && r[i+1] == '*' {
			sc.double[i] = i
		} else {
			sc.single[i] = i
		}
	}

	return sc
}

// inline renders the inline Markdown of one block.
func inline(s string, src ImageSrc, depth int) string {
	var b strings.Builder

	r := []rune(s)
	sc := newScan(r)

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
			if text, addr, next, ok := sc.linkAt(i + 1); ok {
				b.WriteString(renderImage(text, addr, src))

				i = next

				continue
			}

			b.WriteRune(c)

			i++
		case c == '[' && depth < maxInlineDepth:
			if text, addr, next, ok := sc.linkAt(i); ok {
				b.WriteString(renderLink(text, addr, src, depth))

				i = next

				continue
			}

			b.WriteString(html.EscapeString(string(c)))

			i++
		case c == '*' && depth < maxInlineDepth:
			if out, next, ok := sc.emphasisAt(i, src, depth); ok {
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

// linkAt reads [text](address) starting at the [ at r[i]: the ] that closes it, then ( right after, then the first ).
// next is the index after the closing parenthesis.
func (sc *scan) linkAt(i int) (text, addr string, next int, ok bool) {
	r := sc.r

	j := sc.match[i]
	if j < 0 || j+1 >= len(r) || r[j+1] != '(' {
		return "", "", 0, false
	}

	end := sc.paren[j+2]
	if end < 0 {
		return "", "", 0, false
	}

	return string(r[i+1 : j]), strings.TrimSpace(string(r[j+2 : end])), end + 1, true
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
// them inside a scheme (java\tscript:), and so is a backslash, which a browser reads as a slash: /\host is //host, an
// address on another site.
func SafeHref(addr string) bool {
	if addr == "" || strings.ContainsFunc(addr, func(r rune) bool {
		return r <= ' ' || r == 0x7f || r == '\\' || unsafeRune(r)
	}) {
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

// emphasisAt reads *text* or **text** starting at the * at r[i]: the closing marks are the first ones after the first
// character of the text that are not escaped and do not follow a space.
func (sc *scan) emphasisAt(i int, src ImageSrc, depth int) (string, int, bool) {
	r := sc.r

	marks, closers, tag := 1, sc.single, "em"
	if i+1 < len(r) && r[i+1] == '*' {
		marks, closers, tag = 2, sc.double, "strong"
	}

	start := i + marks
	if start >= len(r) || r[start] == ' ' || r[start] == '\n' {
		return "", 0, false
	}

	j := closers[start+1]
	if j < 0 {
		return "", 0, false
	}

	return "<" + tag + ">" + inline(string(r[start:j]), src, depth+1) + "</" + tag + ">", j + marks, true
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
