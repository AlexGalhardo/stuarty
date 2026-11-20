package tiktok

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// The states of a post at TikTok (SDD-020 section 32).
const (
	statusUploading = "PROCESSING_UPLOAD"
	statusInbox     = "SEND_TO_USER_INBOX"
	statusComplete  = "PUBLISH_COMPLETE"
	statusFailed    = "FAILED"
)

// inboxMarker is the address kept in the session file of an upload. TikTok's own upload address holds an upload
// token and is never written to disk; the session keeps only the publish_id, which is no credential.
const inboxMarker = "tiktok-inbox"

// errRestart says the upload address stopped being good (it expired, or the progress did not match): the file
// is sent again from a new request.
var errRestart = errors.New("tiktok: the upload must start again")

type initAnswer struct {
	Data struct {
		PublishID string `json:"publish_id"`
		UploadURL string `json:"upload_url"`
	} `json:"data"`
}

type statusAnswer struct {
	Data struct {
		Status        string `json:"status"`
		FailReason    string `json:"fail_reason"`
		UploadedBytes *int64 `json:"uploaded_bytes"`
	} `json:"data"`
}

// Publish implements social.Connector: the upload of a draft to the inbox (SDD-020 section 37). The file is
// sent from the machine in chunks and never pulled from an address; what TikTok holds in the end is a draft,
// so the state is sent_as_draft and the person finishes it in the TikTok app.
func (c *Connector) Publish(
	ctx context.Context, cred connections.Credential, u social.Upload,
) (social.Published, error) {
	token, err := c.accessToken(ctx, cred)
	if err != nil {
		return social.Published{}, err
	}

	f, err := os.Open(u.Path)
	if err != nil {
		return social.Published{}, fmt.Errorf("tiktok: the file to upload cannot be opened: %w", err)
	}

	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil {
		return social.Published{}, err
	}

	size := st.Size()

	// A file that reached TikTok earlier is not sent again: only its state is read.
	if sess, have, lerr := u.Sessions.Load(u.Key); lerr == nil && have && sess.Uploaded && sess.Size == size &&
		sess.ID != "" {
		out, serr := c.settle(ctx, cred.Client, token, sess.ID, size, true, &u)
		if !errors.Is(serr, errRestart) {
			return out, serr
		}
	}

	var publishID string

	for restarts := 0; ; restarts++ {
		var perr error

		publishID, perr = c.send(ctx, cred.Client, token, f, size, &u)
		if perr == nil {
			break
		}

		if errors.Is(perr, errRestart) && restarts == 0 {
			continue
		}

		if errors.Is(perr, errRestart) {
			return social.Published{}, social.NewError(
				social.KindUnavailable, publish.NetworkTikTok, ReasonUploadExpired, 0, perr,
			)
		}

		return social.Published{}, perr
	}

	// The whole file is at TikTok: from here a retry reads the state and never sends the file again.
	if serr := u.Sessions.Save(
		u.Key,
		social.Session{URI: inboxMarker, ID: publishID, Uploaded: true, Size: size},
	); serr != nil {
		return social.Published{}, serr
	}

	if u.Report != nil {
		u.Report(0.92, "uploaded")
	}

	return c.settle(ctx, cred.Client, token, publishID, size, false, &u)
}

// plan is how a file of size bytes is cut: the chunk, and how many chunks. The last chunk takes what is left,
// so it is at most twice the chunk and under the 128 MB the page allows; a file smaller than the chunk is one.
func plan(size, chunk int64) (chunkSize, count int64) {
	if size <= chunk {
		return size, 1
	}

	return chunk, size / chunk
}

