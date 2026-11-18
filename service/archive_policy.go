package service

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image/jpeg" // registers the decoder that validates preview.jpg
	"io"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/engine/rasterize"
)

// The limits of a template file (SDD-007 section 7.6 item 1 and 2).
const (
	templateFileMax      = 64 << 20
	templateEntriesMax   = 64
	templateTotalMax     = 256 << 20
	templateEntryMax     = 32 << 20
	templateManifestMax  = 64 << 10
	templateDocumentMax  = 4 << 20
	templatePreviewMax   = 2 << 20
	templateJSONDepth    = 64
	templateBombRatio    = 1000
	templatePreviewSide  = 8192
	templateFormat       = "vukem-video-template"
	templateFileVersion  = 1
	templateManifestName = "manifest.json"
	templateDocName      = "template.json"
	templatePreviewName  = "preview.jpg"
)

// templateEntryCaps is the cap of each fixed allowlisted name; anything else is refused, except a
// font entry (templateFontEntry).
var templateEntryCaps = map[string]int64{
	templateManifestName: templateManifestMax,
	templateDocName:      templateDocumentMax,
	templatePreviewName:  templatePreviewMax,
}

// templateFontEntry is the name of a user font in a template file: "font." and 64 lower-case hex
// digits of the SHA-256 of the file (SDD-007 section 7.6 item 2). Each is at most one font file.
var templateFontEntry = regexp.MustCompile(`^font\.[0-9a-f]{64}$`)

// templateEntryCap is the cap of an allowlisted entry name.
func templateEntryCap(name string) (int64, bool) {
	if c, ok := templateEntryCaps[name]; ok {
		return c, true
	}

	if templateFontEntry.MatchString(name) {
		return rasterize.MaxFontBytes, true
	}

	return 0, false
}

func templateFontName(hash string) string { return "font." + strings.TrimPrefix(hash, "sha256:") }

// templateManifest is manifest.json of a template file.
type templateManifest struct {
	Format          string `json:"format"`
	TemplateVersion int    `json:"template_version"`
	AppVersion      string `json:"app_version"`
	SchemaVersion   int    `json:"schema_version"`
	Name            string `json:"name"`
}

// templateContents is what a template archive holds after the policy.
type templateContents struct {
	Manifest templateManifest
	Document []byte
	Preview  []byte
	// Fonts are the font entries, by hash in the form of a document, each checked against its name.
	Fonts map[string][]byte
}

// archiveRefused is the refusal of an archive: always E_VALIDATION, and never
// the raw name of an entry, which is attacker-controlled text.
func archiveRefused(reason string) *Error {
	return newError(command.CodeValidation, "the template file is refused: %s.", reason).
		withHint("export the template again with template:export").withDetail("reason", reason)
}

// foldName is the comparison form of an entry name: NFC, then case folded.
func foldName(n string) string { return strings.ToLower(norm.NFC.String(n)) }

// readTemplateArchive applies the archive policy of SDD-007 section 7.6 to the
// bytes of a template file and returns its three members. It writes nothing
// and never uses an entry name as a path.
func readTemplateArchive(data []byte) (*templateContents, *Error) {
	if len(data) > templateFileMax {
		return nil, archiveRefused("the file is over 64 MiB")
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, archiveRefused("it is not a readable zip")
	}

	if e := scanTemplateEntries(zr); e != nil {
		return nil, e
	}

	// The limits of SDD-007 section 7.6, checked on the declared sizes before
	// anything is read; readEntry then holds every entry to its cap.
	if len(zr.File) > templateEntriesMax {
		return nil, archiveRefused("it holds too many entries")
	}

	var total uint64

	for _, f := range zr.File {
		total += f.UncompressedSize64
		if f.UncompressedSize64 > templateEntryMax || total > templateTotalMax {
			return nil, archiveRefused("an entry or the total is larger than allowed")
		}
	}

	contents := &templateContents{Fonts: map[string][]byte{}}
	got := map[string]bool{}

	for _, en := range zr.File {
		canon, ok := allowlisted(en.Name)
		if !ok {
			return nil, archiveRefused("it holds an entry that a template file may not hold")
		}

		if got[canon] {
			return nil, archiveRefused("it holds the same entry twice")
		}

		got[canon] = true

		limit, _ := templateEntryCap(canon)

		body, rerr := readEntry(en, limit)
		if rerr != nil {
			return nil, archiveRefused("an entry is larger than it declares or cannot be read")
		}

		if templateFontEntry.MatchString(canon) {
			sum := sha256.Sum256(body)
			if hex.EncodeToString(sum[:]) != strings.TrimPrefix(canon, "font.") {
				return nil, archiveRefused("a font does not match its name")
			}

			contents.Fonts["sha256:"+strings.TrimPrefix(canon, "font.")] = body

			continue
		}

		switch canon {
		case templateManifestName:
			contents.Manifest, rerr = parseManifest(body)
		case templateDocName:
			contents.Document = body
			rerr = checkJSONDepth(body)
		case templatePreviewName:
			contents.Preview = body
			rerr = checkPreview(body)
		}

		if rerr != nil {
			return nil, archiveRefused(rerr.Error())
		}
	}

	if !got[templateManifestName] || !got[templateDocName] {
		return nil, archiveRefused("manifest.json and template.json are required")
	}

	return contents, nil
}

