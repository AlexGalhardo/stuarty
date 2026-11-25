package servicetest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/recordings"
)

// The recording a browser captures and uploads (SDD-025 section 17).

type upEnv struct{ *mediaEnv }

func newUpEnv(t *testing.T, realMedia bool) *upEnv {
	t.Helper()

	return &upEnv{newMediaEnv(t, mediaCfg{realIngest: realMedia, realExport: realMedia})}
}

func digest(b []byte) string {
	h := sha256.Sum256(b)

	return hex.EncodeToString(h[:])
}

// webmFixture is a few seconds of picture and tone in WebM, written as a live stream (no duration, no index), as
// MediaRecorder writes it.
func webmFixture(t *testing.T, seconds int, audio bool) []byte {
	t.Helper()

	inst := pinnedFFmpeg(t)
	dir := t.TempDir()
	n := strconv.Itoa(seconds)
	args := []string{
		"-hide_banner",
		"-v",
		"error",
		"-nostdin",
		"-y",
		"-f",
		"lavfi",
		"-i",
		"testsrc2=s=320x240:r=15:d=" + n,
	}

	if audio {
		args = append(args, "-f", "lavfi", "-i", "sine=f=440:r=48000:d="+n)
	}

	args = append(args, "-c:v", "libvpx-vp9", "-deadline", "realtime", "-cpu-used", "8", "-b:v", "300k")

	if audio {
		args = append(args, "-c:a", "libopus")
	}

	args = append(args, "-f", "webm", "-live", "1", "capture.webm")

	if res, err := inst.FFmpeg.Run(context.Background(), dir, args...); err != nil {
		t.Fatalf("the fixture could not be made: %v\n%s", err, res.Stderr)
	}

	b, err := os.ReadFile(filepath.Join(dir, "capture.webm"))
	if err != nil {
		t.Fatal(err)
	}

	return b
}

// split cuts b into n pieces at arbitrary points, as a recorder's time slices cut a stream.
func split(b []byte, n int) [][]byte {
	size := (len(b) + n - 1) / n

	var out [][]byte

	for len(b) > 0 {
		k := min(size, len(b))
		out = append(out, b[:k])
		b = b[k:]
	}

	return out
}

func (u *upEnv) begin(t *testing.T, project string, camera bool) *service.Recording {
	t.Helper()

	rec, err := u.svc.RecordingUploadBegin(personCtx(), service.RecordingUploadBeginRequest{
		ProjectID: project, Container: "webm", Camera: camera, Label: "Browser tab",
	})
	if err != nil {
		t.Fatalf("RecordingUploadBegin: %v", err)
	}

	return rec
}

func (u *upEnv) send(
	ctx context.Context, id, track string, index int, body []byte, elapsed int64,
) (*service.RecordingUploadChunkResult, error) {
	return u.svc.RecordingUploadChunk(ctx, service.RecordingUploadChunkRequest{
		ID: id, Track: track, Index: index, SHA256: digest(body), ElapsedMS: elapsed,
		Body: bytes.NewReader(body), Size: int64(len(body)),
	})
}

func (u *upEnv) mustSend(t *testing.T, id, track string, index int, body []byte, elapsed int64) {
	t.Helper()

	if _, err := u.send(personCtx(), id, track, index, body, elapsed); err != nil {
		t.Fatalf("chunk %d of %s: %v", index, track, err)
	}
}

func (u *upEnv) wait(t *testing.T, id, want string) *service.Recording {
	t.Helper()

	var rec *service.Recording

	pollUntil(60*time.Second, func() bool {
		var err error

		if rec, err = u.svc.RecordingGet(personCtx(), service.RecordingRef{ID: id}); err != nil {
			t.Fatal(err)
		}

		return rec.State == want
	})

	if rec.State != want {
		t.Fatalf("recording %s is %s (%+v), want %s", id, rec.State, rec.Error, want)
	}

	return rec
}

func (u *upEnv) parts(id string) []string {
	m, _ := filepath.Glob(filepath.Join(u.ws, "recordings", id, "parts", "screen", "*"))

	return m
}

