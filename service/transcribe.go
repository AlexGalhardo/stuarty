package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/transcribe"
)

// JobTranscribe is the kind of a transcription job (SDD-011 section 10).
const JobTranscribe = "transcribe"

// transcriptSampleRate is the rate the sample counts of an asset are in.
const transcriptSampleRate = 48000

// TranscribeArgs are the arguments of JobStart for a transcription.
type TranscribeArgs struct {
	AssetID  string `json:"asset_id"`
	Language string `json:"language,omitempty"` // BCP 47 code or "auto"; default "auto"
	// Model is the name of an installed whisper model (tiny, base, small, large-v3-turbo); empty is the
	// workspace default. Only a name, never a path (VV-007).
	Model string `json:"model,omitempty"`
}

// TranscribeResult is the result of a finished transcription job.
type TranscribeResult struct {
	AssetID  string `json:"asset_id"`
	Language string `json:"language"`
	Words    int    `json:"words"`
}

// transcribeInput is what the job row keeps: the asset of the project, the
// language that was asked for, and the key of the artifact it produces.
type transcribeInput struct {
	AssetID  string `json:"asset_id"`
	Language string `json:"language"`
	Model    string `json:"model,omitempty"`
	Key      string `json:"key"`
}

func decodeTranscribeArgs(raw json.RawMessage) (TranscribeArgs, *Error) {
	var a TranscribeArgs

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()

	if len(bytes.TrimSpace(raw)) == 0 {
		return a, newError(command.CodeValidation, "args.asset_id is required.").
			withPath("args.asset_id").withHint("name the asset to transcribe")
	}

	if err := dec.Decode(&a); err != nil {
		return a, newError(command.CodeValidation, "the arguments of a transcription are not valid: %v.", err).
			withPath("args")
	}

	if a.AssetID == "" {
		return a, newError(command.CodeValidation, "args.asset_id is required.").withPath("args.asset_id")
	}

	if a.Language == "" {
		a.Language = "auto"
	}

	if _, err := transcribe.Language(a.Language); err != nil && !strings.EqualFold(a.Language, "auto") {
		return a, newError(command.CodeValidation, "the language %q is not a language code.", a.Language).
			withPath("args.language").withHint("use a BCP 47 code such as en or pt-BR, or auto")
	}

	return a, nil
}

// ModelPicker is implemented by a provider that can run more than one model, so a job may name
// one of the installed models (VV-007).
type ModelPicker interface {
	// InstalledModels are the names of the models installed and verified now.
	InstalledModels() []string
	// WithModel is the provider bound to one model, installed or not.
	WithModel(model string) transcribe.Provider
}

// providerFor is the provider a job runs on: the default one, or the one bound to the model the job
// named. A model name that is not installed is a validation error that lists the installed ones.
func (s *svc) providerFor(model string) (transcribe.Provider, *Error) {
	if model == "" {
		return s.transcriber, nil
	}

	picker, ok := s.transcriber.(ModelPicker)
	if !ok {
		return nil, newError(command.CodeValidation, "the transcription provider %s does not take a model.",
			s.transcriber.Name()).withPath("args.model").withHint("omit args.model")
	}

	installed := picker.InstalledModels()
	if !slices.Contains(installed, model) {
		hint := "install it with: vukem-video tools:install whisper --model " + model
		if slices.Contains(transcribe.Models, model) {
			hint = fmt.Sprintf("installed models: %s; or %s", listOrNone(installed), hint)
		} else {
			hint = "use one of the installed models: " + listOrNone(installed)
		}

		return nil, newError(command.CodeValidation, "the model %q is not installed.", model).
			withPath("args.model").withHint("%s", hint).withDetail("installed", installed)
	}

	return picker.WithModel(model), nil
}

func listOrNone(models []string) string {
	if len(models) == 0 {
		return "none"
	}

	return strings.Join(models, ", ")
}

