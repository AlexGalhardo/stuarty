package service

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/lint"
	enginestore "github.com/AlexGalhardo/stuarty/engine/store"
)

// MaxImageFileBytes is the largest file the browser may send into an image (SDD-021 section 10.2): 64 MiB, or
// STUARTY_MAX_IMPORT_BYTES when that is smaller. A larger picture is put on the server and imported by its path.
const MaxImageFileBytes int64 = 64 << 20

// ImageImportFileRequest brings a file that arrives as the bytes of a request into an image document: the
// options of ImageImport (Path and Hash are not given), the name the browser gave the file, and the bytes.
type ImageImportFileRequest struct {
	ImageImportRequest

	FileName string    // a label, never a path: only its base name is kept
	Body     io.Reader // the bytes of the file; at most MaxImageFileBytes are read
	Size     int64     // the declared length of the body, or -1 when it is not known
}

// ImageImportFile is ImageImport for a file chosen in the browser (SDD-021 section 10.2): the bytes enter the
// content store by hash and are decoded only by the ingest, with every limit of an import and the one of a file
// that comes in a request, before a document is made.
func (s *svc) ImageImportFile(ctx context.Context, req ImageImportFileRequest) (*ImageImportResult, error) {
	c, err := imageAuthorize(ctx, "ImageImportFile")
	if err != nil {
		return nil, err
	}

	if req.Body == nil {
		return nil, newError(command.CodeValidation, "the request carries no file.").
			withPath("body").withHint("send the bytes of the image as the body of the request")
	}

	if cerr := checkImageImport(&req.ImageImportRequest, true); cerr != nil {
		return nil, cerr
	}

	return s.runImageImport(ctx, c, &req.ImageImportRequest, &req)
}

// storeBody reads the bytes of a request into the content store (putImageFile). Nothing is read when the
// declared size is already over the limit or empty, or on a dry run.
func (s *svc) storeBody(
	ctx context.Context,
	c Caller,
	req *ImageImportRequest,
	file *ImageImportFileRequest,
) (hash, name string, early *ImageImportResult, err error) {
	if s.media.Content == nil {
		return "", "", nil, internalError()
	}

	name = displayName(file.FileName)
	if name == "" {
		name = "file"
	}

	limit := min(s.limits.MaxImportBytes, MaxImageFileBytes)

	if file.Size > limit {
		return "", "", nil, s.fileTooLarge(c, file.Size, limit)
	}

	if file.Size >= 0 {
		if lerr := s.checkImportSize(file.Size); lerr != nil {
			return "", "", nil, forCaller(c, lerr)
		}
	}

	if req.DryRun {
		return "", "", &ImageImportResult{
			UntrustedNotice: UntrustedNotice, State: ImageImportPending, Name: name, DryRun: true,
			Findings: []lint.Finding{}, Text: withNote(fmt.Sprintf("dry run: %q would be imported", name)),
		}, nil
	}

	hash, err = s.putImageFile(ctx, c, file.Body, name, limit)
	if err != nil {
		// The cap of a file in a request is lower than the import limit of the server: say so, not a variable
		// the caller never set.
		if se, ok := errors.AsType[*Error](err); ok && se.Code == enginestore.CodeLimitExceeded &&
			limit < s.limits.MaxImportBytes {
			return "", "", nil, s.fileTooLarge(c, limit+1, limit)
		}

		return "", "", nil, err
	}

	return hash, name, nil, nil
}

// fileTooLarge is the refusal of a file in a request over the cap: it names the way out, a path on the server.
func (s *svc) fileTooLarge(c Caller, value, ceiling int64) *Error {
	have, most := sizesApart(value, ceiling)

	return forCaller(
		c,
		newError(
			enginestore.CodeLimitExceeded,
			"The file is %s; the limit of a file sent in a request is %s.",
			have,
			most,
		).
			withHint("choose a smaller file, or put the file on the machine of the server and import it by its path").
			withDetail("limit", "upload_bytes").
			withDetail("value", value).
			withDetail("ceiling", ceiling),
	)
}
