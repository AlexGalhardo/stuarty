package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/engine/export"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
)

var imageExtensions = map[string]string{export.ImagePNG: "png", export.ImageJPEG: "jpg", export.ImageWebP: "webp"}

// ImageExport renders the current revision of a document to a file in the workspace (SDD-021 section 6): the
// lint first (an error that blocks refuses the export), then the Rust renderer on its CPU path, then the encoder.
func (s *svc) ImageExport(ctx context.Context, req ImageExportRequest) (*ImageExportResult, error) {
	c, err := imageAuthorize(ctx, "ImageExport")
	if err != nil {
		return nil, err
	}

	format := req.Format
	if format == "" {
		format = export.ImagePNG
	}

	if _, ok := imageExtensions[format]; !ok {
		return nil, newError(command.CodeValidation, "the format %q is not png, jpeg or webp.", req.Format).
			withPath("format").withHint("use png, jpeg or webp")
	}

	w, h, verr := s.imageExportSize(&req)
	if verr != nil {
		return nil, verr
	}

	if s.workspace == "" {
		return nil, internalError() // the file has no place to go
	}

	if derr := s.checkDisk(); derr != nil {
		return nil, forCaller(c, derr)
	}

	row, doc, lerr := s.loadImageRead(ctx, s.store.Images(), c, req.ImageID)
	if lerr != nil {
		return nil, lerr
	}

	if blocking := blockingFindings(doc); len(blocking) > 0 {
		return nil, newError(CodeLintBlocking, "the image has %d error(s) that stop an export.", len(blocking)).
			withHint("fix them with image:edit, then export again").withDetail("findings", blocking)
	}

	pix, rerr := export.RenderImage(ctx, doc, s.imageRenderConfig(ctx, c))
	if rerr != nil {
		return nil, imageExportError(rerr)
	}

	body, eerr := export.EncodeImage(ctx, pix, export.ImageEncodeOptions{
		Format: format, Quality: req.Quality, Width: w, Height: h, WebP: s.webp,
	})
	if eerr != nil {
		return nil, imageExportError(eerr)
	}

	if s.limits.MaxExportBytes > 0 && int64(len(body)) > s.limits.MaxExportBytes {
		have, ceiling := sizesApart(int64(len(body)), s.limits.MaxExportBytes)

		return nil, newError(
			CodeExportFailed,
			"The export is %s; the export limit is %s (STUARTY_MAX_EXPORT_BYTES).",
			have,
			ceiling,
		).
			withDetail("reason", exportSizeLimitReason).
			withLimit("export_bytes", int64(len(body)), s.limits.MaxExportBytes).
			withDetail("variable", "STUARTY_MAX_EXPORT_BYTES")
	}

	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])
	name := fmt.Sprintf("%d-%s.%s", row.Revision, digest[:12], imageExtensions[format])

	out, aerr := filepath.Abs(filepath.Join(s.workspace, "exports", "images", row.ID, name))
	if aerr != nil || writeFileAtomic(out, body) != nil {
		return nil, internalError()
	}

	width, height := doc.Width, doc.Height
	if w != 0 || h != 0 {
		// one side alone keeps the aspect: the other is the one the encoder computed, not the canvas's
		width, height = export.ImageOutputSize(pix.Rect.Dx(), pix.Rect.Dy(), w, h)
	}

	res := &ImageExportResult{
		ImageID:  row.ID,
		Revision: row.Revision,
		Format:   format,
		Path:     out,
		Bytes:    int64(len(body)),
		SHA256:   digest,
		Width:    width,
		Height:   height,
		Text: fmt.Sprintf(
			"exported %s r%d as %s, %dx%d, %d bytes",
			row.ID,
			row.Revision,
			format,
			width,
			height,
			len(body),
		),
	}

	if jerr := s.recordImageExport(ctx, c, res); jerr != nil {
		return nil, jerr
	}

	// The absolute path of the server is for the person at the machine; a user of the network gets the file under
	// the workspace, which the download route serves by name (VV-301).
	if c.OwnerUserID != "" {
		res.Path = path.Join("exports", "images", res.ImageID, name)
	}

	return res, nil
}

