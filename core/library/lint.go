package library

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// The severities of a finding.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
)

// The codes of the skill lint (SDD-018 section 7.1).
const (
	LintNoSkillMD         = "L_SKILL_NO_SKILL_MD"
	LintFrontMatter       = "L_SKILL_FRONT_MATTER"
	LintName              = "L_SKILL_NAME"
	LintNameMismatch      = "L_SKILL_NAME_MISMATCH"
	LintDescription       = "L_SKILL_DESCRIPTION"
	LintDescriptionLength = "L_SKILL_DESCRIPTION_LENGTH"
	LintBodyEmpty         = "L_SKILL_BODY_EMPTY"
	LintReferenceMissing  = "L_SKILL_REFERENCE_MISSING"
	LintReferenceEscapes  = "L_SKILL_REFERENCE_ESCAPES"
	LintCredential        = "L_SKILL_CREDENTIAL" //nolint:gosec // the name of a lint rule
	LintRemoteExec        = "L_SKILL_REMOTE_EXEC"
	LintHiddenText        = "L_SKILL_HIDDEN_TEXT"
	LintSize              = "L_SKILL_SIZE"
	LintHTMLComment       = "L_SKILL_HTML_COMMENT"
	LintScript            = "L_SKILL_SCRIPT"
	LintAllowedTools      = "L_SKILL_ALLOWED_TOOLS"
	LintLicense           = "L_SKILL_LICENSE"
)

// LintCodes lists every code, for the documentation and for the tests.
var LintCodes = []string{
	LintNoSkillMD, LintFrontMatter, LintName, LintNameMismatch, LintDescription, LintDescriptionLength, LintBodyEmpty,
	LintReferenceMissing, LintReferenceEscapes, LintCredential, LintRemoteExec, LintHiddenText, LintSize,
	LintHTMLComment, LintScript, LintAllowedTools, LintLicense,
}

// Finding is one result of the lint.
type Finding struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Path     string `json:"path,omitempty"` // a file of the folder
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
}

// HasError reports whether any finding is an error.
func HasError(fs []Finding) bool {
	for _, f := range fs {
		if f.Severity == SeverityError {
			return true
		}
	}

	return false
}

// LintSkill checks the folder of a skill: dir is the name of the folder, files
// are its files (SKILL.md included). It never fails: a folder it cannot read as
// a skill is reported as findings.
func LintSkill(dir string, files []File) []Finding {
	l := &linter{files: map[string]File{}}
	for _, f := range files {
		l.files[f.Path] = f
	}

	main, ok := l.files[SkillFile]
	if !ok {
		return []Finding{{
			Code: LintNoSkillMD, Severity: SeverityError, Message: "the folder has no " + SkillFile + ".",
		}}
	}

	l.sizes(files)

	if skill, err := ParseSkill(files); err != nil {
		fe := &FormatError{}
		msg := err.Error()

		line := 0
		if asFormat(err, fe) {
			line = fe.Line
		}

		l.add(LintFrontMatter, SeverityError, SkillFile, line, msg+".")
	} else {
		l.fields(dir, &skill)
		l.references(&skill)
		l.text(SkillFile, string(main.Data), true)
	}

	for _, f := range files {
		if f.Path != SkillFile {
			l.otherFile(f)
		}
	}

	sort.SliceStable(l.out, func(i, j int) bool {
		a, b := l.out[i], l.out[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}

		if a.Line != b.Line {
			return a.Line < b.Line
		}

		return a.Code < b.Code
	})

	return l.out
}

func asFormat(err error, into *FormatError) bool {
	fe, ok := err.(*FormatError) //nolint:errorlint // ParseSkill returns it unwrapped
	if ok {
		*into = *fe
	}

	return ok
}

type linter struct {
	files map[string]File
	out   []Finding
}

func (l *linter) add(code, severity, file string, line int, msg string) {
	l.out = append(l.out, Finding{Code: code, Severity: severity, Path: file, Line: line, Message: msg})
}

