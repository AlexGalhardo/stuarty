package library

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// SkillFile is the name of the file every skill folder has.
const SkillFile = "SKILL.md"

// The keys of `metadata` that the product owns (SDD-018 section 7).
const (
	MetaOrigin   = "stuarty-origin"
	MetaSource   = "stuarty-source"
	MetaReviewed = "stuarty-reviewed"
)

// The origins of a skill (RF-387).
const (
	OriginBuiltIn  = "built-in"
	OriginUser     = "user"
	OriginImported = "imported"
)

// ValidOrigin reports whether o is one of the three origins.
func ValidOrigin(o string) bool {
	return o == OriginBuiltIn || o == OriginUser || o == OriginImported
}

// File is one file of a skill folder, by its path inside the folder with
// forward slashes.
type File struct {
	Path string
	Data []byte
}

// Skill is a skill folder in the Agent Skills layout.
type Skill struct {
	Name          string
	Description   string
	License       string
	Compatibility string
	AllowedTools  string
	Metadata      map[string]string
	Body          string
	Files         []File // every file except SKILL.md

	fm *FrontMatter
}

// ParseSkill reads a skill from the files of its folder. It needs SKILL.md and
// checks the shape of its front matter, not the rules of the fields: LintSkill
// does that.
func ParseSkill(files []File) (Skill, error) {
	var main *File

	rest := make([]File, 0, len(files))

	for i := range files {
		if files[i].Path == SkillFile {
			main = &files[i]

			continue
		}

		rest = append(rest, files[i])
	}

	if main == nil {
		return Skill{}, &FormatError{Msg: "the folder has no " + SkillFile}
	}

	if !utf8.Valid(main.Data) {
		return Skill{}, &FormatError{Msg: SkillFile + " is not valid UTF-8"}
	}

	fm, body, err := ParseDocument(string(main.Data))
	if err != nil {
		return Skill{}, err
	}

	sort.Slice(rest, func(i, j int) bool { return rest[i].Path < rest[j].Path })

	return Skill{
		Name: fm.Scalar("name"), Description: fm.Scalar("description"), License: fm.Scalar("license"),
		Compatibility: fm.Scalar("compatibility"), AllowedTools: fm.Scalar("allowed-tools"),
		Metadata: fm.Map("metadata"), Body: body, Files: rest, fm: fm,
	}, nil
}

// frontMatter builds the front matter from the fields, keeping the keys the
// product does not know.
func (s Skill) frontMatter() *FrontMatter {
	fm := baseFrontMatter(s.fm)
	fm.SetScalar("name", s.Name)
	fm.SetScalar("description", s.Description)
	fm.SetScalar("license", s.License)
	fm.SetScalar("compatibility", s.Compatibility)
	fm.SetScalar("allowed-tools", s.AllowedTools)
	fm.SetMap("metadata", s.Metadata)

	return fm
}

// SkillMD writes the text of SKILL.md.
func (s Skill) SkillMD() (string, error) {
	return Compose(s.frontMatter(), s.Body)
}

// Origin is the recorded origin of the skill, "" when the file records none.
func (s Skill) Origin() string { return s.Metadata[MetaOrigin] }

// Source is the recorded source text of the skill.
func (s Skill) Source() string { return s.Metadata[MetaSource] }

// SetOrigin records the origin and the source; an empty source removes it.
func (s *Skill) SetOrigin(origin, source string) {
	s.setMeta(MetaOrigin, origin)
	s.setMeta(MetaSource, source)
}

func (s *Skill) setMeta(key, value string) {
	m := cloneMap(s.Metadata)
	if m == nil {
		m = map[string]string{}
	}

	if value == "" {
		delete(m, key)
	} else {
		m[key] = value
	}

	s.Metadata = m
}

// ReviewedMark is the review a file records: the hash a person read, or "".
func (s Skill) ReviewedMark() string { return s.Metadata[MetaReviewed] }

// MarkReviewed records that a person read the content as it is now.
func (s *Skill) MarkReviewed() { s.setMeta(MetaReviewed, s.ContentHash()) }

// ClearReview removes the review mark.
func (s *Skill) ClearReview() { s.setMeta(MetaReviewed, "") }

// Reviewed reports whether the recorded review is of the content as it is now:
// the mark is the content hash, so any edit from any tool takes the review
// away, and a file cannot claim a review it does not carry the hash of.
func (s Skill) Reviewed() bool {
	mark := s.ReviewedMark()

	return mark != "" && mark == s.ContentHash()
}

// ContentHash is "sha256:" and the hex digest of what a person reads: the front
// matter without the review mark, the body and every other file of the folder
// (path, length and bytes, in path order).
func (s Skill) ContentHash() string {
	clean := s
	clean.Metadata = cloneMap(s.Metadata)
	delete(clean.Metadata, MetaReviewed)

	h := sha256.New()

	front, err := clean.frontMatter().Render()
	if err != nil {
		front = "unrenderable"
	}

	writeHashPart(h, []byte(front))
	writeHashPart(h, []byte(s.Body))

	files := append([]File(nil), s.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	for _, f := range files {
		writeHashPart(h, []byte(f.Path))
		writeHashPart(h, f.Data)
	}

	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

type hashWriter interface{ Write(p []byte) (int, error) }

// writeHashPart writes a length and the bytes, so that two different splits of
// the same bytes hash differently.
func writeHashPart(h hashWriter, b []byte) {
	_, _ = h.Write([]byte(strconv.Itoa(len(b)) + ":"))
	_, _ = h.Write(b)
}

// AllFiles returns SKILL.md followed by the other files, as written to disk.
func (s Skill) AllFiles() ([]File, error) {
	md, err := s.SkillMD()
	if err != nil {
		return nil, err
	}

	return append([]File{{Path: SkillFile, Data: []byte(md)}}, s.Files...), nil
}

// ValidFilePath checks the path of a file inside a skill folder: printable
// ASCII (so no two names fold together and none hides a character), forward
// slashes, relative, no "." or ".." component, no backslash, drive letter or
// control character, no Windows device name, no trailing dot or space.
func ValidFilePath(p string) error {
	if p == "" || len(p) > 200 {
		return errors.New("the path is empty or longer than 200 bytes")
	}

	if !utf8.ValidString(p) {
		return errors.New("the path is not valid UTF-8")
	}

	if strings.HasPrefix(p, "/") || strings.ContainsAny(p, "\\:") {
		return errors.New("the path is absolute or has a backslash or a colon")
	}

	for _, r := range p {
		if r < 0x20 || r > 0x7e {
			return errors.New("the path has a control character or one that is not printable ASCII")
		}
	}

	for part := range strings.SplitSeq(p, "/") {
		switch {
		case part == "" || part == "." || part == "..":
			return errors.New("the path has an empty, . or .. component")
		case strings.HasSuffix(part, ".") || strings.HasSuffix(part, " "):
			return errors.New("a component ends with a dot or a space")
		case strings.ContainsAny(part, `<>"|?*`):
			return errors.New("a component has a character Windows refuses")
		case reservedNames[strings.ToLower(strings.SplitN(part, ".", 2)[0])]:
			return errors.New("a component is a Windows device name")
		}
	}

	return nil
}
