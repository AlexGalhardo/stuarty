package export

import (
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
)

func TestSameRateReadsTheNominalRate(t *testing.T) {
	t.Parallel()

	fps := timeline.Rational{Num: 30, Den: 1}

	for _, tc := range []struct {
		got  string
		want bool
	}{
		{"30/1", true},
		{"60/2", true},
		{"30000/1001", false},
		{"460800/14849", false}, // the avg_frame_rate of a VideoToolbox file, which is why it is not used
		{"0/0", false},
		{"", false},
	} {
		if ok := sameRate(tc.got, fps); ok != tc.want {
			t.Errorf("sameRate(%q) = %v, want %v", tc.got, ok, tc.want)
		}
	}
}
