package service

import (
	"testing"

	"github.com/AlexGalhardo/stuarty/core/lint"
)

func TestSilentAudioIsError(t *testing.T) {
	t.Parallel()

	in := []lint.Finding{{Code: lint.CodeSilentExport, Severity: lint.SeverityWarning}}

	for _, format := range []string{"wav", "aac"} {
		if got := silentAudioIsError(in, format); got[0].Severity != lint.SeverityError {
			t.Errorf("%s: severity = %q, want error", format, got[0].Severity)
		}
	}

	for _, format := range []string{"", "mp4", "gif"} {
		if got := silentAudioIsError(in, format); got[0].Severity != lint.SeverityWarning {
			t.Errorf("%s: severity = %q, want warning", format, got[0].Severity)
		}
	}

	if in[0].Severity != lint.SeverityWarning {
		t.Error("the input slice was modified")
	}
}