// unavailable is E_TRANSCRIBE_UNAVAILABLE for a provider that is not installed:
// it names what is missing, its size and the command that installs it, and
// downloads nothing (SDD-011 section 10.2, SDD-007 section 6.8).
func unavailable(err error, model string) *Error {
	var missing *transcribe.MissingError

	if !errors.As(err, &missing) {
		return newError(codeTranscribeOff, "no transcription provider is available.").
			withHint("install the local helper with tools:install, or configure an API provider")
	}

	parts := make([]string, 0, len(missing.Items))
	for _, it := range missing.Items {
		parts = append(parts, fmt.Sprintf("%s (%s)", it.Name, sizesText(it.Bytes)))
	}

	install := "vukem-video tools:install whisper --model " + model

	items := make([]map[string]any, 0, len(missing.Items))
	for _, it := range missing.Items {
		items = append(items, map[string]any{"name": it.Name, "url_host": it.URLHost, "bytes": it.Bytes})
	}

	return newError(codeTranscribeOff, "Local transcription needs %s, %s in all, and they are not installed.",
		strings.Join(parts, " and "), sizesText(missing.DownloadBytes)).
		withHint("Run %q (or use the install dialog of the editor), or configure an API provider.", install).
		withDetail("missing", items).withDetail("download_bytes", missing.DownloadBytes).
		withDetail("install_command", install).
		withDetail("tool", transcribe.HelperName).withDetail("model", model)
}

func sizesText(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/1e6)
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/1e3)
	}

	return fmt.Sprintf("%d B", n)
}

// startTranscribe is JobStart for kind transcribe (SDD-003 section 2.8, SDD-011
// section 10.1). It refuses at once, without a job row and without a download,
// when the helper is not installed.
func (s *svc) startTranscribe(ctx context.Context, c Caller, req JobStartRequest) (*Job, error) {
	args, aerr := decodeTranscribeArgs(req.Args)
	if aerr != nil {
		return nil, aerr
	}

	project, lerr := s.loadProject(ctx, c, req.ProjectID)
	if lerr != nil {
		return nil, lerr
	}

	if project.State == ProjectTrashed {
		return nil, projectInTrash(project.ID)
	}

	pa, err := s.store.Assets().GetProjectAsset(ctx, project.ID, args.AssetID)
	if err != nil || pa.State == ProjectAssetTrashed {
		return nil, newError(command.CodeNotFound, "the project has no asset %s.", args.AssetID).
			withPath("args.asset_id").withHint("list the assets with media_inspect and no asset_id")
	}

	ar, err := s.store.Assets().Get(ctx, pa.AssetHash)
	if err != nil {
		return nil, asError(err)
	}

	if ar.IngestState != IngestReady {
		return nil, newError("E_ASSET_NOT_READY", "the asset is %s; it cannot be transcribed yet.", ar.IngestState).
			withHint("wait for the asset to be ready")
	}

	if !ar.Facts.HasAudio || ar.Facts.SampleCount == nil {
		return nil, newError(command.CodeInvalidState, "the asset %s has no audio to transcribe.", args.AssetID).
			withPath("args.asset_id")
	}

	if secs := *ar.Facts.SampleCount / transcriptSampleRate; secs > s.limits.MaxTranscribeSeconds {
		return nil, newError("E_LIMIT_EXCEEDED", "The audio is %d seconds; the transcription limit is %d seconds "+
			"(VUKEM_VIDEO_MAX_TRANSCRIBE_SECONDS).", secs, s.limits.MaxTranscribeSeconds).
			withHint("transcribe a shorter file; the person running the server can raise "+
				"VUKEM_VIDEO_MAX_TRANSCRIBE_SECONDS").
			withDetail("limit", "transcribe_seconds").withDetail("value", secs).
			withDetail("ceiling", s.limits.MaxTranscribeSeconds).
			withDetail("variable", "VUKEM_VIDEO_MAX_TRANSCRIBE_SECONDS")
	}

	if s.transcriber == nil {
		return nil, newError(codeTranscribeOff, "no transcription provider is available.").
			withHint("install the local helper with tools:install, or configure an API provider")
	}

	provider, perr := s.providerFor(args.Model)
	if perr != nil {
		return nil, perr
	}

	key := transcribe.ArtifactKey(pa.AssetHash, provider.Name(), provider.Model(), args.Language)

	arts, err := s.store.Assets().Artifacts(ctx, pa.AssetHash)
	if err != nil {
		return nil, asError(err)
	}

	stored := slices.ContainsFunc(arts, func(a ArtifactRow) bool {
		return a.Kind == "transcript" && a.Variant == key && a.Ext == "json"
	})

	if !stored {
		if cerr := provider.Check(ctx); cerr != nil {
			return nil, unavailable(cerr, provider.Model())
		}

		if derr := s.checkDisk(); derr != nil && !req.DryRun {
			return nil, forCaller(c, derr)
		}
	}

	projectID, hash := project.ID, pa.AssetHash
	job := JobRow{
		ID:        s.newIDs("job"),
		Kind:      JobTranscribe,
		ProjectID: &projectID,
		AssetHash: &hash,
		Actor:     c.Actor,
		State:     JobQueued,
		Input: marshalOrNull(
			transcribeInput{AssetID: pa.AssetID, Language: args.Language, Model: args.Model, Key: key},
		),
	}

	if req.DryRun {
		return s.jobView(c, s.stamp(job), false), nil
	}

	lim := &JobLimit{
		Kind: JobTranscribe, Max: s.limits.MaxTranscribes, Actor: c.Actor, MaxPerActor: s.limits.MaxTranscribesPerActor,
	}

	err = s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := s.nowMs()
		job.CreatedMs, job.UpdatedMs = now, now

		if err := tx.Jobs().CreateLimited(ctx, job, lim); err != nil {
			return err
		}

		if err := tx.Audit().Append(ctx, AuditRow{
			AtMs: now, Actor: c.Actor, Action: "job.start", ProjectID: &projectID, SubjectID: &job.ID,
		}); err != nil {
			return err
		}

		queued := job
		tx.AfterCommit(func() { s.emitJob(ctx, &queued) })

		if s.enqueue != nil {
			return s.enqueue.Enqueue(ctx, tx, queueKindFor(job.Kind), job.ID)
		}

		return nil
	})
	if err != nil {
		return nil, callerError(c, transcribeLimitError(err, s.limits))
	}

	if req.WaitMS > 0 {
		s.waitJob(ctx, job.ID, time.Duration(req.WaitMS)*time.Millisecond)
	}

	row, gerr := s.store.Jobs().Get(ctx, job.ID)
	if gerr != nil {
		return nil, asError(gerr)
	}

	return s.jobView(c, row, req.WaitMS > 0), nil
}

