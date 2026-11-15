package command

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func init() {
	Register(Define(Spec{
		Name:    "font.add",
		Summary: "Adds a user font family to the project, or more weights to one it already has.",
		Description: "Names a font that is already stored by hash in the workspace as a family of the project, so that " +
			"styles can use it. The family name must not be a bundled family. Adding to an existing family adds the " +
			"weights and replaces the file of a weight it already has. The command never reads or stores a file; the " +
			"service refuses a hash that the workspace does not hold.",
		Example: `{"family": "Brand Sans", "files": [{"weight": 400, "hash": "sha256:` + strings.Repeat("ab", 32) +
			`"}, {"weight": 700, "hash": "sha256:` + strings.Repeat("cd", 32) + `"}]}`,
		Errors: []string{CodeValidation, CodeNotFound},
		Scope:  ScopeMedia,
		Phase:  Beta,
	}, applyFontAdd))

	Register(Define(Spec{
		Name:    "font.remove",
		Summary: "Removes a user font family from the project.",
		Description: "Removes the family from the fonts of the project. A family that a text style, a caption style or " +
			"a track still uses cannot be removed. The font files stay in the workspace.",
		Example: `{"family": "Brand Sans"}`,
		Errors:  []string{CodeValidation, CodeNotFound, CodeInUse},
		Scope:   ScopeMedia,
		Phase:   Beta,
	}, applyFontRemove))
}

var (
	fontFamilyForm = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,59}$`)
	fontHashForm   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type fontFileArg struct {
	Weight int    `json:"weight" desc:"100 to 900 in steps of 100."                                                               min:"100" max:"900"`
	Hash   string `json:"hash"   desc:"sha256: and 64 lower-case hex characters; a font stored by media import of the workspace."`
}

type fontAddArgs struct {
	Family string        `json:"family" desc:"1 to 60 characters: letters, digits, space, dot, hyphen, underscore; not a bundled family." min:"1" max:"60"`
	Files  []fontFileArg `json:"files"  desc:"One file per weight, 1 to 9; a weight appears once."                                        min:"1" max:"9"`
}

func applyFontAdd(c *Context, a *fontAddArgs) error {
	if !fontFamilyForm.MatchString(a.Family) {
		return Errorf(CodeValidation, "args.family %q is not a valid family name.", a.Family).
			WithHint("Use 1 to 60 letters, digits, spaces, dots, hyphens or underscores, starting with a letter or digit.").
			WithPath("args.family")
	}
	if slices.Contains(timeline.BundledFamilies, a.Family) {
		return Errorf(CodeValidation, "args.family %q is a bundled family.", a.Family).
			WithHint("Choose another name; the bundled families are: %s.", strings.Join(sortedBundled(), ", ")).
			WithPath("args.family")
	}
	seen := map[int]bool{}
	files := make([]timeline.FontFile, 0, len(a.Files))
	for i, f := range a.Files {
		path := fmt.Sprintf("args.files[%d]", i)
		switch {
		case f.Weight%100 != 0:
			return Errorf(CodeValidation, "%s.weight is %d, which is not a multiple of 100.", path, f.Weight).
				WithHint("Use 100, 200, ... 900.").WithPath(path + ".weight")
		case seen[f.Weight]:
			return Errorf(CodeValidation, "%s.weight %d is given twice.", path, f.Weight).
				WithHint("Send one file per weight.").WithPath(path + ".weight")
		case !fontHashForm.MatchString(f.Hash):
			return Errorf(CodeValidation, "%s.hash is not a sha256 hash.", path).
				WithHint("Use \"sha256:\" and 64 lower-case hex characters.").WithPath(path + ".hash")
		}
		seen[f.Weight] = true
		files = append(files, timeline.FontFile{Weight: f.Weight, Hash: f.Hash})
	}

	merged := slices.Clone(c.Doc.Fonts[a.Family].Files)
	for _, nf := range files {
		if i := slices.IndexFunc(merged, func(f timeline.FontFile) bool { return f.Weight == nf.Weight }); i >= 0 {
			merged[i].Hash = nf.Hash
			continue
		}
		merged = append(merged, nf)
	}
	slices.SortFunc(merged, func(x, y timeline.FontFile) int { return x.Weight - y.Weight })

	fonts := maps.Clone(c.Doc.Fonts)
	if fonts == nil {
		fonts = map[string]timeline.ProjectFont{}
	}
	pf := fonts[a.Family]
	pf.Files = merged
	fonts[a.Family] = pf
	c.Doc.Fonts = fonts

	weights, _ := timeline.FontWeights(c.Doc, a.Family)
	c.SetEffective("family", a.Family)
	c.SetEffective("weights", weights)
	return nil
}

func sortedBundled() []string {
	out := slices.Clone(timeline.BundledFamilies)
	slices.Sort(out)
	return out
}

type fontRemoveArgs struct {
	Family string `json:"family" desc:"A user family of the project."`
}

// fontUses lists the items and tracks whose style names the family, with the field.
func fontUses(d *timeline.Document, family string) (users, uses []string) {
	for _, id := range slices.Sorted(maps.Keys(d.Items)) {
		if s := d.Items[id].Style; s != nil && s.FontFamily != nil && *s.FontFamily == family {
			users, uses = append(users, id), append(uses, id+" (style.font_family)")
		}
	}
	for _, id := range slices.Sorted(maps.Keys(d.Tracks)) {
		if s := d.Tracks[id].CaptionStyle; s != nil && s.FontFamily != nil && *s.FontFamily == family {
			users, uses = append(users, id), append(uses, id+" (caption_style.font_family)")
		}
	}
	return users, uses
}

func applyFontRemove(c *Context, a *fontRemoveArgs) error {
	if _, ok := c.Doc.Fonts[a.Family]; !ok {
		if slices.Contains(timeline.BundledFamilies, a.Family) {
			return Errorf(CodeValidation, "Font family %q is bundled and cannot be removed.", a.Family).
				WithHint("Only the families added with font.add can be removed.").WithPath("args.family")
		}
		return Errorf(CodeNotFound, "Font family %q is not one of the project fonts.", a.Family).
			WithHint("Use one of: %s.", familyList(c.Doc)).WithPath("args.family")
	}
	if users, uses := fontUses(c.Doc, a.Family); len(users) > 0 {
		return Errorf(CodeInUse, "Font family %q is used by %s.", a.Family, strings.Join(uses, ", ")).
			WithHint("Change those styles to another font_family with text.set, caption.set_style and track.set_caption_style, then remove %q.",
				a.Family).
			WithDetail("used_by", users).WithPath("args.family")
	}
	fonts := maps.Clone(c.Doc.Fonts)
	delete(fonts, a.Family)
	if len(fonts) == 0 {
		fonts = nil
	}
	c.Doc.Fonts = fonts
	c.SetEffective("family", a.Family)
	return nil
}

func familyList(d *timeline.Document) string {
	if len(d.Fonts) == 0 {
		return "none (the project has no user fonts)"
	}
	return strings.Join(slices.Sorted(maps.Keys(d.Fonts)), ", ")
}
