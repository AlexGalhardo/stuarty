package servicetest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/facetrack"
	"github.com/AlexGalhardo/stuarty/engine/aihost"
	"github.com/AlexGalhardo/stuarty/service"
)

// stubFaces is a face detector that needs no helper: one face crossing the frame.
type stubFaces struct {
	calls atomic.Int64
	last  atomic.Pointer[aihost.Input]
}

func (s *stubFaces) Detect(
	_ context.Context,
	in *aihost.Input,
	progress func(done, total int),
) (facetrack.Track, error) {
	s.calls.Add(1)
	s.last.Store(in)

	t := facetrack.Track{
		Version: facetrack.Version, Model: aihost.ModelName, Provider: "cpu",
		Source: facetrack.Size{Width: in.Width, Height: in.Height}, FPS: in.FPS,
		StartMS: int(in.StartMS), EndMS: int(in.EndMS),
	}

	for ms := in.StartMS; ms < in.EndMS; ms += 1000 / int64(in.FPS) {
		cx := 2000 + int(6000*(ms-in.StartMS)/(in.EndMS-in.StartMS))
		t.Samples = append(t.Samples, facetrack.Sample{
			T: int(ms), Faces: []facetrack.Face{{X: cx - 750, Y: 2500, W: 1500, H: 3300, Score: 900}},
		})
	}

	progress(len(t.Samples), len(t.Samples))

	return t, nil
}

func (m *mediaEnv) faceJob(ctx context.Context, project, asset, extra string) (*service.Job, error) {
	return m.svc.JobStart(ctx, service.JobStartRequest{
		ProjectID: project, Kind: service.JobFaceTrack,
		Args: []byte(fmt.Sprintf(`{"asset_id":%q%s}`, asset, extra)),
	})
}

func (m *mediaEnv) projectAssetOf(project, hash string) string {
	m.t.Helper()

	rows, _ := m.store.Assets().ListProjectAssets(context.Background(), project)
	for i := range rows {
		if rows[i].AssetHash == hash {
			return rows[i].AssetID
		}
	}

	m.t.Fatal("the asset is not in the project")

	return ""
}

func TestFaceTrackJobStoresTheTrackOnceAndFeedsTheFraming(t *testing.T) {
	det := &stubFaces{}
	m := newMediaEnv(t, mediaCfg{faces: det})
	project, clip, _, hash := m.speechProject()
	asset := m.projectAssetOf(project, hash)

	// Without a track the framing says which job to run, and starts nothing.
	_, err := m.svc.Edit(m.as(ui), service.EditRequest{
		ProjectID: project, BaseRevision: m.head(project).Revision,
		Operations: []command.Operation{op("clip.face_frame", fmt.Sprintf(`{"item_id":%q}`, clip))},
	})
	if code(err) != "E_INVALID_STATE" || !strings.Contains(err.Error(), "face") || det.calls.Load() != 0 {
		t.Fatalf("framing before the job = %v (detector ran %d times)", err, det.calls.Load())
	}

	// A dry run names no job and runs nothing.
	dry, err := m.svc.JobStart(m.as(ui), service.JobStartRequest{
		ProjectID: project,
		Kind:      service.JobFaceTrack,
		DryRun:    true,
		Args:      []byte(fmt.Sprintf(`{"asset_id":%q}`, asset)),
	})
	if err != nil || !dry.DryRun || dry.JobID != "" || det.calls.Load() != 0 {
		t.Fatalf("dry run = %+v, %v", dry, err)
	}

	job, err := m.faceJob(m.as(ui), project, asset, "")
	if err != nil || job.State != service.JobSucceeded || job.Result == nil || job.Result.FaceTrack == nil ||
		job.Result.FaceTrack.Samples != 10 || job.Result.FaceTrack.Faces != 10 || job.Result.FaceTrack.FPS != 5 {
		t.Fatalf("job = %+v, %v", job, err)
	}

	if in := det.last.Load(); in.Width != 320 || in.Height != 240 || in.StartMS != 0 || in.EndMS != 2000 ||
		!strings.HasSuffix(filepath.ToSlash(in.Source), "/proxy-1.mp4") && !strings.Contains(in.Source, "proxy") {
		t.Errorf("the detector got %+v", in)
	}

	hex := strings.TrimPrefix(hash, "sha256:")
	if files, _ := filepath.Glob(filepath.Join(m.ws, "artifacts", hex, "facetrack-1-*.json")); len(files) != 1 {
		t.Fatalf("face track files = %v", files)
	}

	again, err := m.faceJob(m.as(ui), project, asset, "")
	if err != nil || again.State != service.JobSucceeded || det.calls.Load() != 1 {
		t.Errorf("second job = %+v, %v; the detector ran %d times, want 1", again, err, det.calls.Load())
	}

	// The track now feeds the framing: the 16:9... fake clip is 320x240 in a project frame of 1080x1920.
	res := m.mustEdit(ui, project, m.head(project).Revision, op("clip.face_frame", fmt.Sprintf(`{"item_id":%q}`, clip)))
	if len(res.Diff) == 0 {
		t.Errorf("the framing changed nothing: %+v", res)
	}

	stored := storedOps(t, m, project, res.Revision)
	if len(stored) != 1 || !strings.Contains(string(stored[0].Args), `"resolved"`) ||
		!strings.Contains(string(stored[0].Args), `"samples"`) {
		t.Errorf("the stored operation has no resolved track: %s", stored[0].Args)
	}
}