func transcribeLimitError(err error, l Limits) error {
	out := asError(err)

	if e, ok := errors.AsType[*Error](out); ok && e.Code == CodeQueueFull {
		return e.withHint("at most %d transcriptions run at the same time and %d per key: wait for one to finish "+
			"with job_get or stop it with job_cancel; the person running the server can raise "+
			"VUKEM_VIDEO_MAX_TRANSCRIBES or VUKEM_VIDEO_MAX_TRANSCRIBES_PER_KEY",
			l.MaxTranscribes, l.MaxTranscribesPerActor)
	}

	return out
}

// runTranscribe is the handler of a transcription job (SDD-011 section 10):
// check, prepare, run, postprocess, write, verify. An artifact that already
// holds the same key makes it succeed at once.
func (s *svc) runTranscribe(ctx context.Context, job *JobRow, pw *progress) (json.RawMessage, error) {
	var in transcribeInput
	if job.AssetHash == nil || json.Unmarshal(job.Input, &in) != nil || s.transcriber == nil {
		return nil, internalError()
	}

	provider, perr := s.providerFor(in.Model)
	if perr != nil {
		return nil, perr
	}

	hash := *job.AssetHash
	hexHash := strings.TrimPrefix(hash, "sha256:")

	arts, err := s.store.Assets().Artifacts(ctx, hash)
	if err != nil {
		return nil, err
	}

	if i := slices.IndexFunc(arts, func(a ArtifactRow) bool {
		return a.Kind == "transcript" && a.Variant == in.Key && a.Ext == "json"
	}); i >= 0 {
		return s.transcribeResult(in, s.readArtifact(arts[i:i+1], "transcript", in.Key))
	}

	pw.report(ctx, 0.02, "checking the provider")

	if cerr := provider.Check(ctx); cerr != nil {
		return nil, unavailable(cerr, provider.Model())
	}

	if derr := s.checkDisk(); derr != nil {
		return nil, derr
	}

	ar, err := s.store.Assets().Get(ctx, hash)
	if err != nil {
		return nil, err
	}

	norm := slices.IndexFunc(arts, func(a ArtifactRow) bool { return a.Kind == "normalized" })
	if norm < 0 || ar.Facts.SampleCount == nil {
		return nil, newError(command.CodeInvalidState, "the asset has no normalized audio to transcribe.")
	}

	audio, ok := s.artifactPath(arts[norm].RelPath)
	if !ok {
		return nil, internalError()
	}

	audioMS := *ar.Facts.SampleCount * 1000 / transcriptSampleRate
	silences := silenceRangesMS(s.readArtifact(arts, "silence", ""))

	var runDir *ffmpeg.RunDir

	if s.media.Runner != nil {
		var rerr error
		if runDir, rerr = s.media.Runner.NewRunDir("transcribe-" + job.ID); rerr != nil {
			return nil, rerr
		}

		defer func() { _ = runDir.Remove() }()
	}

	pw.report(ctx, 0.06, "transcribing")

	raw, terr := provider.Transcribe(ctx, transcribe.Input{
		Audio: audio, InputDirs: []string{filepath.Join(s.workspace, "artifacts", hexHash)},
		Duration: time.Duration(audioMS) * time.Millisecond, Language: in.Language, Silences: silences, WorkDir: runDir,
	}, func(done, total time.Duration) {
		if total > 0 {
			pw.report(ctx, 0.06+0.86*float64(done)/float64(total), "transcribing")
		}
	})
	if terr != nil {
		return nil, terr
	}

	pw.report(ctx, 0.93, "writing the transcript")

	tr, stats := transcribe.Postprocess(raw, silences, audioMS)

	doc := transcriptFile{
		Version:           1,
		Untrusted:         true,
		Asset:             hash,
		Provider:          provider.Name(),
		ProviderVersion:   raw.Info["provider_version"],
		Model:             provider.Model(),
		ModelSHA256:       raw.Info["model_sha256"],
		Language:          tr.Language,
		LanguageRequested: in.Language,
		AudioMS:           audioMS,
		CreatedAt:         s.now().UTC().Format(time.RFC3339),
		Postprocess:       stats,
		Warnings:          []string{},
		Words:             tr.Words,
		Segments:          tr.Segments,
	}

	if verr := verifyTranscript(&doc, ar.Facts.SampleCount); verr != nil {
		return nil, verr
	}

	body, merr := json.Marshal(doc)
	if merr != nil {
		return nil, internalError()
	}

	rel := "artifacts/" + hexHash + "/transcript-1-" + in.Key + ".json"
	if werr := writeFileAtomic(filepath.Join(s.workspace, filepath.FromSlash(rel)), body); werr != nil {
		return nil, internalError()
	}

	sum := sha256.Sum256(body)

	if perr := s.store.Assets().PutArtifact(ctx, ArtifactRow{
		AssetHash: hash, Kind: "transcript", Profile: 1, Variant: in.Key, Ext: "json", RelPath: rel,
		SizeBytes: int64(len(body)), SHA256: hex.EncodeToString(sum[:]), CreatedMs: s.nowMs(),
	}); perr != nil {
		return nil, perr
	}

	return s.transcribeResult(in, body)
}

