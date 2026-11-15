package timeline

import "fmt"

func (v *validator) projectFont(family string, f ProjectFont) {
	path := "fonts." + family
	if len(f.Files) == 0 {
		v.add(VShape, path+".files", family, "a font needs at least one file")
	}
	for i, ff := range f.Files {
		if ff.Weight < 100 || ff.Weight > 900 || ff.Weight%100 != 0 {
			v.add(
				VShape,
				fmt.Sprintf("%s.files[%d].weight", path, i),
				family,
				"weight must be 100 to 900 in steps of 100",
			)
		}
		if !hashPattern.MatchString(ff.Hash) {
			v.add(
				VShape,
				fmt.Sprintf("%s.files[%d].hash", path, i),
				family,
				"hash must be sha256: followed by 64 lower-case hex characters",
			)
		}
	}
}
