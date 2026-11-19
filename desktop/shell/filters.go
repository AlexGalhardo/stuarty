package shell

import "strings"

// FileFilter is one entry of the native file dialog's type list.
type FileFilter struct {
	Name    string
	Pattern string
}

var (
	videoExts = []string{"mp4", "mov", "mkv", "webm", "avi", "m4v", "mpg", "mpeg", "mts", "m2ts", "wmv", "flv"}
	audioExts = []string{"mp3", "wav", "m4a", "aac", "flac", "ogg", "oga", "opus", "aiff", "wma"}
	imageExts = []string{"png", "jpg", "jpeg", "gif", "webp", "bmp", "tif", "tiff", "heic", "avif"}
)

func pattern(groups ...[]string) string {
	var parts []string
	for _, g := range groups {
		for _, ext := range g {
			parts = append(parts, "*."+ext, "*."+strings.ToUpper(ext))
		}
	}
	return strings.Join(parts, ";")
}

// TemplateFilters lists the dialog filters for importing a template file (a zip
// named <id>.stuarty-template.zip, SDD-005 section 15), then any file.
func TemplateFilters() []FileFilter {
	return []FileFilter{
		{"Vukem templates", "*.zip"},
		{"All files", "*.*"},
	}
}

// MediaFilters lists the dialog filters for importing media: everything
// supported first (the default), then each kind, then any file, because the
// engine decides by content and an unusual extension must stay reachable.
func MediaFilters() []FileFilter {
	return []FileFilter{
		{"Video, audio and images", pattern(videoExts, audioExts, imageExts)},
		{"Video", pattern(videoExts)},
		{"Audio", pattern(audioExts)},
		{"Images", pattern(imageExts)},
		{"All files", "*.*"},
	}
}
