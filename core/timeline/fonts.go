package timeline

import "fmt"

// projectFont checks one entry of the fonts map (SDD-001 §3.9): a family that does
// not take the name of a bundled one, and one file per weight.
func (v *validator) projectFont(family string, f ProjectFont) {
	path := "fonts." + family
	if _, bundled := BundledWeights[family]; bundled {
		v.add(VFont, path, family, "font family %q is a bundled family; choose another name", family)
	}
	if len(f.Files) == 0 {
		v.add(VShape, path+".files", family, "a font needs at least one file")
	}
	seen := map[int]bool{}
	for i, ff := range f.Files {
		fp := fmt.Sprintf("%s.files[%d]", path, i)
		if ff.Weight < 100 || ff.Weight > 900 || ff.Weight%100 != 0 {
			v.add(VShape, fp+".weight", family, "weight must be 100 to 900 in steps of 100")
		} else if seen[ff.Weight] {
			v.add(VFont, fp+".weight", family, "font %q has weight %d twice", family, ff.Weight)
		}
		seen[ff.Weight] = true
		if !hashPattern.MatchString(ff.Hash) {
			v.add(VShape, fp+".hash", family, "hash must be sha256: followed by 64 lower-case hex characters")
		}
	}
}