func (l *linter) sizes(files []File) {
	total := 0

	for _, f := range files {
		total += len(f.Data)

		limit := MaxSkillFile
		if f.Path == SkillFile {
			limit = MaxSkillMD
		}

		if len(f.Data) > limit {
			l.add(
				LintSize,
				SeverityError,
				f.Path,
				0,
				fmt.Sprintf("the file is %d bytes; the limit is %d.", len(f.Data), limit),
			)
		}
	}

	if total > MaxSkillBytes {
		l.add(
			LintSize,
			SeverityError,
			"",
			0,
			fmt.Sprintf("the folder is %d bytes; the limit is %d.", total, MaxSkillBytes),
		)
	}

	if len(files) > MaxSkillFiles {
		l.add(
			LintSize,
			SeverityError,
			"",
			0,
			fmt.Sprintf("the folder has %d files; the limit is %d.", len(files), MaxSkillFiles),
		)
	}
}

func (l *linter) fields(dir string, s *Skill) {
	switch err := ValidName(s.Name); {
	case s.Name == "":
		l.add(LintName, SeverityError, SkillFile, 0, "the front matter has no name.")
	case err != nil:
		l.add(LintName, SeverityError, SkillFile, 0, "name: "+err.Error()+".")
	case dir != "" && s.Name != dir:
		l.add(
			LintNameMismatch,
			SeverityError,
			SkillFile,
			0,
			fmt.Sprintf("the name %q is not the folder name %q.", s.Name, dir),
		)
	}

	switch n := utf8.RuneCountInString(s.Description); {
	case strings.TrimSpace(s.Description) == "":
		l.add(LintDescription, SeverityError, SkillFile, 0, "the front matter has no description.")
	case n > MaxSkillDescription:
		l.add(LintDescriptionLength, SeverityError, SkillFile, 0,
			fmt.Sprintf("the description is %d characters; the limit is %d.", n, MaxSkillDescription))
	}

	if utf8.RuneCountInString(s.Compatibility) > MaxCompatibility {
		l.add(LintDescriptionLength, SeverityError, SkillFile, 0,
			fmt.Sprintf("compatibility is longer than %d characters.", MaxCompatibility))
	}

	if strings.TrimSpace(s.Body) == "" {
		l.add(LintBodyEmpty, SeverityError, SkillFile, 0, "the skill has no instructions after the front matter.")
	}

	if s.AllowedTools != "" {
		l.add(LintAllowedTools, SeverityWarning, SkillFile, 0,
			"allowed-tools is set: another agent may pre-approve those tools when it loads the skill.")
	}

	if s.Origin() == OriginImported && strings.TrimSpace(s.License) == "" {
		l.add(LintLicense, SeverityWarning, SkillFile, 0, "an imported skill names no licence.")
	}
}