func TestRecordingUploadRecordsFinishesConfirmsAndPlaces(t *testing.T) {
	u := newUpEnv(t, true)
	project := u.newProject("p")
	data := webmFixture(t, 3, true)
	chunks := split(data, 6)

	rec := u.begin(t, project, false)
	if rec.State != "recording" || rec.Source.Kind != "browser" || rec.Upload == nil ||
		rec.Upload.Container != "webm" || rec.Upload.MaxChunkBytes != 16<<20 || len(rec.Upload.Tracks) != 1 {
		t.Fatalf("begin = %+v %+v", rec, rec.Upload)
	}

	for i, c := range chunks {
		u.mustSend(t, rec.ID, "screen", i, c, int64(i+1)*500)
	}

	got, err := u.svc.RecordingGet(u.as(agent), service.RecordingRef{ID: rec.ID})
	if err != nil || got.Upload == nil || got.Upload.Tracks[0].NextIndex != len(chunks) ||
		got.Bytes != int64(len(data)) {
		t.Fatalf("an agent reads where the upload is: %+v %v", got, err)
	}

	fin, err := u.svc.RecordingUploadFinish(personCtx(), service.RecordingUploadFinishRequest{
		ID: rec.ID, Tracks: []service.RecordingUploadCount{{Kind: "screen", Chunks: len(chunks)}}, ElapsedMS: 3000,
	})
	if err != nil || (fin.State != "finishing" && fin.State != "ready") {
		t.Fatalf("finish = %+v %v", fin, err)
	}

	ready := u.wait(t, rec.ID, "ready")
	if ready.Review != "pending" || ready.Width != 320 || ready.Height != 240 || ready.StopReason != "stopped" ||
		ready.DurationMS < 2500 || ready.DurationMS > 3600 || len(ready.Tracks) != 1 || ready.Upload != nil {
		t.Fatalf("ready = %+v", ready)
	}

	if !pollUntil(5*time.Second, func() bool { return len(u.parts(rec.ID)) == 0 }) {
		t.Errorf("the chunks stay after the file was made: %v", u.parts(rec.ID))
	}

	if _, err := os.Stat(filepath.Join(u.ws, "recordings", rec.ID, "capture.webm")); err != nil {
		t.Fatal(err)
	}

	if _, err := u.svc.RecordingConfirm(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	added := u.wait(t, rec.ID, "ready")
	if added.Review != "added" || added.AssetID == "" {
		t.Fatalf("added = %+v", added)
	}

	res, err := u.svc.RecordingPlace(u.as(agent), service.RecordingPlaceRequest{ID: rec.ID})
	if err != nil || res.Revision < 2 {
		t.Fatalf("place = %+v %v", res, err)
	}
}

func TestRecordingUploadCarriesTheCameraOnItsOwnTrack(t *testing.T) {
	u := newUpEnv(t, true)
	project := u.newProject("p")
	screen, camera := webmFixture(t, 2, true), webmFixture(t, 2, false)

	rec := u.begin(t, project, true)
	if len(rec.Upload.Tracks) != 2 {
		t.Fatalf("tracks = %+v", rec.Upload.Tracks)
	}

	for i, c := range split(screen, 3) {
		u.mustSend(t, rec.ID, "screen", i, c, 0)
	}

	for i, c := range split(camera, 2) {
		u.mustSend(t, rec.ID, "camera", i, c, 0)
	}

	if _, err := u.svc.RecordingUploadFinish(personCtx(), service.RecordingUploadFinishRequest{
		ID: rec.ID, Tracks: []service.RecordingUploadCount{{Kind: "screen", Chunks: 3}, {Kind: "camera", Chunks: 2}},
	}); err != nil {
		t.Fatal(err)
	}

	ready := u.wait(t, rec.ID, "ready")
	if len(ready.Tracks) != 2 || ready.Tracks[0].Kind != "screen" || ready.Tracks[1].Kind != "camera" ||
		ready.Tracks[1].DurationMS <= 0 {
		t.Fatalf("tracks = %+v", ready.Tracks)
	}

	if _, err := u.svc.RecordingConfirm(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	added := u.wait(t, rec.ID, "ready")
	if len(added.Tracks) != 2 || added.Tracks[1].AssetID == "" {
		t.Fatalf("the camera is not an asset: %+v", added.Tracks)
	}
}

func TestRecordingUploadOrderRetryAndConflict(t *testing.T) {
	u := newUpEnv(t, false)
	project := u.newProject("p")
	rec := u.begin(t, project, false)

	a, b, c := []byte("first chunk"), []byte("second chunk"), []byte("third chunk")

	// a gap is refused and says what the track waits for
	_, err := u.send(personCtx(), rec.ID, "screen", 1, b, 0)
	if code(err) != "E_INVALID_STATE" || !strings.Contains(err.Error(), "waits for chunk 0") {
		t.Fatalf("a gap: %v", err)
	}

	var se *service.Error
	if !errors.As(err, &se) || se.Details["reason"] != "out_of_order" || se.Details["expected"] != 0 {
		t.Fatalf("details = %+v", se)
	}

	u.mustSend(t, rec.ID, "screen", 0, a, 0)
	u.mustSend(t, rec.ID, "screen", 1, b, 0)

	// the same chunk again, as after a lost answer, changes nothing
	dup, err := u.send(personCtx(), rec.ID, "screen", 1, b, 0)
	if err != nil || !dup.Duplicate || dup.NextIndex != 2 || dup.Bytes != int64(len(a)+len(b)) {
		t.Fatalf("a retry: %+v %v", dup, err)
	}

	// other bytes under a stored index are a conflict, and the stored chunk stays
	_, err = u.send(personCtx(), rec.ID, "screen", 1, []byte("other bytes!"), 0)
	if se = nil; code(err) != "E_INVALID_STATE" || !errors.As(err, &se) || se.Details["reason"] != "chunk_conflict" {
		t.Fatalf("a conflict: %v", err)
	}

	if have, _ := os.ReadFile(
		filepath.Join(u.ws, "recordings", rec.ID, "parts", "screen", "000001.part"),
	); !bytes.Equal(
		have,
		b,
	) {
		t.Fatalf("the stored chunk changed: %q", have)
	}

	// the track goes on from where it was
	got, err := u.svc.RecordingGet(personCtx(), service.RecordingRef{ID: rec.ID})
	if err != nil || got.Upload.Tracks[0].NextIndex != 2 {
		t.Fatalf("resume point: %+v %v", got, err)
	}

	u.mustSend(t, rec.ID, "screen", 2, c, 0)

	// the browser says it sent 5 chunks and the server has 3: it is told, and the recording stays open
	_, err = u.svc.RecordingUploadFinish(personCtx(), service.RecordingUploadFinishRequest{
		ID: rec.ID, Tracks: []service.RecordingUploadCount{{Kind: "screen", Chunks: 5}},
	})
	if se = nil; code(err) != "E_INVALID_STATE" || !errors.As(err, &se) || se.Details["reason"] != "chunks_missing" ||
		se.Details["received"] != 3 {
		t.Fatalf("chunks_missing: %v", err)
	}

	if again, _ := u.svc.RecordingGet(personCtx(), service.RecordingRef{ID: rec.ID}); again.State != "recording" {
		t.Fatalf("the recording is %s", again.State)
	}
}

func TestRecordingUploadRefusesAWrongDigestAnEmptyBodyAndABadRequest(t *testing.T) {
	u := newUpEnv(t, false)
	project := u.newProject("p")
	rec := u.begin(t, project, false)

	body := []byte("some bytes")

	cases := map[string]service.RecordingUploadChunkRequest{
		"a wrong digest": {ID: rec.ID, Track: "screen", SHA256: digest([]byte("other")), Body: bytes.NewReader(body)},
		"an empty body":  {ID: rec.ID, Track: "screen", SHA256: digest(nil), Body: bytes.NewReader(nil)},
		"no body":        {ID: rec.ID, Track: "screen", SHA256: digest(body)},
		"a short digest": {ID: rec.ID, Track: "screen", SHA256: "abc", Body: bytes.NewReader(body)},
		"an upper case digest": {
			ID: rec.ID, Track: "screen", SHA256: strings.ToUpper(digest(body)), Body: bytes.NewReader(body),
		},
		"a track the row did not ask for": {
			ID:     rec.ID,
			Track:  "camera",
			SHA256: digest(body),
			Body:   bytes.NewReader(body),
		},
		"a track that is no track": {
			ID:     rec.ID,
			Track:  "microphone",
			SHA256: digest(body),
			Body:   bytes.NewReader(body),
		},
		"a negative clock": {
			ID: rec.ID, Track: "screen", SHA256: digest(body), Body: bytes.NewReader(body), ElapsedMS: -1,
		},
		"a negative index": {
			ID: rec.ID, Track: "screen", Index: -1, SHA256: digest(body), Body: bytes.NewReader(body),
		},
	}

	for name, req := range cases {
		if _, err := u.svc.RecordingUploadChunk(personCtx(), req); code(err) != command.CodeValidation {
			t.Errorf("%s: err = %v, want E_VALIDATION", name, err)
		}
	}

	if left := u.parts(rec.ID); len(left) != 0 {
		t.Errorf("a refused chunk left %v", left)
	}

	// the digest refusal says which kind it is
	_, err := u.svc.RecordingUploadChunk(personCtx(), service.RecordingUploadChunkRequest{
		ID: rec.ID, Track: "screen", SHA256: digest([]byte("other")), Body: bytes.NewReader(body),
	})

	var se *service.Error
	if !errors.As(err, &se) || se.Path != "sha256" || se.Details["reason"] != "checksum_mismatch" {
		t.Errorf("details = %+v", se)
	}

	// an index that cannot be a chunk
	_, err = u.svc.RecordingUploadChunk(personCtx(), service.RecordingUploadChunkRequest{
		ID: rec.ID, Track: "screen", Index: 100000, SHA256: digest(body), Body: bytes.NewReader(body),
	})
	if code(err) != "E_LIMIT_EXCEEDED" {
		t.Errorf("an index out of range: %v", err)
	}
}

func TestRecordingUploadLimits(t *testing.T) {
	u := newUpEnv(t, false)
	project := u.newProject("p")
	rec := u.begin(t, project, false)

	reason := func(err error) string {
		if e, ok := errors.AsType[*service.Error](err); ok {
			if r, ok := e.Details["reason"].(string); ok {
				return r
			}
		}

		return ""
	}

	// the clock of the recorder past the longest recording
	_, err := u.send(personCtx(), rec.ID, "screen", 0, []byte("x"), 14400*1000)
	if code(err) != "E_LIMIT_EXCEEDED" || reason(err) != "duration" {
		t.Errorf("duration: %v", err)
	}

	// a chunk over the limit of a chunk is refused while it streams, and nothing is stored
	big := io.LimitReader(zeroReader{}, 16<<20+1)
	_, err = u.svc.RecordingUploadChunk(personCtx(), service.RecordingUploadChunkRequest{
		ID: rec.ID, Track: "screen", SHA256: digest([]byte("x")), Body: big, Size: -1,
	})

	if code(err) != "E_PAYLOAD_TOO_LARGE" {
		t.Errorf("over the chunk limit: %v", err)
	}

	// a body whose reader reports the route limit
	_, err = u.svc.RecordingUploadChunk(personCtx(), service.RecordingUploadChunkRequest{
		ID: rec.ID, Track: "screen", SHA256: digest([]byte("x")), Body: errReader{service.ErrUploadBodyTooLarge},
	})
	if code(err) != "E_PAYLOAD_TOO_LARGE" {
		t.Errorf("the route limit: %v", err)
	}

	// a body that breaks is a refusal of the request, not a server fault
	_, err = u.svc.RecordingUploadChunk(personCtx(), service.RecordingUploadChunkRequest{
		ID: rec.ID, Track: "screen", SHA256: digest([]byte("x")), Body: errReader{errors.New("connection reset")},
	})
	if code(err) != command.CodeValidation {
		t.Errorf("a broken body: %v", err)
	}

	if left := u.parts(rec.ID); len(left) != 0 {
		t.Errorf("refused chunks left %v", left)
	}

	// the disk of the workspace is under the free space the product keeps
	u.free.Store(1 << 10)

	_, err = u.send(personCtx(), rec.ID, "screen", 0, []byte("x"), 0)
	if code(err) != "E_LIMIT_EXCEEDED" || reason(err) != "disk_low" {
		t.Errorf("disk: %v", err)
	}

	u.free.Store(1 << 40)

	// nothing was lost: the track still waits for chunk 0 and takes it
	u.mustSend(t, rec.ID, "screen", 0, []byte("x"), 0)
}

func TestRecordingUploadByteLimit(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{with: func(_ *env, o *service.Options) { o.Limits.MaxImportBytes = 30 }})
	u := &upEnv{m}
	project := u.newProject("p")
	rec := u.begin(t, project, false)

	u.mustSend(t, rec.ID, "screen", 0, bytes.Repeat([]byte("a"), 20), 0)

	_, err := u.send(personCtx(), rec.ID, "screen", 1, bytes.Repeat([]byte("b"), 11), 0)

	var se *service.Error
	if code(err) != "E_LIMIT_EXCEEDED" || !errors.As(err, &se) || se.Details["reason"] != "size" {
		t.Fatalf("size: %v", err)
	}

	// what was received is kept: the next chunk that fits is taken
	u.mustSend(t, rec.ID, "screen", 1, bytes.Repeat([]byte("b"), 10), 0)
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestRecordingUploadOneOpenUploadPerActorAndNativeIsNotBlocked(t *testing.T) {
	r := newRecEnv(t)
	u := &upEnv{r.mediaEnv}
	project := r.newProject("p")

	first := u.begin(t, project, false)

	_, err := u.svc.RecordingUploadBegin(personCtx(), service.RecordingUploadBeginRequest{
		ProjectID: project, Container: "webm",
	})

	var se *service.Error
	if code(err) != "E_QUEUE_FULL" || !errors.As(err, &se) || se.Details["limit"] != "recordings" {
		t.Fatalf("a second upload of the same person: %v", err)
	}

	// a browser upload does not stand in the way of the one native capture, and the other way round
	native := r.start(t, project)
	if native.State != "recording" {
		t.Fatalf("native = %+v", native)
	}

	if _, err := u.svc.RecordingCancel(personCtx(), service.RecordingRef{ID: first.ID}); err != nil {
		t.Fatal(err)
	}

	if again := u.begin(t, project, false); again.State != "recording" {
		t.Fatalf("again = %+v", again)
	}
}

func TestRecordingUploadFourOpenUploadsOnTheWorkspace(t *testing.T) {
	u := newUpEnv(t, false)
	project := u.newProject("p")

	for i := range 4 {
		ctx := u.as("local:person-" + strconv.Itoa(i))
		ctx = service.WithCaller(ctx, service.Caller{
			Actor: "local:person-" + strconv.Itoa(i), Scopes: service.AllScopes, Browser: true,
		})

		if _, err := u.svc.RecordingUploadBegin(ctx, service.RecordingUploadBeginRequest{
			ProjectID: project, Container: "webm",
		}); err != nil {
			t.Fatalf("upload %d: %v", i, err)
		}
	}

	_, err := u.svc.RecordingUploadBegin(service.WithCaller(context.Background(), service.Caller{
		Actor: "local:person-9", Scopes: service.AllScopes, Browser: true,
	}), service.RecordingUploadBeginRequest{ProjectID: project, Container: "webm"})
	if code(err) != "E_QUEUE_FULL" {
		t.Fatalf("a fifth upload: %v", err)
	}
}

// A tab closed in the middle: the chunks that arrived are finished into a playable file by a stop (SDD-025 17.6).
func TestRecordingUploadClosedTabIsFinishedByAStop(t *testing.T) {
	u := newUpEnv(t, true)
	project := u.newProject("p")
	chunks := split(webmFixture(t, 3, true), 6)

	rec := u.begin(t, project, false)

	// the tab sends four of six chunks and is gone
	for i := range 4 {
		u.mustSend(t, rec.ID, "screen", i, chunks[i], int64(i)*500)
	}

	stopped, err := u.svc.RecordingStop(u.as(agent), service.RecordingRef{ID: rec.ID})
	if err != nil || (stopped.State != "finishing" && stopped.State != "ready") {
		t.Fatalf("stop = %+v %v", stopped, err)
	}

	ready := u.wait(t, rec.ID, "ready")
	if ready.DurationMS <= 0 || ready.StopReason != "stopped" || ready.Review != "pending" {
		t.Fatalf("ready = %+v", ready)
	}

	// a stop is idempotent
	if again, err := u.svc.RecordingStop(
		personCtx(),
		service.RecordingRef{ID: rec.ID},
	); err != nil ||
		again.State != "ready" {
		t.Fatalf("a second stop: %+v %v", again, err)
	}

	// and a chunk that comes after is refused
	if _, err := u.send(personCtx(), rec.ID, "screen", 4, chunks[4], 0); code(err) != "E_INVALID_STATE" {
		t.Fatalf("a late chunk: %v", err)
	}
}

func TestRecordingUploadIdleUploadIsSettledByTheNextBegin(t *testing.T) {
	u := newUpEnv(t, true)
	project := u.newProject("p")
	chunks := split(webmFixture(t, 2, false), 4)

	first := u.begin(t, project, false)
	for i := range 2 {
		u.mustSend(t, first.ID, "screen", i, chunks[i], 0)
	}

	// four minutes later nothing changed; five minutes later the tab is taken for gone
	u.clock.advance(4 * time.Minute)

	if _, err := u.svc.RecordingUploadBegin(personCtx(), service.RecordingUploadBeginRequest{
		ProjectID: project, Container: "webm",
	}); code(err) != "E_QUEUE_FULL" {
		t.Fatalf("an upload that is not idle yet: %v", err)
	}

	u.clock.advance(2 * time.Minute)

	second := u.begin(t, project, false)
	if second.ID == first.ID {
		t.Fatal("same id")
	}

	settled := u.wait(t, first.ID, "ready")
	if settled.StopReason != "abandoned" || settled.DurationMS <= 0 {
		t.Fatalf("settled = %+v", settled)
	}

	_, _ = u.svc.RecordingCancel(personCtx(), service.RecordingRef{ID: second.ID})
}

func TestRecordingUploadRecoveryAssemblesTheChunksOfAnUploadTheServerLost(t *testing.T) {
	u := newUpEnv(t, true)
	project := u.newProject("p")
	chunks := split(webmFixture(t, 2, true), 4)

	const id = "rec_0000000042"

	if err := u.store.Recordings().Insert(context.Background(), recordings.Row{
		ID: id, ProjectID: project, State: "recording", SourceKind: "browser", Encoder: "webm", Actor: ui,
		CreatedMs: 1, StartedMs: 1, Tracks: `[{"kind":"screen","file":"capture.webm"}]`,
	}); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(u.ws, "recordings", id, "parts", "screen")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	for i, c := range chunks {
		if err := os.WriteFile(filepath.Join(dir, "00000"+strconv.Itoa(i)+".part"), c, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	runner, _ := service.RunnerOf(u.svc)
	if err := runner.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}

	got, err := u.svc.RecordingGet(personCtx(), service.RecordingRef{ID: id})
	if err != nil || got.State != "ready" || got.StopReason != "recovered" || got.DurationMS < 1500 {
		t.Fatalf("recovered = %+v %v", got, err)
	}
}

func TestRecordingUploadRecoveryWithNoFFmpegFailsAndKeepsTheChunks(t *testing.T) {
	u := newUpEnv(t, false)
	project := u.newProject("p")

	const id = "rec_0000000043"

	if err := u.store.Recordings().Insert(context.Background(), recordings.Row{
		ID: id, ProjectID: project, State: "recording", SourceKind: "browser", Encoder: "webm", Actor: ui,
		CreatedMs: 1, StartedMs: 1, Tracks: `[{"kind":"screen","file":"capture.webm"}]`,
	}); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(u.ws, "recordings", id, "parts", "screen")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "000000.part"), []byte("bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	runner, _ := service.RunnerOf(u.svc)
	if err := runner.Recover(context.Background()); err != nil {
		t.Fatal(err)
	}

	got, err := u.svc.RecordingGet(personCtx(), service.RecordingRef{ID: id})
	if err != nil || got.State != "failed" || got.Error == nil ||
		!strings.HasPrefix(got.Error.Message, "ffmpeg_missing:") {
		t.Fatalf("recovered = %+v %v", got, err)
	}

	if len(u.parts(id)) != 1 {
		t.Error("the chunks must stay for a person to delete")
	}
}

func TestRecordingUploadOfBytesThatAreNoMediaFailsAndKeepsTheChunks(t *testing.T) {
	u := newUpEnv(t, true)
	project := u.newProject("p")
	rec := u.begin(t, project, false)

	u.mustSend(
		t,
		rec.ID,
		"screen",
		0,
		append([]byte{0x00, 0x01, 0x02, 0xff}, bytes.Repeat([]byte{0x9a, 0x00}, 300)...),
		0,
	)

	if _, err := u.svc.RecordingUploadFinish(
		personCtx(),
		service.RecordingUploadFinishRequest{ID: rec.ID},
	); err != nil {
		t.Fatal(err)
	}

	failed := u.wait(t, rec.ID, "failed")
	if failed.Error == nil || failed.Error.Code != "E_CAPTURE_FAILED" ||
		!strings.HasPrefix(failed.Error.Message, "assemble:") {
		t.Fatalf("failed = %+v", failed)
	}

	if len(u.parts(rec.ID)) != 1 {
		t.Error("the chunks must stay for a person to delete")
	}

	if _, err := os.Stat(filepath.Join(u.ws, "recordings", rec.ID, "capture.webm")); err == nil {
		t.Error("a file was made of bytes that are not media")
	}

	// a person can delete it, and the chunks go
	if _, err := u.svc.RecordingCancel(personCtx(), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(u.ws, "recordings", rec.ID)); err == nil {
		t.Error("the folder of a deleted recording stays")
	}
}

func TestRecordingUploadWithNoChunkOfTheScreenFails(t *testing.T) {
	u := newUpEnv(t, true)
	project := u.newProject("p")
	rec := u.begin(t, project, false)

	if _, err := u.svc.RecordingUploadFinish(
		personCtx(),
		service.RecordingUploadFinishRequest{ID: rec.ID},
	); err != nil {
		t.Fatal(err)
	}

	failed := u.wait(t, rec.ID, "failed")
	if failed.Error == nil || !strings.HasPrefix(failed.Error.Message, "no_frames:") {
		t.Fatalf("failed = %+v", failed)
	}
}

func TestRecordingUploadCancelDiscardsTheChunks(t *testing.T) {
	u := newUpEnv(t, false)
	project := u.newProject("p")
	rec := u.begin(t, project, false)
	u.mustSend(t, rec.ID, "screen", 0, []byte("abc"), 0)

	// any caller with media may discard a recording in progress
	got, err := u.svc.RecordingCancel(u.as(agent), service.RecordingRef{ID: rec.ID})
	if err != nil || got.State != "cancelled" {
		t.Fatalf("cancel = %+v %v", got, err)
	}

	if _, err := os.Stat(filepath.Join(u.ws, "recordings", rec.ID)); err == nil {
		t.Error("the folder stays")
	}

	if _, err := u.send(personCtx(), rec.ID, "screen", 1, []byte("abc"), 0); code(err) != "E_INVALID_STATE" {
		t.Errorf("a chunk after the cancel: %v", err)
	}
}

func TestRecordingUploadHasNoPauseAndTakesNoProjectThatIsNotThere(t *testing.T) {
	u := newUpEnv(t, false)
	project := u.newProject("p")
	rec := u.begin(t, project, false)

	if _, err := u.svc.RecordingPause(personCtx(), service.RecordingRef{ID: rec.ID}); code(err) != "E_INVALID_STATE" {
		t.Errorf("pause: %v", err)
	}

	for name, req := range map[string]service.RecordingUploadBeginRequest{
		"a bad container":  {ProjectID: project, Container: "avi"},
		"a bad project id": {ProjectID: "nope", Container: "webm"},
		"an unknown project": {
			ProjectID: "prj_0000000000", Container: "webm",
		},
	} {
		if _, err := u.svc.RecordingUploadBegin(personCtx(), req); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}

	// the label of the take is text: it is cleaned and never builds a path
	got, err := u.svc.RecordingUploadBegin(service.WithCaller(context.Background(), service.Caller{
		Actor: "local:second", Scopes: service.AllScopes, Browser: true,
	}), service.RecordingUploadBeginRequest{ProjectID: project, Container: "mp4", Label: "../../x\x1b[31m\ny"})
	if err != nil || strings.ContainsAny(got.Source.Name, "\x1b\n") {
		t.Fatalf("label = %q %v", got.Source.Name, err)
	}

	if got.Upload.Container != "mp4" {
		t.Errorf("container = %s", got.Upload.Container)
	}
}

// SDD-007 T-137: every route of the upload is for a person, and the answer to anyone else comes before the project,
// the recording or the disk is looked at.
func TestSecurity_T137_OnlyAPersonUploads(t *testing.T) {
	u := newUpEnv(t, false)
	project := u.newProject("p")
	rec := u.begin(t, project, false)

	body := []byte("bytes")

	callers := map[string]context.Context{
		"the local token": service.WithCaller(context.Background(), service.Caller{
			Actor: "local:cli", Scopes: service.AllScopes,
		}),
		"an agent": u.as(agent),
		"an api key": service.WithCaller(
			context.Background(), service.Caller{Actor: "key:key_0000000001", Scopes: service.AllScopes},
		),
		"an mcp client": service.WithCaller(
			context.Background(), service.Caller{Actor: "local:mcp", Scopes: service.AllScopes},
		),
		"an operator": service.WithCaller(context.Background(), service.Caller{
			Actor: "local:cli", Scopes: service.AllScopes, Operator: true,
		}),
	}

	for name, ctx := range callers {
		if _, err := u.svc.RecordingUploadBegin(ctx, service.RecordingUploadBeginRequest{
			ProjectID: project, Container: "webm",
		}); code(err) != "E_PRIVILEGED_SESSION" {
			t.Errorf("%s begins: %v", name, err)
		}

		if _, err := u.svc.RecordingUploadChunk(ctx, service.RecordingUploadChunkRequest{
			ID: rec.ID, Track: "screen", SHA256: digest(body), Body: bytes.NewReader(body),
		}); code(err) != "E_PRIVILEGED_SESSION" {
			t.Errorf("%s uploads a chunk: %v", name, err)
		}

		if _, err := u.svc.RecordingUploadFinish(
			ctx,
			service.RecordingUploadFinishRequest{ID: rec.ID},
		); code(
			err,
		) != "E_PRIVILEGED_SESSION" {
			t.Errorf("%s finishes: %v", name, err)
		}
	}

	if left := u.parts(rec.ID); len(left) != 0 {
		t.Errorf("a caller that is not a person stored %v", left)
	}

	if again, _ := u.svc.RecordingGet(personCtx(), service.RecordingRef{ID: rec.ID}); again.State != "recording" {
		t.Errorf("a refused caller moved the recording to %s", again.State)
	}

	// stopping is the one thing every caller with media may do (it only reduces what is held)
	if _, err := u.svc.RecordingStop(u.as(agent), service.RecordingRef{ID: rec.ID}); err != nil {
		t.Errorf("an agent may stop a recording: %v", err)
	}
}

// SDD-007 T-137: a chunk out of order, with a wrong digest or over a limit is refused and stores nothing, and the
// track goes on from where it was.
func TestSecurity_T137_ARefusedChunkStoresNothingAndTheTrackGoesOn(t *testing.T) {
	u := newUpEnv(t, false)
	project := u.newProject("p")
	rec := u.begin(t, project, false)

	u.mustSend(t, rec.ID, "screen", 0, []byte("zero"), 0)

	refusals := map[string]func() error{
		"a gap": func() error {
			_, err := u.send(personCtx(), rec.ID, "screen", 3, []byte("three"), 0)

			return err
		},
		"a wrong digest": func() error {
			_, err := u.svc.RecordingUploadChunk(personCtx(), service.RecordingUploadChunkRequest{
				ID: rec.ID, Track: "screen", Index: 1, SHA256: digest([]byte("x")), Body: strings.NewReader("one"),
			})

			return err
		},
		"a clock over the limit": func() error {
			_, err := u.send(personCtx(), rec.ID, "screen", 1, []byte("one"), 1<<40)

			return err
		},
		"a body over the chunk limit": func() error {
			_, err := u.svc.RecordingUploadChunk(personCtx(), service.RecordingUploadChunkRequest{
				ID:     rec.ID,
				Track:  "screen",
				Index:  1,
				SHA256: digest([]byte("x")),
				Body:   io.LimitReader(zeroReader{}, 16<<20+1),
			})

			return err
		},
	}

	want := map[string]string{
		"a gap": "E_INVALID_STATE", "a wrong digest": "E_VALIDATION", "a clock over the limit": "E_LIMIT_EXCEEDED",
		"a body over the chunk limit": "E_PAYLOAD_TOO_LARGE",
	}

	for name, do := range refusals {
		if got := code(do()); got != want[name] {
			t.Errorf("%s = %s, want %s", name, got, want[name])
		}
	}

	if left := u.parts(rec.ID); len(left) != 1 {
		t.Errorf("the track holds %v, want the one good chunk", left)
	}

	u.mustSend(t, rec.ID, "screen", 1, []byte("one"), 0)
}

// SDD-007 T-137: what is uploaded is not trusted for what it says it is. A playlist, a concat script, markup or
// subtitles sent as the chunks of a WebM recording are not assembled into a file: the recording fails and no file is
// made for the library.
func TestSecurity_T137_AHostileFileIsNeverAssembledIntoARecording(t *testing.T) {
	u := newUpEnv(t, true)
	project := u.newProject("p")

	for name, text := range map[string]string{
		"a concat script": "ffconcat version 1.0\nfile '/etc/passwd'\n",
		"a playlist":      "#EXTM3U\n#EXTINF:1,x\nhttp://127.0.0.1:1/x\n",
		"markup":          "<html><script>alert(1)</script></html>",
		"subtitles":       "WEBVTT\n\n00:00.000 --> 00:01.000\nhello\n",
	} {
		t.Run(name, func(t *testing.T) {
			// one recording at a time of the same person: each case is its own
			rec := u.begin(t, project, false)
			u.mustSend(t, rec.ID, "screen", 0, []byte(text), 0)

			if _, err := u.svc.RecordingUploadFinish(
				personCtx(),
				service.RecordingUploadFinishRequest{ID: rec.ID},
			); err != nil {
				t.Fatal(err)
			}

			failed := u.wait(t, rec.ID, "failed")
			if failed.Error == nil || !strings.HasPrefix(failed.Error.Message, "assemble:") {
				t.Fatalf("failed = %+v", failed)
			}

			if _, err := os.Stat(filepath.Join(u.ws, "recordings", rec.ID, "capture.webm")); err == nil {
				t.Error("a file was made")
			}

			if _, err := u.svc.RecordingConfirm(
				personCtx(),
				service.RecordingRef{ID: rec.ID},
			); code(
				err,
			) != "E_INVALID_STATE" {
				t.Errorf("a failed recording was added: %v", err)
			}
		})
	}
}
