package recording

import "testing"

func TestJudgeChunk(t *testing.T) {
	t.Parallel()

	cases := []struct {
		next, index int
		want        ChunkVerdict
	}{
		{0, 0, ChunkAccept},
		{5, 5, ChunkAccept},
		{5, 4, ChunkRetry},
		{5, 0, ChunkRetry},
		{5, 6, ChunkOutOfOrder},
		{0, 1, ChunkOutOfOrder},
		{0, -1, ChunkMalformed},
		{5, -1, ChunkMalformed},
		{0, MaxChunksPerTrack, ChunkTooMany},
		{MaxChunksPerTrack - 1, MaxChunksPerTrack - 1, ChunkAccept},
	}

	for _, c := range cases {
		if got := JudgeChunk(c.next, c.index); got != c.want {
			t.Errorf("JudgeChunk(%d, %d) = %s, want %s", c.next, c.index, got, c.want)
		}
	}
}

func TestValidateUpload(t *testing.T) {
	t.Parallel()

	ok := UploadSpec{ProjectID: "prj_0123456789", Container: "webm"}
	if f := ValidateUpload(ok); len(f) != 0 {
		t.Errorf("a valid request was refused: %v", f)
	}

	bad := ValidateUpload(UploadSpec{ProjectID: "x", Container: "avi"})
	if len(bad) != 2 || bad[0].Field != "project_id" || bad[1].Field != "container" {
		t.Errorf("fields = %v", bad)
	}
}

func TestUploadTracksNameTheFileOfTheContainer(t *testing.T) {
	t.Parallel()

	tr := UploadTracks(UploadSpec{Container: "mp4", Camera: true})
	if len(tr) != 2 || tr[0].File != "capture.mp4" || tr[1].File != "camera.mp4" {
		t.Errorf("tracks = %+v", tr)
	}

	if tr := UploadTracks(UploadSpec{Container: "webm"}); len(tr) != 1 || tr[0].Kind != TrackScreen {
		t.Errorf("a screen only upload = %+v", tr)
	}

	if ContainerWebM.File(TrackMicrophone) != "" {
		t.Error("a sound track has no uploaded file of its own")
	}

	if UploadKind(TrackMicrophone) || UploadKind(TrackSystem) || !UploadKind(TrackScreen) || !UploadKind(TrackCamera) {
		t.Error("only the screen and the camera are uploaded")
	}
}

func TestValidSHA256(t *testing.T) {
	t.Parallel()

	good := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if !ValidSHA256(good) {
		t.Error("the digest of the empty string was refused")
	}

	for _, bad := range []string{"", good[:63], good + "0", "E3B0C44298FC1C149AFBF4C8996FB92427AE41E4649B934CA495991B7852B855", good[:63] + "g"} {
		if ValidSHA256(bad) {
			t.Errorf("%q was accepted", bad)
		}
	}
}

func TestFitsAfter(t *testing.T) {
	t.Parallel()

	l := Limits{MaxBytes: 100}
	if !l.FitsAfter(60, 40) || l.FitsAfter(60, 41) {
		t.Error("the byte limit is inclusive")
	}
}