func (s *svc) transcribeResult(in transcribeInput, body []byte) (json.RawMessage, error) {
	var doc struct {
		Language string            `json:"language"`
		Words    []json.RawMessage `json:"words"`
	}

	if json.Unmarshal(body, &doc) != nil {
		return nil, newError("E_TRANSCRIBE_FAILED", "the stored transcript cannot be read.").
			withDetail("reason", "output_missing")
	}

	raw, err := json.Marshal(JobResult{Transcribe: &TranscribeResult{
		AssetID: in.AssetID, Language: doc.Language, Words: len(doc.Words),
	}})
	if err != nil {
		return nil, internalError()
	}

	return raw, nil
}

// transcriptFile is the transcript artifact of SDD-011 section 10.5.
type transcriptFile struct {
	Version           int                  `json:"version"`
	Untrusted         bool                 `json:"untrusted"`
	Asset             string               `json:"asset"`
	Provider          string               `json:"provider"`
	ProviderVersion   string               `json:"provider_version,omitempty"`
	Model             string               `json:"model"`
	ModelSHA256       string               `json:"model_sha256,omitempty"`
	Language          string               `json:"language"`
	LanguageRequested string               `json:"language_requested"`
	AudioMS           int64                `json:"audio_ms"`
	CreatedAt         string               `json:"created_at"`
	Postprocess       transcribe.Stats     `json:"postprocess"`
	Warnings          []string             `json:"warnings"`
	Words             []transcribe.Word    `json:"words"`
	Segments          []transcribe.Segment `json:"segments"`
}

