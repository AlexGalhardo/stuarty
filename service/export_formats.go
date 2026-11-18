package service

import (
	"strings"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

// The formats of an export job (SDD-003 section 2.8, SDD-004 section 8.1). The empty
// format is mp4.
const (
	FormatMP4 = "mp4"
	FormatGIF = "gif"
	FormatWAV = "wav"
	FormatAAC = "aac"
	// FormatPNG is one frame at the size of the project, chosen with args.at (VV-037).
	FormatPNG = "png"
)

// ExportFormats are the export formats in the order the help lists them; the default
// (mp4) comes first. The MCP definition of job_start must list the same values.
var ExportFormats = []string{FormatMP4, FormatGIF, FormatWAV, FormatAAC, FormatPNG}

// JoinOr writes names as "a, b or c", the way the hints and the help list choices.
func JoinOr(names []string) string {
	if len(names) < 2 {
		return strings.Join(names, "")
	}

	return strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1]
}

// The ceilings of an exported project (SDD-007 section 9.1). The document schema already
// holds them; the export checks them again before any work.
const (
	exportMaxSide = 4096
	exportMaxFPS  = 120
)

type exportFormat struct {
	ext         string
	contentType string
}

var exportFormats = map[string]exportFormat{
	FormatMP4: {".mp4", "video/mp4"},
	FormatGIF: {".gif", "image/gif"},
	FormatWAV: {".wav", "audio/wav"},
	FormatAAC: {".m4a", "audio/mp4"},
	FormatPNG: {".png", "image/png"},
}

// exportFormatOf returns the format of an export, mp4 for the empty one; ok is false for a
// name that is not a format.
func exportFormatOf(name string) (exportFormat, string, bool) {
	if name == "" {
		name = FormatMP4
	}

	f, ok := exportFormats[name]

	return f, name, ok
}

// checkExportFormat validates the format argument of an export.
func checkExportFormat(name string) (string, *Error) {
	_, norm, ok := exportFormatOf(name)
	if !ok {
		return "", newError(command.CodeValidation, "the export format %q is not supported.", name).
			withPath("args.format").withHint("use %s", JoinOr(ExportFormats))
	}

	return norm, nil
}

// checkExportSettings refuses a project beyond the size and frame rate ceilings with
// E_LIMIT, before a job row exists. Whether the encoder of this machine can do 4K or 60 fps
// is the engine's test encode, which ends the job with reason unsupported_size.
func checkExportSettings(st *timeline.Settings) *Error {
	over := func(what string, value, ceiling int) *Error {
		return newError(command.CodeLimit, "the project %s is %d; an export allows at most %d.", what, value, ceiling).
			withHint("lower the %s of the project, then export again", what).
			withDetail("limit", what).withDetail("value", value).withDetail("ceiling", ceiling)
	}

	switch {
	case st.Width > exportMaxSide:
		return over("width", st.Width, exportMaxSide)
	case st.Height > exportMaxSide:
		return over("height", st.Height, exportMaxSide)
	case st.FPS.Den > 0 && st.FPS.Num > exportMaxFPS*st.FPS.Den:
		return over("fps", (st.FPS.Num+st.FPS.Den-1)/st.FPS.Den, exportMaxFPS)
	}

	return nil
}

// exportFileName is the name of the file of a job in exports/<project>/.
func exportFileName(jobID, format string) string {
	f, _, _ := exportFormatOf(format)

	return jobID + f.ext
}

// formatName is the format a result reports: mp4 for an input stored without one.
func formatName(format string) string {
	_, norm, _ := exportFormatOf(format)

	return norm
}