func TestFaceTrackRefusesBadArgumentsAndRanges(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{faces: &stubFaces{}})
	project, _, _, hash := m.speechProject()
	asset := m.projectAssetOf(project, hash)

	for name, extra := range map[string]string{
		"fps too high":   `,"fps":11`,
		"fps negative":   `,"fps":-1`,
		"empty range":    `,"start_ms":500,"end_ms":500`,
		"past the end":   `,"start_ms":5000,"end_ms":6000`,
		"negative start": `,"start_ms":-5,"end_ms":100`,
	} {
		if _, err := m.faceJob(m.as(ui), project, asset, extra); code(err) != "E_VALIDATION" {
			t.Errorf("%s: error = %v, want E_VALIDATION", name, err)
		}
	}

	if _, err := m.svc.JobStart(m.as(ui), service.JobStartRequest{
		ProjectID: project, Kind: service.JobFaceTrack, Args: []byte(`{"asset_id":"ast_zzzzzzzzzz"}`),
	}); code(err) != "E_NOT_FOUND" {
		t.Errorf("an unknown asset = %v", err)
	}
}

// With the set not installed the job refuses at once, names what to install and downloads nothing
// (acceptance criterion 1 of epic 70: no outbound connection without a request).
func TestFaceTrackWithoutTheSetNamesTheInstall(t *testing.T) {
	fs := newFaceServer(t)
	tl := fs.tools(faceDir(t), true)
	m := newMediaEnv(t, mediaCfg{tools: tl})
	project, _, _, hash := m.speechProject()

	_, err := m.faceJob(m.as(ui), project, m.projectAssetOf(project, hash), "")
	if code(err) != "E_TRANSCRIBE_UNAVAILABLE" {
		t.Fatalf("error = %v, want E_TRANSCRIBE_UNAVAILABLE", err)
	}

	se, _ := errors.AsType[*service.Error](err)
	if se == nil || se.Details["tool"] != "face" || se.Details["install_command"] != "stuarty tools:install face" ||
		se.Details["download_bytes"] == nil {
		t.Errorf("details = %+v", se)
	}

	if list, lerr := m.svc.JobList(m.as(ui), service.JobListRequest{ProjectID: project}); lerr == nil {
		for i := range list.Jobs {
			if list.Jobs[i].Kind == service.JobFaceTrack {
				t.Errorf("a job row was created for a refused start: %+v", list.Jobs[i])
			}
		}
	}

	if fs.requests.Load() != 0 {
		t.Errorf("%d requests were made without an install", fs.requests.Load())
	}
}

func TestFaceTrackIsAMediaJobOfItsOwner(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{faces: &stubFaces{}})
	project, _, _, hash := m.speechProject()

	read := service.Caller{Actor: "key:r", Scopes: []string{service.ScopeRead}}

	_, err := m.faceJob(service.WithCaller(context.Background(), read), project, m.projectAssetOf(project, hash), "")
	if code(err) != service.CodeForbidden {
		t.Errorf("a read key started a face track: %v", err)
	}
}

func TestFaceTrackArtifactIsInTheWorkspaceOnly(t *testing.T) {
	m := newMediaEnv(t, mediaCfg{faces: &stubFaces{}})
	project, _, _, hash := m.speechProject()

	if _, err := m.faceJob(m.as(ui), project, m.projectAssetOf(project, hash), `,"fps":2`); err != nil {
		t.Fatal(err)
	}

	hex := strings.TrimPrefix(hash, "sha256:")

	files, _ := filepath.Glob(filepath.Join(m.ws, "artifacts", hex, "facetrack-1-*.json"))
	if len(files) != 1 || !strings.Contains(files[0], "-2-0-2000") {
		t.Fatalf("files = %v", files)
	}

	if body, err := os.ReadFile(files[0]); err != nil || !strings.Contains(string(body), `"fps":2`) {
		t.Errorf("artifact = %s, %v", body, err)
	}
}
