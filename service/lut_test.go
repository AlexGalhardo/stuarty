package service

import (
	"strings"
	"testing"
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

func TestCheckCubeRefusesAFilePastTheCap(t *testing.T) {
	t.Parallel()

	body := "LUT_3D_SIZE 2\n" + strings.Repeat(
		"# padding padding padding padding padding padding\n",
		cubeMaxBytes/48+10,
	)
	if _, err := checkCube(strings.NewReader(body)); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Errorf("err = %v, want the size refusal", err)
	}

	if n, err := checkCube(
		strings.NewReader("LUT_3D_SIZE 2\n" + strings.Repeat("0 0.5 1\n", 8)),
	); err != nil ||
		n != 2 {
		t.Errorf("valid table = %d, %v", n, err)
	}
}