// imageExportInput is the input an image.export job keeps: the image, and the user who exported it (empty in local
// mode), since the export of a template every user may read is still the export of one person.
type imageExportInput struct {
	ImageID     string `json:"image_id"`
	OwnerUserID string `json:"owner_user_id,omitempty"`
}

// recordImageExport writes the export job of the file just made, already succeeded: the rule of a job that a
// publication may take as a thumbnail (D-111). The job has no project, since an image has none; who may use it is
// the person who exported it, while they may still see the image (canSeeImageJob).
func (s *svc) recordImageExport(ctx context.Context, c Caller, res *ImageExportResult) error {
	// The path of the file is rebuilt from the image, the revision and the digest when it is used, and the job
	// keeps only those; the absolute path of the machine is not stored.
	stored := *res
	stored.Path, stored.JobID = path.Join("exports", "images", res.ImageID, filepath.Base(res.Path)), ""

	id := s.newIDs("job")
	input := marshalOrNull(imageExportInput{ImageID: res.ImageID, OwnerUserID: c.OwnerUserID})
	result := marshalOrNull(JobResult{Image: &stored})

	err := s.store.Write(ctx, func(ctx context.Context, tx Tx) error {
		now := s.nowMs()
		job := JobRow{
			ID: id, Kind: JobImageExport, Actor: c.Actor, State: JobQueued, Input: input,
			CreatedMs: now, UpdatedMs: now,
		}

		if err := tx.Jobs().CreateLimited(ctx, job, nil); err != nil {
			return err
		}

		if ok, err := tx.Jobs().Claim(ctx, id, now); err != nil || !ok {
			return errors.Join(err, internalError())
		}

		if ok, err := tx.Jobs().Finish(ctx, id, JobSucceeded, result, nil, now); err != nil || !ok {
			return errors.Join(err, internalError())
		}

		return nil
	})
	if err != nil {
		return asError(err)
	}

	res.JobID = id

	// The job is born finished and no stream follows it, so the event of a webhook is made here (SDD-022 section 3).
	s.fanWebhooks(Event{
		Type: "job.updated", JobID: id, Kind: JobImageExport, State: JobSucceeded, Result: &JobResult{Image: &stored},
		At: s.now(),
	})

	return nil
}

// imageJobOf is the image an image.export job exported and the file it wrote, as the job recorded them.
func imageJobOf(j *JobRow) (*ImageExportResult, bool) {
	var res JobResult

	if j.Kind != JobImageExport || json.Unmarshal(j.Result, &res) != nil || res.Image == nil {
		return nil, false
	}

	return res.Image, res.Image.ImageID != ""
}

// canSeeImageJob says whether the caller may use an image.export job: a key limited to one project reaches no
// image (SDD-021 section 10); in network mode only the user who exported it reaches it, so the export one user made
// of a shared template is not another user's; and the image must still be one the caller may see.
func (s *svc) canSeeImageJob(ctx context.Context, c Caller, j *JobRow) bool {
	img, ok := imageJobOf(j)
	if !ok || c.ProjectID != "" {
		return false
	}

	var in imageExportInput
	if json.Unmarshal(j.Input, &in) != nil || in.ImageID != img.ImageID ||
		c.OwnerUserID != "" && in.OwnerUserID != c.OwnerUserID {
		return false
	}

	row, err := s.store.Images().Get(ctx, img.ImageID)

	return err == nil && (visible(c, row.OwnerUserID) || SharedImageTemplate(row.ImageHead))
}

