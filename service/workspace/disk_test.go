package workspace

import "testing"

func TestFreeBytesOfTheWorkspaceVolume(t *testing.T) {
	n, err := FreeBytes(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	if n == 0 {
		t.Fatal("a volume with no free space cannot run this test")
	}
}