// scanTemplateEntries refuses what the size checks cannot see: symlinks,
// encrypted entries, an implausible compression ratio, and entries whose name
// is not on the allowlist once folded (which also catches a duplicate that
// differs only by case or normalisation).
func scanTemplateEntries(zr *zip.Reader) *Error {
	seen := map[string]bool{}

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			return archiveRefused("it holds a directory entry")
		}

		if f.Mode()&fs.ModeSymlink != 0 {
			return archiveRefused("it holds a symbolic link")
		}

		if f.Flags&0x1 != 0 {
			return archiveRefused("it holds an encrypted entry")
		}

		// Compared folded, so that "Manifest.json" next to "manifest.json" is a
		// duplicate and not two different names.
		folded := foldName(f.Name)
		if _, ok := templateEntryCap(folded); !ok {
			return archiveRefused("it holds an entry that a template file may not hold")
		}

		if seen[folded] {
			return archiveRefused("it holds the same entry twice")
		}

		seen[folded] = true

		if f.UncompressedSize64 > 64<<10 && f.UncompressedSize64 > templateBombRatio*max(f.CompressedSize64, 1) {
			return archiveRefused("an entry has an implausible compression ratio")
		}
	}

	return nil
}

// allowlisted tells whether name is exactly one of the allowed names.
func allowlisted(name string) (string, bool) {
	_, ok := templateEntryCap(name)

	return name, ok
}

// readEntry reads an entry up to its cap; a header that lies about the size, a
// checksum error and a stream longer than the cap all fail.
func readEntry(en *zip.File, limit int64) ([]byte, error) {
	size := int64(en.UncompressedSize64) //nolint:gosec // bounded by templateEntryMax above
	if size > limit {
		return nil, errors.New("over the cap")
	}

	rc, err := en.Open()
	if err != nil {
		return nil, err
	}

	defer func() { _ = rc.Close() }()

	body, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, err
	}

	if int64(len(body)) > limit || int64(len(body)) != size {
		return nil, errors.New("size mismatch")
	}

	return body, nil
}

// checkJSONDepth refuses a document nested deeper than the limit before any
// decoder builds it.
func checkJSONDepth(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	depth := 0

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}

		if err != nil {
			return errors.New("an entry is not valid JSON")
		}

		if d, ok := tok.(json.Delim); ok {
			if d == '{' || d == '[' {
				depth++
				if depth > templateJSONDepth {
					return errors.New("a JSON document is nested too deeply")
				}
			} else {
				depth--
			}
		}
	}
}

func parseManifest(b []byte) (templateManifest, error) {
	var m templateManifest

	if err := checkJSONDepth(b); err != nil {
		return m, err
	}

	if err := json.Unmarshal(b, &m); err != nil {
		return m, errors.New("manifest.json is not valid")
	}

	if m.Format != templateFormat || m.TemplateVersion < 1 {
		return m, errors.New("manifest.json is not a vukem-video template manifest")
	}

	return m, nil
}

// checkPreview validates the JPEG by decoding its header only.
func checkPreview(b []byte) error {
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > templatePreviewSide ||
		cfg.Height > templatePreviewSide {
		return errors.New("preview.jpg is not a valid JPEG")
	}

	return nil
}

// buildTemplateArchive writes the template file: manifest.json, template.json
// preview.jpg when there is one, and a font.<hex> entry per user font (by hash in the form of a
// document), with flat names.
func buildTemplateArchive(m templateManifest, doc, preview []byte, fonts map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer

	zw := zip.NewWriter(&buf)

	manifest, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}

	members := []struct {
		name string
		body []byte
	}{{templateManifestName, manifest}, {templateDocName, doc}}
	if len(preview) > 0 {
		members = append(members, struct {
			name string
			body []byte
		}{templatePreviewName, preview})
	}

	for _, hash := range slices.Sorted(maps.Keys(fonts)) {
		members = append(members, struct {
			name string
			body []byte
		}{templateFontName(hash), fonts[hash]})
	}

	for _, mem := range members {
		w, werr := zw.CreateHeader(&zip.FileHeader{Name: mem.name, Method: zip.Deflate})
		if werr != nil {
			return nil, werr
		}

		if _, werr = w.Write(mem.body); werr != nil {
			return nil, werr
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// migrateTemplateDocument parses, migrates and validates the document of a
// template file and checks that it carries no media (ADR-0022, SDD-007 7.6
// item 5).
func migrateTemplateDocument(data []byte) (*timeline.Document, *Error) {
	raw, err := timeline.DecodeRaw(data)
	if err != nil {
		return nil, newError(command.CodeValidation, "template.json is not a valid document.").
			withHint("export the template again with template:export")
	}

	if schemaVersionOf(raw) > timeline.CurrentSchemaVersion {
		return nil, newError(command.CodeValidation, "the template was made by a newer version of the editor.").
			withHint("update vukem-video, then import it again").withDetail("code", "V_SCHEMA_VERSION")
	}

	raw, err = timeline.Migrate(raw)
	if err != nil {
		return nil, newError(command.CodeValidation, "template.json cannot be migrated to this version.").
			withHint("export the template again with template:export")
	}

	doc, err := timeline.FromRaw(raw)
	if err != nil {
		return nil, newError(command.CodeValidation, "template.json is not a valid document.").
			withHint("export the template again with template:export")
	}

	if vs := timeline.Validate(doc); len(vs) > 0 {
		return nil, newError(command.CodeValidation, "template.json is not valid: %s", vs[0].String()).
			withDetail("code", vs[0].Code)
	}

	if doc.Template == nil {
		return nil, newError(command.CodeValidation, "template.json has no template member.")
	}

	if len(doc.Assets) > 0 {
		return nil, newError(command.CodeValidation, "a template carries no media.").
			withHint("save the template again from a project")
	}

	return doc, nil
}

// schemaVersionOf reads schema_version of a raw document, whatever number type
// the decoder used; 0 when it is missing or not a number.
func schemaVersionOf(raw timeline.RawDoc) int {
	switch v := raw["schema_version"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return 0
		}

		return int(n)
	}

	return 0
}