// verifyTranscript is the verification step: the words are ordered, inside the
// audio and not overlapping, and the audio length agrees with the asset (SDD-011
// section 10.5).
func verifyTranscript(doc *transcriptFile, samples *int64) *Error {
	fail := func(why string) *Error {
		return newError("E_TRANSCRIBE_FAILED", "the transcript failed verification: %s.", why).
			withDetail("reason", "output_missing")
	}

	if samples != nil {
		if d := doc.AudioMS - *samples/(transcriptSampleRate/1000); d > 1 || d < -1 {
			return fail("the audio length does not match the asset")
		}
	}

	var prev int64

	for i, w := range doc.Words {
		if w.StartMS < 0 || w.EndMS <= w.StartMS || w.EndMS > doc.AudioMS || w.StartMS < prev {
			return fail(fmt.Sprintf("word %d is out of order or out of range", i))
		}

		prev = w.EndMS
	}

	return nil
}

// silenceRangesMS reads the -35 dB set of the silence artifact as milliseconds.
func silenceRangesMS(raw []byte) []transcribe.Range {
	var doc struct {
		SampleRate int `json:"sample_rate"`
		Sets       []struct {
			ThresholdDB int `json:"threshold_db"`
			Ranges      []struct {
				Start int64 `json:"start"`
				End   int64 `json:"end"`
			} `json:"ranges"`
		} `json:"sets"`
	}

	if json.Unmarshal(raw, &doc) != nil || doc.SampleRate < 1 {
		return nil
	}

	var out []transcribe.Range

	for _, set := range doc.Sets {
		if set.ThresholdDB != -35 {
			continue
		}

		for _, r := range set.Ranges {
			out = append(out, transcribe.Range{
				StartMS: r.Start * 1000 / int64(doc.SampleRate), EndMS: r.End * 1000 / int64(doc.SampleRate),
			})
		}
	}

	return out
}

// writeFileAtomic writes a file under a temporary name and renames it, so a
// reader never sees half of it.
func writeFileAtomic(path string, body []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}

	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())

		return err
	}

	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())

		return err
	}

	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		_ = os.Remove(tmp.Name())

		return err
	}

	if err := renameReplacing(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())

		return err
	}

	return nil
}

// renameReplacing renames over an existing file. On Windows the rename fails
// with a sharing violation while a reader (a status poll, a virus scanner) has
// the target open, and the reader lets go within milliseconds, so it retries
// briefly; elsewhere the first attempt succeeds.
func renameReplacing(from, to string) error {
	var err error

	for attempt := range 50 {
		if err = os.Rename(from, to); err == nil {
			return nil
		}

		if runtime.GOOS != "windows" {
			return err
		}

		time.Sleep(time.Duration(attempt+1) * time.Millisecond)
	}

	return err
}