var (
	mdLink    = regexp.MustCompile(`\[[^\]\n]*\]\(\s*<?([^)\s>]+)>?(?:\s+"[^"\n]*")?\s*\)`)
	pathQuote = regexp.MustCompile("`((?:references|assets|scripts)/[^`\\s]+)`")
	scheme    = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*:`)
)

// references checks that every file the instructions point to is in the folder.
func (l *linter) references(s *Skill) {
	body := stripFences(s.Body)
	offset := lineOffset(s)

	for i, line := range strings.Split(body, "\n") {
		var targets []string
		for _, m := range mdLink.FindAllStringSubmatch(line, -1) {
			targets = append(targets, m[1])
		}

		for _, m := range pathQuote.FindAllStringSubmatch(line, -1) {
			targets = append(targets, m[1])
		}

		for _, t := range targets {
			l.checkTarget(t, i+offset)
		}
	}
}

func lineOffset(s *Skill) int {
	// The body starts after the front matter and its two fences.
	n := 1

	if s.fm != nil {
		if front, err := s.fm.Render(); err == nil {
			n += strings.Count(front, "\n") + 2
		}
	}

	return n
}

func (l *linter) checkTarget(target string, line int) {
	if i := strings.IndexAny(target, "#?"); i >= 0 {
		target = target[:i]
	}

	if target == "" || scheme.MatchString(target) {
		return
	}

	if strings.HasPrefix(target, "/") || strings.HasPrefix(target, `\`) || strings.Contains(target, `\`) {
		l.add(LintReferenceEscapes, SeverityError, SkillFile, line, fmt.Sprintf("%q is an absolute path.", target))

		return
	}

	clean := path.Clean(target)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		l.add(LintReferenceEscapes, SeverityError, SkillFile, line, fmt.Sprintf("%q leaves the skill folder.", target))

		return
	}

	if _, ok := l.files[clean]; ok {
		return
	}

	for p := range l.files {
		if strings.HasPrefix(p, strings.TrimSuffix(clean, "/")+"/") {
			return
		}
	}

	l.add(LintReferenceMissing, SeverityError, SkillFile, line, fmt.Sprintf("%q is not a file of the folder.", target))
}

// stripFences blanks the lines inside fenced code blocks, keeping the line
// numbers: an example in a code block is not a reference.
func stripFences(body string) string {
	lines := strings.Split(body, "\n")
	in := false

	for i, l := range lines {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
			in = !in
			lines[i] = ""

			continue
		}

		if in {
			lines[i] = ""
		}
	}

	return strings.Join(lines, "\n")
}

var (
	negation = regexp.MustCompile(`(?i)\b(not|never|don'?t|do not|must not|mustn'?t|without|avoid|refuse|no)\b`)
	verbs    = `ask|request|prompt|require|collect|obtain|get|fetch|provide|enter|paste|type|send|share|give|include|upload|post|print|echo|cat|read|show|display|reveal|export|copy|exfiltrate|leak|forward|email|submit|attach|open|dump`
	secrets  = `api[ _-]?keys?|access[ _-]?tokens?|auth(?:orization)?[ _-]?tokens?|bearer[ _-]?tokens?|session[ _-]?tokens?|refresh[ _-]?tokens?|oauth[ _-]?tokens?|passwords?|passphrases?|secrets?|credentials?|private[ _-]?keys?|ssh[ _-]?keys?|master[ _-]?key|seed[ _-]?phrase|cookies?` //nolint:gosec // a list of words the lint looks for
	secFiles = `\.env\b|vault\.json|\.ssh/|id_rsa|id_ed25519|\.aws/credentials|\.npmrc|\.netrc|stuarty_master_key|credentials\.json|\.git-credentials`

	credVerbObject = regexp.MustCompile(`(?i)\b(` + verbs + `)\b[^.\n]{0,60}?\b(` + secrets + `)\b`)
	credVerbFile   = regexp.MustCompile(`(?i)\b(` + verbs + `|curl|scp)\b[^\n]{0,60}?(` + secFiles + `)`)
	remoteExec     = regexp.MustCompile(
		`(?i)\b(curl|wget|iwr|invoke-webrequest|invoke-restmethod|irm)\b[^\n|]*\|\s*(sudo\s+)?` +
			`(sh|bash|zsh|python3?|node|perl|ruby|pwsh|powershell|iex|invoke-expression)\b`)
	remoteExecPS = regexp.MustCompile(`(?i)\b(iex|invoke-expression)\b[^\n]*(downloadstring|invoke-webrequest|iwr|irm)`)
)

// text applies the rules that read the words of an instruction: hidden text,
// a request for a credential, a remote execution and an HTML comment.
func (l *linter) text(file, text string, instruction bool) {
	for i, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		n := i + 1

		if hasHidden(line) {
			l.add(
				LintHiddenText,
				SeverityError,
				file,
				n,
				"the line has characters a person cannot see and a model reads.",
			)
		}

		if remoteExec.MatchString(line) || remoteExecPS.MatchString(line) {
			l.add(LintRemoteExec, SeverityError, file, n, "the text pipes a download into a shell or an interpreter.")
		}

		if !instruction {
			continue
		}

		if credentialAsk(line) {
			l.add(
				LintCredential,
				SeverityError,
				file,
				n,
				"the text tells the agent to ask for, read, print or send a credential.",
			)
		}

		if strings.Contains(line, "<!--") {
			l.add(
				LintHTMLComment,
				SeverityWarning,
				file,
				n,
				"an HTML comment is a place to hide an instruction; the reader does not see it.",
			)
		}
	}
}

// credentialAsk reports whether the line tells the agent to deal with a
// credential, unless a negation stands in front of the verb ("never ask for").
func credentialAsk(line string) bool {
	for _, re := range []*regexp.Regexp{credVerbObject, credVerbFile} {
		for _, m := range re.FindAllStringSubmatchIndex(line, -1) {
			from := max(0, m[0]-30)
			if !negation.MatchString(line[from : m[0]+len(line[m[2]:m[3]])]) {
				return true
			}
		}
	}

	return false
}

var scriptExt = map[string]bool{
	".sh": true, ".bash": true, ".zsh": true, ".fish": true, ".ps1": true, ".psm1": true, ".bat": true, ".cmd": true,
	".py": true, ".js": true, ".mjs": true, ".cjs": true, ".ts": true, ".rb": true, ".pl": true, ".php": true,
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".bin": true, ".jar": true, ".com": true, ".msi": true,
}

var textExt = map[string]bool{".md": true, ".txt": true, ".markdown": true, ".rst": true}

// otherFile checks a file that is not SKILL.md.
func (l *linter) otherFile(f File) {
	ext := strings.ToLower(path.Ext(f.Path))

	isScript := strings.HasPrefix(f.Path, "scripts/") || scriptExt[ext] || strings.HasPrefix(string(f.Data), "#!")
	if isScript {
		l.add(
			LintScript,
			SeverityWarning,
			f.Path,
			0,
			"a script: it is stored as an inert file and Stuarty never runs it.",
		)
	}

	if !utf8.Valid(f.Data) {
		return
	}

	// An agent reads any text file a skill points to (a .json, a .yaml, a file with no extension), so hidden
	// characters and a piped download are looked for in every one; the words of an instruction only in prose.
	l.text(f.Path, string(f.Data), textExt[ext])
}

// hasHidden reports whether the text has a character a person cannot see.
func hasHidden(s string) bool {
	run := 0

	for _, r := range s {
		if hiddenRune(r) {
			return true
		}

		if r == 0x200C || r == 0x200D || r >= 0xFE00 && r <= 0xFE0F {
			run++
			if run >= 2 {
				return true
			}

			continue
		}

		run = 0
	}

	return false
}

// hiddenRune is true for what SDD-007 section 8.2 removes from untrusted text:
// control characters, bidirectional controls, zero width and tag characters.
func hiddenRune(r rune) bool {
	switch {
	case r < 0x20 && r != '\n' && r != '\t' && r != '\r', r >= 0x7f && r <= 0x9f:
		return true
	case r >= 0x202A && r <= 0x202E, r >= 0x2066 && r <= 0x2069, r == 0x200E, r == 0x200F:
		return true
	case r == 0x200B, r >= 0x2060 && r <= 0x2064, r == 0xFEFF, r == 0x180E, r >= 0xFFF9 && r <= 0xFFFB:
		return true
	case r == 0x2028, r == 0x2029:
		return true
	case r >= 0xE0000 && r <= 0xE007F, r >= 0xE0100 && r <= 0xE01EF:
		return true
	}

	return false
}

// HasHiddenText reports whether a text holds a character that a person cannot
// see (used on names and descriptions the service reads from outside).
func HasHiddenText(s string) bool { return hasHidden(s) }
