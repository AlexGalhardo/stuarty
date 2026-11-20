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

// Whatever the bytes, the ingest accepts a table exactly when the export parser does, and its refusal names a
// reason, never a word of the file (VV-287, T-92).
func TestCheckCubeAgreesWithTheExportParserAndEchoesNothing(t *testing.T) {
	t.Parallel()

	const canary = "CANARYSECRET"

	eight := strings.Repeat("0 0.5 1\n", 8)
	bodies := map[string]string{
		"valid":                  "LUT_3D_SIZE 2\n" + eight,
		"title and domains":      "TITLE \"t\"\nDOMAIN_MIN 0 0 0\nDOMAIN_MAX 1 1 1\nLUT_3D_SIZE 2\n" + eight,
		"header after the data":  "LUT_3D_SIZE 2\n" + eight + "TITLE \"late\"\n",
		"byte order mark":        "\xef\xbb\xbfLUT_3D_SIZE 2\n" + eight,
		"lower-case keyword":     "lut_3d_size 2\n" + eight,
		"1D table":               "LUT_1D_SIZE 2\n0 0 0\n1 1 1\n",
		"unknown keyword":        canary + " 1\nLUT_3D_SIZE 2\n" + eight,
		"unknown value":          "LUT_3D_SIZE 2\n" + strings.Repeat("0 0.5 "+canary+"\n", 8),
		"size 66":                "LUT_3D_SIZE 66\n",
		"size repeated":          "LUT_3D_SIZE 2\nLUT_3D_SIZE 2\n" + eight,
		"too few entries":        "LUT_3D_SIZE 2\n" + strings.Repeat("0 0 0\n", 7),
		"too many entries":       "LUT_3D_SIZE 2\n" + eight + "0 0 0\n",
		"not a finite number":    "LUT_3D_SIZE 2\n" + strings.Repeat("0 0 1e999\n", 8),
		"hexadecimal float":      "LUT_3D_SIZE 2\n" + strings.Repeat("0 0 0x1p-1\n", 8),
		"input range":            "LUT_3D_INPUT_RANGE 0 1\nLUT_3D_SIZE 2\n" + eight,
		"comment after the data": "LUT_3D_SIZE 2\n" + strings.Repeat("0 0.5 1 # "+canary+"\n", 8),
	}

	for name, body := range bodies {
		_, ingestErr := checkCube(strings.NewReader(body))
		_, exportErr := lut.Parse(strings.NewReader(body))

		if (ingestErr == nil) != (exportErr == nil) {
			t.Errorf("%s: ingest err = %v, export err = %v, want the same answer", name, ingestErr, exportErr)
		}

		if ingestErr != nil && strings.Contains(ingestErr.Error(), canary) {
			t.Errorf("%s: the refusal %q carries the content of the file", name, ingestErr)
		}
	}
}
