package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// progressReader reports how much of the file has gone out.
type progressReader struct {
	r      io.Reader
	sent   int64
	total  int64
	report func(float64, string)
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.sent += int64(n)

	if p.report != nil && p.total > 0 {
		p.report(float64(p.sent)/float64(p.total), "uploading")
	}

	return n, err
}

// Upload sends the file, from offset, to the upload address Meta gave, in one request (the resumable
// protocol of SDD-020 section 15). The token is in the Authorization header with the scheme OAuth that the
// upload host asks for. It returns nil only when the host answers success.
func (b *Base) Upload(
	ctx context.Context, cl connections.Doer, uploadURL, token string, f *os.File, size, offset int64,
	report func(float64, string),
) error {
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return err
	}

	body := &progressReader{r: f, sent: offset, total: size, report: report}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, body)
	if err != nil {
		return err
	}

	req.ContentLength = size - offset
	req.Header.Set("Authorization", "OAuth "+token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("offset", strconv.FormatInt(offset, 10))
	req.Header.Set("file_size", strconv.FormatInt(size, 10))
	req.Header.Set("Accept", "application/json")

	res, err := cl.Do(req)
	if err != nil {
		return TransportError(b.kind.Network, err)
	}

	defer func() { _ = res.Body.Close() }()

	data, _ := io.ReadAll(io.LimitReader(res.Body, MaxBody))

	if res.StatusCode != http.StatusOK {
		return Classify(b.kind.Network, res.StatusCode, data)
	}

	var ans struct {
		Success bool `json:"success"`
	}

	if json.Unmarshal(data, &ans) != nil || !ans.Success {
		return social.NewError(social.KindUnavailable, b.kind.Network, ReasonUploadFailed, res.StatusCode, nil)
	}

	return nil
}

// Retryable says whether a failure is one worth sending the file again for: no answer, or a kind that says
// to try again later. The refusal of the gate and a cancelled context are not.
func Retryable(err error) bool {
	if GateError(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if se, ok := social.AsError(err); ok {
		return se.Kind == social.KindUnavailable
	}

	return true
}

// Backoff is the pause before the attempt-th try, doubling up to 32 seconds.
func Backoff(attempt int) time.Duration { return time.Duration(1<<min(attempt, 5)) * time.Second }

// OpenFile opens the file to send and its size.
func OpenFile(network, path string) (*os.File, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: the file to upload cannot be opened: %w", network, err)
	}

	st, err := f.Stat()
	if err != nil {
		_ = f.Close()

		return nil, 0, err
	}

	return f, st.Size(), nil
}
