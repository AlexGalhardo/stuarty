package service

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
)

// Limits of the brand kit (SDD-005 section 5.11).
const (
	maxBrandColors = 32
	maxBrandFonts  = 16
	maxBrandWeight = 9
)

var (
	brandColorName  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	brandColorValue = regexp.MustCompile(`^#(?:[0-9A-Fa-f]{6}|[0-9A-Fa-f]{8})$`)
	brandFamilyForm = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,59}$`)
)

// BrandFontFile is one weight of a brand font, stored by hash in the font store.
type BrandFontFile struct {
	Weight int    `json:"weight"`
	Hash   string `json:"hash"`
}

// BrandFont is a family of the brand kit.
type BrandFont struct {
	Files []BrandFontFile `json:"files"`
}

// BrandKitGetRequest has no members.
type BrandKitGetRequest struct{}

// BrandKitSetRequest replaces the brand kit (SDD-005 section 5.11). Every member is optional, but at least one must
// be present: an empty member (an empty map) clears it, a body with none is refused.
type BrandKitSetRequest struct {
	Logo   string               `json:"logo,omitempty"` // sha256: and 64 hex digits of an image asset
	Colors map[string]string    `json:"colors,omitempty"`
	Fonts  map[string]BrandFont `json:"fonts,omitempty"`
}

// BrandKit is the brand kit of the workspace, or of the user in network mode.
type BrandKit struct {
	Logo      string               `json:"logo,omitempty"`
	Colors    map[string]string    `json:"colors,omitempty"`
	Fonts     map[string]BrandFont `json:"fonts,omitempty"`
	UpdatedMs int64                `json:"updated_ms,omitempty"`
	Text      string               `json:"text"`
}

// brandKitDoc is the stored form: the three members, nothing else.
type brandKitDoc struct {
	Logo   string               `json:"logo,omitempty"`
	Colors map[string]string    `json:"colors,omitempty"`
	Fonts  map[string]BrandFont `json:"fonts,omitempty"`
}

func brandKitRefused(c Caller) error {
	if c.ProjectID == "" {
		return nil
	}

	return newError(CodeForbidden, "the key is limited to one project and cannot use the brand kit.").
		withHint("use a key without a project limit")
}

// BrandKitGet returns the brand kit of the caller, empty when none was set.
func (s *svc) BrandKitGet(ctx context.Context, _ BrandKitGetRequest) (*BrandKit, error) {
	c, err := authorize(ctx, "BrandKitGet", "")
	if err != nil {
		return nil, err
	}

	if err := brandKitRefused(c); err != nil {
		return nil, err
	}

	kit, lerr := s.loadBrandKit(ctx, s.store, c)
	if lerr != nil {
		return nil, lerr
	}

	return kit, nil
}

// loadBrandKit reads the kit of the caller; an absent row is an empty kit.
func (s *svc) loadBrandKit(ctx context.Context, repos Repos, c Caller) (*BrandKit, *Error) {
	row, err := repos.BrandKit().Get(ctx, c.OwnerUserID)
	if isNotFound(err) {
		return brandKitView(&BrandKit{}), nil
	}

	if err != nil {
		return nil, asErr(err)
	}

	var doc brandKitDoc
	if json.Unmarshal(row.Document, &doc) != nil {
		return nil, internalError()
	}

	return brandKitView(&BrandKit{Logo: doc.Logo, Colors: doc.Colors, Fonts: doc.Fonts, UpdatedMs: row.UpdatedMs}), nil
}

func brandKitView(k *BrandKit) *BrandKit {
	var parts []string

	if k.Logo != "" {
		parts = append(parts, "a logo")
	}

	parts = append(parts, fmt.Sprintf("%d colours", len(k.Colors)), fmt.Sprintf("%d font families", len(k.Fonts)))
	k.Text = "brand kit: " + strings.Join(parts, ", ")

	return k
}

// BrandKitSet validates the kit and replaces the stored one.
func (s *svc) BrandKitSet(ctx context.Context, req BrandKitSetRequest) (*BrandKit, error) {
	c, err := authorize(ctx, "BrandKitSet", "")
	if err != nil {
		return nil, err
	}

	if err := brandKitRefused(c); err != nil {
		return nil, err
	}

	kit, verr := normalizeBrandKit(&req)
	if verr != nil {
		return nil, verr
	}

	if kerr := s.checkBrandKitFiles(ctx, c, kit); kerr != nil {
		return nil, kerr
	}

	doc, merr := json.Marshal(brandKitDoc{Logo: kit.Logo, Colors: kit.Colors, Fonts: kit.Fonts})
	if merr != nil {
		return nil, internalError()
	}

	now := s.nowMs()

	werr := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		if err := tx.BrandKit().Put(ctx, BrandKitRow{
			OwnerUserID: c.OwnerUserID, Document: doc, UpdatedMs: now, UpdatedBy: c.Actor,
		}); err != nil {
			return err
		}

		return appendAudit(ctx, tx, now, c.Actor, "brand.set", "", "")
	})
	if werr != nil {
		return nil, asError(werr)
	}

	kit.UpdatedMs = now

	return brandKitView(kit), nil
}

// normalizeBrandKit checks the form of every member and returns the kit with upper-case colours.
func normalizeBrandKit(req *BrandKitSetRequest) (*BrandKit, *Error) {
	// A body with no member would silently empty the kit; clearing it is asked for with an explicit
	// empty member, such as {"colors": {}}.
	if req.Logo == "" && req.Colors == nil && req.Fonts == nil {
		return nil, newError(
			command.CodeValidation,
			`the brand kit body has none of logo, colors or fonts; to clear the kit send an explicit empty member, such as {"colors": {}}.`,
		)
	}

	if req.Logo != "" && !isFontHash(req.Logo) {
		return nil, newError(command.CodeValidation, "logo is not a sha256 hash.").withPath("logo").
			withHint(`use "sha256:" and 64 lower-case hex characters of an image asset; the hash is in the answer of ` +
				`media:import and in the list of GET /api/v1/assets`)
	}

	if len(req.Colors) > maxBrandColors {
		return nil, newError(
			command.CodeValidation, "the brand kit has %d colours; at most %d.", len(req.Colors), maxBrandColors,
		).withPath("colors")
	}

	if len(req.Fonts) > maxBrandFonts {
		return nil, newError(
			command.CodeValidation, "the brand kit has %d font families; at most %d.", len(req.Fonts), maxBrandFonts,
		).withPath("fonts")
	}

	kit := &BrandKit{Logo: req.Logo}

	for _, name := range sortedKeys(req.Colors) {
		value := req.Colors[name]

		switch {
		case !brandColorName.MatchString(name):
			return nil, newError(
				command.CodeValidation, "colors %q is not a valid name; it must match [a-z][a-z0-9_]{0,39}.", name,
			).withPath("colors").withHint("use lower case letters, digits and underscores")
		case !brandColorValue.MatchString(value):
			return nil, newError(command.CodeValidation, "colors.%s is not a colour.", name).
				withPath("colors." + name).withHint("use #RRGGBB or #RRGGBBAA")
		}

		if kit.Colors == nil {
			kit.Colors = map[string]string{}
		}

		kit.Colors[name] = strings.ToUpper(value)
	}

	for _, family := range sortedKeys(req.Fonts) {
		files, e := normalizeBrandFont(family, req.Fonts[family])
		if e != nil {
			return nil, e
		}

		if kit.Fonts == nil {
			kit.Fonts = map[string]BrandFont{}
		}

		kit.Fonts[family] = BrandFont{Files: files}
	}

	return kit, nil
}

func normalizeBrandFont(family string, f BrandFont) ([]BrandFontFile, *Error) {
	path := "fonts." + family

	switch {
	case !brandFamilyForm.MatchString(family):
		return nil, newError(command.CodeValidation, "fonts %q is not a valid family name.", family).
			withPath("fonts").withHint("use 1 to 60 letters, digits, spaces, dots, hyphens or underscores")
	case slices.Contains(timeline.BundledFamilies, family):
		return nil, newError(command.CodeValidation, "fonts %q is a bundled family.", family).
			withPath(path).withHint("choose another name for the family")
	case len(f.Files) < 1 || len(f.Files) > maxBrandWeight:
		return nil, newError(
			command.CodeValidation, "fonts.%s has %d files; it needs 1 to %d.", family, len(f.Files), maxBrandWeight,
		).withPath(path + ".files")
	}

	seen := map[int]bool{}
	files := make([]BrandFontFile, 0, len(f.Files))

	for i, file := range f.Files {
		fpath := fmt.Sprintf("%s.files[%d]", path, i)

		switch {
		case file.Weight < 100 || file.Weight > 900 || file.Weight%100 != 0:
			return nil, newError(
				command.CodeValidation, "%s.weight is %d; it must be 100 to 900 in steps of 100.", fpath, file.Weight,
			).withPath(fpath + ".weight")
		case seen[file.Weight]:
			return nil, newError(command.CodeValidation, "%s.weight %d is given twice.", fpath, file.Weight).
				withPath(fpath + ".weight").withHint("send one file per weight")
		case !isFontHash(file.Hash):
			return nil, newError(command.CodeValidation, "%s.hash is not a sha256 hash.", fpath).
				withPath(fpath + ".hash").withHint(`use "sha256:" and 64 lower-case hex characters`)
		}

		seen[file.Weight] = true
		files = append(files, file)
	}

	slices.SortFunc(files, func(a, b BrandFontFile) int { return a.Weight - b.Weight })

	return files, nil
}

// checkBrandKitFiles refuses a font the caller may not use (E_NOT_FOUND, the refusal of font.add)
// and a logo that is not an image asset the caller can see.
func (s *svc) checkBrandKitFiles(ctx context.Context, c Caller, kit *BrandKit) *Error {
	var scope fontScope

	for _, family := range sortedKeys(kit.Fonts) {
		for i, f := range kit.Fonts[family].Files {
			if !scope.allowed(ctx, s.store, s, f.Hash) {
				return fontNotUsable(fmt.Sprintf("fonts.%s.files[%d]", family, i))
			}
		}
	}

	if kit.Logo == "" {
		return nil
	}

	notAnImage := newError(command.CodeNotFound, "the logo is not an image of the workspace.").
		withPath("logo").withHint("import the image into a project first")

	if s.media.Content == nil || !s.unshippedAllowed(ctx, c, kit.Logo) {
		return notAnImage
	}

	a, err := s.store.Assets().Get(ctx, kit.Logo)
	if err != nil || (a.Kind != "" && a.Kind != string(timeline.AssetImage)) {
		return notAnImage
	}

	return nil
}

// applyBrandKit sets the colours and fonts that a template instantiation asked for from the brand kit
// of the caller (SDD-005 section 5.11). It changes doc, which the caller just made; nothing is stored.
func (s *svc) applyBrandKit(
	ctx context.Context, c Caller, doc *timeline.Document, req *TemplateInstantiateRequest,
) *Error {
	if len(req.BrandColors) == 0 && len(req.BrandFonts) == 0 {
		return nil
	}

	kit, lerr := s.loadBrandKit(ctx, s.store, c)
	if lerr != nil {
		return lerr
	}

	missing := func(path, reason, format string, args ...any) *Error {
		return newError(command.CodeValidation, format, args...).withPath(path).withDetail("reason", reason).
			withHint("set it with brand:set, or leave it out of the request")
	}

	for i, name := range req.BrandColors {
		path := fmt.Sprintf("brand_colors[%d]", i)

		if _, ok := doc.Template.Colors[name]; !ok {
			return missing(path, "brand_color_unknown", "the template has no colour named %q.", name)
		}

		value, ok := kit.Colors[name]
		if !ok {
			return missing(path, "brand_color_missing", "the brand kit has no colour named %q.", name)
		}

		doc.Template.Colors[name] = timeline.Color(value)
	}

	for i, family := range req.BrandFonts {
		bf, ok := kit.Fonts[family]
		if !ok {
			return missing(
				fmt.Sprintf("brand_fonts[%d]", i), "brand_font_missing", "the brand kit has no font family %q.", family,
			)
		}

		files := make([]timeline.FontFile, 0, len(bf.Files))
		for _, f := range bf.Files {
			files = append(files, timeline.FontFile{Weight: f.Weight, Hash: f.Hash})
		}

		if doc.Fonts == nil {
			doc.Fonts = map[string]timeline.ProjectFont{}
		}

		doc.Fonts[family] = timeline.ProjectFont{Files: files}
	}

	if vs := timeline.Validate(doc); len(vs) > 0 {
		return newError(command.CodeValidation, "the brand kit does not fit the template: %s", vs[0].String()).
			withHint("check the weights of the brand fonts against the styles of the template")
	}

	return nil
}