// send makes the request that opens the upload and sends the file in chunks, in order. It returns the
// publish_id. A chunk that fails with a transient error is sent again, with a pause, up to Resumes times; an
// address that expired is errRestart.
func (c *Connector) send(
	ctx context.Context, cl connections.Doer, token string, f *os.File, size int64, u *social.Upload,
) (string, error) {
	chunk, count := plan(size, c.cfg.ChunkBytes)

	body, err := json.Marshal(map[string]any{"source_info": map[string]any{
		"source": "FILE_UPLOAD", "video_size": size, "chunk_size": chunk, "total_chunk_count": count,
	}})
	if err != nil {
		return "", err
	}

	var init initAnswer
	if err := c.call(ctx, cl, http.MethodPost, "/v2/post/publish/inbox/video/init/", token, body, &init); err != nil {
		return "", err
	}

	if init.Data.PublishID == "" || init.Data.UploadURL == "" {
		return "", social.NewError(social.KindUnavailable, publish.NetworkTikTok, "no_upload_address", 0, nil)
	}

	if !c.ownAddress(init.Data.UploadURL) {
		// The address comes from the network. The gate would refuse another host; the file is not even offered.
		return "", social.NewError(social.KindInvalid, publish.NetworkTikTok, "upload_address", 0, nil)
	}

	for i := range count {
		first := i * chunk
		n := chunk

		if i == count-1 {
			n = size - first
		}

		if err := c.putChunk(ctx, cl, init.Data.UploadURL, f, size, first, n, i == count-1); err != nil {
			return "", err
		}

		if u.Report != nil {
			u.Report(0.9*float64(first+n)/float64(size), "uploading")
		}
	}

	return init.Data.PublishID, nil
}

// ownAddress says whether an upload address is on a host of the connection and uses a scheme the product
// speaks.
func (c *Connector) ownAddress(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return false
	}

	h := strings.ToLower(u.Hostname())

	return h != "" && (h == host(c.cfg.UploadBase) || h == host(c.cfg.APIBase))
}

// putChunk sends one chunk to the upload address with the Content-Range the page shows. The last chunk is
// answered 201, the others 206; any other answer of the 2xx class is accepted for a chunk that is not last.
func (c *Connector) putChunk(
	ctx context.Context, cl connections.Doer, address string, f *os.File, size, first, n int64, last bool,
) error {
	var lastErr error

	for attempt := 0; attempt <= c.cfg.Resumes; attempt++ {
		if attempt > 0 {
			c.cfg.Wait(ctx, backoff(attempt))

			if ctx.Err() != nil {
				return ctx.Err()
			}
		}

		lastErr = c.putOnce(ctx, cl, address, io.NewSectionReader(f, first, n), size, first, n, last)
		if lastErr == nil {
			return nil
		}

		if !transient(lastErr) {
			return lastErr
		}
	}

	return lastErr
}

func (c *Connector) putOnce(
	ctx context.Context, cl connections.Doer, address string, body io.Reader, size, first, n int64, last bool,
) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, address, body)
	if err != nil {
		return err
	}

	req.ContentLength = n
	req.Header.Set("Content-Type", "video/mp4")
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", first, first+n-1, size))

	res, err := cl.Do(req)
	if err != nil {
		return transportError(err)
	}

	defer func() { _ = res.Body.Close() }()

	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, maxBody))

	switch {
	case res.StatusCode == http.StatusCreated || res.StatusCode == http.StatusOK:
		return nil
	case res.StatusCode == http.StatusPartialContent && !last:
		return nil
	case res.StatusCode == http.StatusPartialContent:
		return social.NewError(
			social.KindUnavailable,
			publish.NetworkTikTok,
			ReasonUploadIncomplete,
			res.StatusCode,
			nil,
		)
	case res.StatusCode == http.StatusForbidden || res.StatusCode == http.StatusNotFound ||
		res.StatusCode == http.StatusRequestedRangeNotSatisfiable:
		// The address expired (it lasts an hour), is not a task of TikTok's, or the progress does not match:
		// ask for a new one.
		return errRestart
	case res.StatusCode >= http.StatusInternalServerError:
		return social.NewError(social.KindUnavailable, publish.NetworkTikTok, "", res.StatusCode, nil)
	default:
		return social.NewError(social.KindInvalid, publish.NetworkTikTok, ReasonInvalidParam, res.StatusCode, nil)
	}
}