// imageExportSize is the size asked for: a preset, or a width and a height (either may be 0 to keep the aspect).
func (s *svc) imageExportSize(req *ImageExportRequest) (w, h int, err *Error) {
	if req.Quality < 0 || req.Quality > 100 {
		return 0, 0, newError(
			command.CodeValidation,
			"quality is %d; it must be 1 to 100.",
			req.Quality,
		).withPath("quality")
	}

	if req.Preset != "" {
		if req.Width != 0 || req.Height != 0 {
			return 0, 0, newError(command.CodeValidation, "give a preset or a size, not both.").
				withPath("preset").withHint("remove width and height, or remove preset")
		}

		p, ok := coreimage.PresetByName(req.Preset)
		if !ok {
			return 0, 0, newError(command.CodeValidation, "the preset %q is not known.", req.Preset).
				withPath("preset").withHint("use one of %s", strings.Join(coreimage.PresetNames(), ", "))
		}

		return p.Width, p.Height, nil
	}

	if req.Width < 0 || req.Height < 0 || req.Width > coreimage.MaxExportSide || req.Height > coreimage.MaxExportSide {
		return 0, 0, newError(
			command.CodeValidation,
			"the size is outside 1 to %d pixels on each side.",
			coreimage.MaxExportSide,
		).
			withPath("width").
			withHint("give a width and a height from 1 to %d, or leave them out", coreimage.MaxExportSide)
	}

	return req.Width, req.Height, nil
}

func blockingFindings(doc *coreimage.Document) []map[string]string {
	var out []map[string]string

	for _, f := range coreimage.Lint(doc, coreimage.LintEnv{}) {
		if coreimage.Blocking(f.Code) {
			out = append(out, map[string]string{"code": f.Code, "item": f.Item, "message": f.Message, "hint": f.Hint})
		}
	}

	return out
}

// imageAssetPath is the normalized PNG of an image of the workspace, by content hash.
// An asset the caller may not use is as missing as one that is not there.
func (s *svc) imageAssetPath(ctx context.Context, c Caller) func(hash string) (string, error) {
	allowed := s.imageAssetAccess(ctx, c)

	return func(hash string) (string, error) {
		if _, _, ok := allowed(hash); !ok {
			return "", fmt.Errorf("the image %s is not available to this caller", hash)
		}

		return s.preparedImagePath(hash)
	}
}

// imageLUTPath is the normalized .cube file of a lookup table of the workspace, by content hash (SDD-021 section
// 3.1). The same access rule as an image applies: a table the caller may not use is as missing as one that is not
// there.
func (s *svc) imageLUTPath(ctx context.Context, c Caller) func(hash string) (string, error) {
	allowed := s.imageAssetAccess(ctx, c)

	return func(hash string) (string, error) {
		if _, _, ok := allowed(hash); !ok {
			return "", fmt.Errorf("the lookup table %s is not available to this caller", hash)
		}

		return s.preparedArtifactPath(hash, "cube", "lookup table")
	}
}

// imageRenderConfig is what the render of an image needs from the workspace for this caller.
func (s *svc) imageRenderConfig(ctx context.Context, c Caller) export.ImageConfig {
	return export.ImageConfig{AssetPath: s.imageAssetPath(ctx, c), LUTPath: s.imageLUTPath(ctx, c)}
}

func (s *svc) preparedImagePath(hash string) (string, error) {
	return s.preparedArtifactPath(hash, "png", "image")
}

// preparedArtifactPath is the normalized artifact of a file of the workspace with the extension, if it exists.
// The digits are checked here again, whatever the caller did, because they become a folder name (T-94).
func (s *svc) preparedArtifactPath(hash, ext, what string) (string, error) {
	hexDigest, ok := strings.CutPrefix(hash, "sha256:")
	if !ok || !enginestore.ValidHash(hexDigest) {
		return "", fmt.Errorf("%q is not a content hash", hash)
	}

	p := filepath.Join(
		s.workspace,
		"artifacts",
		hexDigest,
		ingest.ArtifactName("normalized", ingest.ProfileNormalized, "", ext),
	)
	if _, err := os.Stat(p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("the prepared %s %s is missing; import it again", what, hash)
		}

		return "", err
	}

	return p, nil
}

// imageExportError maps the failure of the render or the encoder to the error of the export.
func imageExportError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	return fromEngine(err)
}
