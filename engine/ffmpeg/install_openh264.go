package ffmpeg

import (
	"compress/bzip2"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
)

// Downloader does one HTTP request. In the product it is the client of outbound.HelperDownload, the
// only one that may dial, which checks every hop against the manifest hosts.
type Downloader interface {
	Do(req *http.Request) (*http.Response, error)
}

// InstallOpenH264Options is what the install step needs.
type InstallOpenH264Options struct {
	// ToolsDir is <workspace>/tools.
	ToolsDir string
	Manifest OpenH264Manifest
	Client   Downloader
	// GOOS and GOARCH select the entry; the runtime's when empty.
	GOOS, GOARCH string
	// Progress, when set, is told the bytes of the download read so far and the total.
	Progress func(done, total int64)
}

// InstallOpenH264 downloads the Cisco library of the platform, checks the size and the SHA-256 of the
// download and of the library inside it, and only then moves it to tools/openh264/<version>/. A wrong size
// or digest deletes everything of the attempt and leaves no library. It is the only function of the
// engine that downloads, and only the explicit install step calls it (SDD-007 6.8, T-26).
func InstallOpenH264(ctx context.Context, opt InstallOpenH264Options) (OpenH264State, error) {
	goos, goarch := opt.GOOS, opt.GOARCH
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	e, ok := opt.Manifest.For(goos, goarch)
	if !ok {
		return OpenH264State{}, openH264Error(OpenH264State{Reason: ReasonOpenH264Platform})
	}
	if !e.Published {
		return OpenH264State{}, installFailure(
			ReasonNotPublished,
			e,
			"the library for this platform is not published yet.",
		)
	}
	// The manifest is validated when it is parsed; a value built in memory gets the same rules here, so
	// an http:// or unlisted URL is never requested whoever made the manifest.
	if err := e.validate(opt.Manifest.Hosts); err != nil {
		return OpenH264State{}, installFailure(
			ReasonDownloadFailed, e, "The manifest entry for this platform is not allowed.",
		)
	}
	if opt.Client == nil || opt.ToolsDir == "" {
		return OpenH264State{}, internalErr("the install step has no download client or tools directory", nil)
	}

	staging := filepath.Join(opt.ToolsDir, ".download")
	if err := os.MkdirAll(staging, 0o700); err != nil {
		return OpenH264State{}, internalErr("cannot create the download directory", err)
	}
	part := filepath.Join(staging, "openh264.part")
	lib := filepath.Join(staging, "openh264.lib")
	defer func() { _ = os.Remove(part); _ = os.Remove(lib) }()

	if err := download(ctx, opt, e, part); err != nil {
		return OpenH264State{}, err
	}
	if err := unpack(part, lib, e); err != nil {
		return OpenH264State{}, err
	}

	dir := filepath.Dir(OpenH264Path(opt.ToolsDir, e))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return OpenH264State{}, internalErr("cannot create the library directory", err)
	}
	if err := os.Rename(lib, OpenH264Path(opt.ToolsDir, e)); err != nil {
		return OpenH264State{}, internalErr("cannot move the library into place", err)
	}
	return CheckOpenH264(opt.ToolsDir, opt.Manifest, goos, goarch), nil
}

func installFailure(reason string, e OpenH264Entry, msg string) *Error {
	return &Error{
		Code: CodeNoEncoder, Reason: reason, Message: msg,
		Hint: "Try the install again; if it keeps failing, the host serves a different file than the manifest pins.",
		Details: map[string]any{
			"reason": reason, "tool": OpenH264Tool, "host": hostOfURL(e.URL),
		},
	}
}

// digestMatches compares two hex digests in constant time.
func digestMatches(got, want string) bool {
	return len(got) == len(want) && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// download streams the body to path while it hashes it. The body is read at most to the size of the
// manifest plus one byte, so a longer answer is cut and refused; a different size or digest deletes the file.
func download(ctx context.Context, opt InstallOpenH264Options, e OpenH264Entry, path string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.URL, http.NoBody)
	if err != nil {
		return internalErr("cannot build the download request", err)
	}
	resp, err := opt.Client.Do(req)
	if err != nil {
		return installFailure(ReasonDownloadFailed, e, "The download of the OpenH264 library failed.")
	}
	defer func() { _ = resp.Body.Close() }()
	// The client checks every hop; this is the second lock, on the hop that answered.
	if final := resp.Request; final != nil && final.URL != nil &&
		(final.URL.Scheme != "https" || !slices.Contains(opt.Manifest.Hosts, final.URL.Hostname())) {
		return installFailure(
			ReasonDownloadFailed, e, "The OpenH264 download was answered by a host the manifest does not list.",
		)
	}
	if resp.StatusCode != http.StatusOK {
		return installFailure(ReasonDownloadFailed, e, "The download of the OpenH264 library was refused by the host.")
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return internalErr("cannot create the download file", err)
	}
	sum := sha256.New()
	body := io.LimitReader(resp.Body, e.Bytes+1)
	n, cerr := io.Copy(io.MultiWriter(f, sum), &progressReader{r: body, total: e.Bytes, fn: opt.Progress})
	if err := f.Close(); err != nil && cerr == nil {
		cerr = err
	}
	switch {
	case cerr != nil:
		return installFailure(ReasonDownloadFailed, e, "The download of the OpenH264 library was interrupted.")
	case n != e.Bytes:
		return installFailure(
			ReasonSizeMismatch,
			e,
			"The OpenH264 download is not the size the manifest pins and was deleted.",
		)
	case !digestMatches(hex.EncodeToString(sum.Sum(nil)), e.SHA256):
		return installFailure(
			ReasonChecksumMismatch,
			e,
			"The OpenH264 download is not the file the manifest pins and was deleted.",
		)
	}
	return nil
}

type progressReader struct {
	r     io.Reader
	done  int64
	total int64
	fn    func(done, total int64)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if p.fn != nil && n > 0 {
		p.fn(p.done, p.total)
	}
	return n, err
}

// unpack decompresses the archive that already matched its digest, reading at most the library size
// of the manifest plus one byte (a decompression bomb is cut), and checks the library itself.
func unpack(archive, out string, e OpenH264Entry) error {
	in, err := os.Open(archive)
	if err != nil {
		return internalErr("cannot read the download", err)
	}
	defer func() { _ = in.Close() }()

	f, err := os.OpenFile(out, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return internalErr("cannot create the library file", err)
	}
	sum := sha256.New()
	n, cerr := io.Copy(io.MultiWriter(f, sum), io.LimitReader(bzip2.NewReader(in), e.LibraryBytes+1))
	if err := f.Close(); err != nil && cerr == nil {
		cerr = err
	}
	switch {
	case cerr != nil:
		return installFailure(ReasonBadArchive, e, "The OpenH264 download is not a valid archive.")
	case n != e.LibraryBytes:
		return installFailure(
			ReasonSizeMismatch,
			e,
			"The OpenH264 library is not the size the manifest pins and was deleted.",
		)
	case !digestMatches(hex.EncodeToString(sum.Sum(nil)), e.LibrarySHA256):
		return installFailure(
			ReasonChecksumMismatch,
			e,
			"The OpenH264 library is not the file the manifest pins and was deleted.",
		)
	}
	return nil
}
