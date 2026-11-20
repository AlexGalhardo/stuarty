package service

import (
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/engine/lut"
)

func TestSniffCube(t *testing.T) {
	t.Parallel()

	yes := []string{
		"LUT_3D_SIZE 33\n",
		"# c\n\nTITLE \"x\"\nLUT_3D_SIZE 2\n",
		"\xef\xbb\xbfDOMAIN_MIN 0 0 0\n",
		"TITLE\tx\n",
	}
	no := []string{"", "LUT_3D_SIZE 33", "hello\nworld\n", "# only comments\n", "RIFF\x00\x00\n", "LUT_3D_SIZE 2\x00\n"}

	for _, s := range yes {
		if !sniffCube([]byte(s)) {
			t.Errorf("%q is a .cube header", s)
		}
	}

	for _, s := range no {
		if sniffCube([]byte(s)) {
			t.Errorf("%q is not a .cube header", s)
		}
	}
}

// The ingest and the export read a table with one parser, so they hold it to one set of limits (VV-287).
func TestCheckCubeHoldsTheLimitsOfTheExportParser(t *testing.T) {
	t.Parallel()

	eight := strings.Repeat("0 0.5 1\n", 8)
	cases := []struct {
		name, body, want string
	}{
		{
			"past the size the export reads",
			"LUT_3D_SIZE 2\n" + strings.Repeat("# padding padding padding padding\n", lut.MaxBytes/34+10),
			"too large",
		},
		{"a line the export refuses", "LUT_3D_SIZE 2\n# " + strings.Repeat("x", 5000) + "\n" + eight, "too long"},
		{"a domain that is not 0 to 1", "LUT_3D_SIZE 2\nDOMAIN_MAX 2 2 2\n" + eight, "domain"},
	}

	for _, c := range cases {
		if _, err := checkCube(strings.NewReader(c.body)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want a refusal that says %q", c.name, err, c.want)
		}
	}

	if n, err := checkCube(strings.NewReader("LUT_3D_SIZE 2\n" + eight)); err != nil || n != 2 {
		t.Errorf("valid table = %d, %v", n, err)
	}
}
