package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/command"
	coreimage "github.com/AlexGalhardo/stuarty/core/image"
	"github.com/AlexGalhardo/stuarty/engine/export"
	"github.com/AlexGalhardo/stuarty/engine/ingest"
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

	row, doc, lerr := s.loadImage(ctx, s.store.Images(), c, req.ImageID)
	if lerr != nil {
		return nil, lerr
	}

	if blocking := blockingFindings(doc); len(blocking) > 0 {
		return nil, newError(CodeLintBlocking, "the image has %d error(s) that stop an export.", len(blocking)).
			withHint("fix them with image:edit, then export again").withDetail("findings", blocking)
	}

	pix, rerr := export.RenderImage(ctx, doc, export.ImageConfig{AssetPath: s.imageAssetPath(ctx, c)})
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
		width, height = pix.Rect.Dx(), pix.Rect.Dy()
		if w != 0 {
			width = w
		}

		if h != 0 {
			height = h
		}
	}

	return &ImageExportResult{
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
	}, nil
}

// imageExportSize is the size asked for: a preset, or a width and a height (either may be 0 to keep the aspect).
func (s *svc) imageExportSize(req *ImageExportRequest) (w, h int, err *Error) {
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

	if req.Quality < 0 || req.Quality > 100 {
		return 0, 0, newError(
			command.CodeValidation,
			"quality is %d; it must be 1 to 100.",
			req.Quality,
		).withPath("quality")
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
		if _, ok := allowed(hash); !ok {
			return "", fmt.Errorf("the image %s is not available to this caller", hash)
		}

		return s.preparedImagePath(hash)
	}
}

func (s *svc) preparedImagePath(hash string) (string, error) {
	hexDigest, ok := strings.CutPrefix(hash, "sha256:")
	if !ok || len(hexDigest) != 64 {
		return "", fmt.Errorf("%q is not a content hash", hash)
	}

	p := filepath.Join(
		s.workspace,
		"artifacts",
		hexDigest,
		ingest.ArtifactName("normalized", ingest.ProfileNormalized, "", "png"),
	)
	if _, err := os.Stat(p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("the prepared image %s is missing; import it again", hash)
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