// settle reads the state of the post until the draft is in the inbox: PollEvery apart, at most Polls times. For
// a session kept from an earlier attempt (resumed), a post still being uploaded with fewer bytes than the file
// is an upload that never finished: the session is dropped and errRestart sends the file again. More waiting
// than Polls is unavailable and the session is kept.
func (c *Connector) settle(
	ctx context.Context, cl connections.Doer, token, publishID string, size int64, resumed bool, u *social.Upload,
) (social.Published, error) {
	for poll := range c.cfg.Polls {
		if poll > 0 {
			c.cfg.Wait(ctx, c.cfg.PollEvery)

			if ctx.Err() != nil {
				return social.Published{}, ctx.Err()
			}
		}

		st, err := c.state(ctx, cl, token, publishID)
		if err != nil {
			return social.Published{}, err
		}

		switch st.Data.Status {
		case statusInbox, statusComplete:
			_ = u.Sessions.Clear(u.Key)

			if u.Report != nil {
				u.Report(1, "sent as a draft")
			}

			return social.Published{RemoteID: publishID, State: publish.StateSentAsDraft}, nil
		case statusFailed:
			_ = u.Sessions.Clear(u.Key)

			return social.Published{}, failReason(st.Data.FailReason)
		case statusUploading:
			// A session kept from an earlier attempt may be for an upload that never finished: TikTok then
			// holds fewer bytes than the file, and the file is sent again.
			if got := st.Data.UploadedBytes; resumed && got != nil && *got < size {
				_ = u.Sessions.Clear(u.Key)

				return social.Published{}, errRestart
			}
		}
	}

	return social.Published{}, social.NewError(
		social.KindUnavailable, publish.NetworkTikTok, ReasonStillProcessing, 0, nil,
	)
}

// state asks TikTok how the post of a publish_id is: POST /v2/post/publish/status/fetch/.
func (c *Connector) state(
	ctx context.Context, cl connections.Doer, token, publishID string,
) (statusAnswer, error) {
	body, err := json.Marshal(map[string]string{"publish_id": publishID})
	if err != nil {
		return statusAnswer{}, err
	}

	var out statusAnswer

	err = c.call(ctx, cl, http.MethodPost, "/v2/post/publish/status/fetch/", token, body, &out)

	return out, err
}

// Status implements social.Connector: the state of a post TikTok holds, read once.
func (c *Connector) Status(ctx context.Context, cred connections.Credential, remoteID string) (social.Remote, error) {
	token, err := c.accessToken(ctx, cred)
	if err != nil {
		return social.Remote{}, err
	}

	st, err := c.state(ctx, cred.Client, token, remoteID)
	if err != nil {
		return social.Remote{}, err
	}

	switch st.Data.Status {
	case statusInbox, statusComplete:
		return social.Remote{State: publish.StateSentAsDraft}, nil
	case statusFailed:
		reason := ""
		if se := failReason(st.Data.FailReason); se != nil {
			reason = se.Reason
		}

		return social.Remote{State: publish.StateFailed, Reason: reason}, nil
	default:
		return social.Remote{State: publish.StateProcessing}, nil
	}
}

// call makes one JSON request to the API and decodes a success answer into out. TikTok answers `error.code`
// "ok" for a success; any other code is a failure whatever the HTTP status. A failure is a connector error;
// the refusal of the gate passes through.
func (c *Connector) call(
	ctx context.Context, cl connections.Doer, method, path, token string, body []byte, out any,
) error {
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.APIBase+path, bytes.NewReader(body))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("Accept", "application/json")

	res, err := cl.Do(req)
	if err != nil {
		return transportError(err)
	}

	defer func() { _ = res.Body.Close() }()

	data, _ := io.ReadAll(io.LimitReader(res.Body, maxBody))

	if res.StatusCode != http.StatusOK {
		return classify(res.StatusCode, data)
	}

	if code := codeOf(data); code != "" && code != "ok" {
		return classify(res.StatusCode, data)
	}

	if out != nil {
		if jerr := json.Unmarshal(data, out); jerr != nil {
			return social.NewError(
				social.KindUnavailable,
				publish.NetworkTikTok,
				"unreadable_answer",
				res.StatusCode,
				nil,
			)
		}
	}

	return nil
}

// transient says whether a failure of a request is worth sending it again for: no answer, or a kind that says
// to try again later. The refusal of the gate, an upload address that must be renewed and a cancelled context
// are not.
func transient(err error) bool {
	if gateError(err) || errors.Is(err, errRestart) || errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if se, ok := social.AsError(err); ok {
		return se.Kind == social.KindUnavailable
	}

	return true
}

// backoff is the pause before the attempt-th try, doubling up to 32 seconds.
func backoff(attempt int) time.Duration { return time.Duration(1<<min(attempt, 5)) * time.Second }
