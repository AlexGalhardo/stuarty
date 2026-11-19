package youtube

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
	"strconv"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// categoryPeople is the YouTube category "People and Blogs", the one the upload says when the post names
// none.
const categoryPeople = "22"

type videoResource struct {
	ID string `json:"id"`
}

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

// Publish implements social.Connector: the resumable upload of SDD-020 section 7.2.
func (c *Connector) Publish(
	ctx context.Context,
	cred connections.Credential,
	u social.Upload,
) (social.Published, error) {
	token, err := c.accessToken(ctx, cred)
	if err != nil {
		return social.Published{}, err
	}

	f, err := os.Open(u.Path)
	if err != nil {
		return social.Published{}, fmt.Errorf("youtube: the file to upload cannot be opened: %w", err)
	}

	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil {
		return social.Published{}, err
	}

	id, err := c.upload(ctx, cred, token, f, st.Size(), u)
	if err != nil {
		return social.Published{}, err
	}

	if u.Report != nil {
		u.Report(0.95, "uploaded")
	}

	out := social.Published{RemoteID: id, URL: c.cfg.WatchURL + url.QueryEscape(id), State: publish.StateProcessing}

	if u.ThumbnailPath != "" {
		if terr := c.thumbnail(ctx, cred.Client, token, id, u.ThumbnailPath); terr != nil {
			if gateError(terr) || errors.Is(terr, context.Canceled) {
				return out, terr
			}

			if se, ok := social.AsError(terr); ok {
				out.ThumbnailError = se.Kind
			} else {
				out.ThumbnailError = social.KindUnavailable
			}
		}
	}

	state, perr := c.settle(ctx, cred, token, id)
	if perr != nil {
		var se *social.Error
		if errors.As(perr, &se) && se.Kind == social.KindRejected {
			return out, perr
		}
		// The video exists; only the reading of its state failed. It stays processing and PublishStatus
		// asks again.
		state = publish.StateProcessing
	}

	out.State = state

	return out, nil
}

// uploadRun is the state of one upload: its session, its token and what it learned from the last failure.
type uploadRun struct {
	c     *Connector
	cred  connections.Credential
	token string
	f     *os.File
	size  int64
	u     social.Upload

	sess social.Session
	have bool
	// needQuery is true when the next send must begin by asking the network how much of the file it
	// holds: after a session found on disk and after every failure. A session made a moment ago has none.
	needQuery bool
	// refreshed is true once the token was refreshed: a token that ran out is refreshed once per
	// upload, and a second 401 is a sign-in that does not work.
	refreshed bool
	lastErr   error
}

// failed decides what a failure of an attempt means: nil to try again, or the error to give up with.
func (r *uploadRun) failed(ctx context.Context, err error) error {
	r.lastErr = err

	if se, ok := social.AsError(
		err,
	); ok && !r.refreshed && se.Kind == social.KindAuth &&
		se.Status == http.StatusUnauthorized {
		fresh, rerr := r.c.accessToken(ctx, r.cred)
		if rerr != nil {
			return rerr
		}

		r.token, r.refreshed = fresh, true

		return nil
	}

	if !retryable(err) {
		return err
	}

	return nil
}

// begin makes sure the upload has a session. retry is true when the attempt failed in a way worth
// another attempt.
func (r *uploadRun) begin(ctx context.Context) (retry bool, err error) {
	if r.have {
		return false, nil
	}

	uri, err := r.c.startSession(ctx, r.cred.Client, r.token, r.u.Post, r.size)
	if err != nil {
		if ferr := r.failed(ctx, err); ferr != nil {
			return false, ferr
		}

		return true, nil
	}

	r.sess = social.Session{URI: uri, Size: r.size}
	r.needQuery, r.have = false, true

	return false, r.u.Sessions.Save(r.u.Key, r.sess)
}

// upload sends the file and returns the id of the video. It resumes: from the session kept for the key,
// and after every failure that the protocol says to resume from.
func (c *Connector) upload(
	ctx context.Context, cred connections.Credential, token string, f *os.File, size int64, u social.Upload,
) (string, error) {
	sess, have, err := u.Sessions.Load(u.Key)
	if err != nil {
		return "", err
	}

	if have && sess.Size != size {
		_ = u.Sessions.Clear(u.Key)
		have = false
	}

	r := &uploadRun{c: c, cred: cred, token: token, f: f, size: size, u: u, sess: sess, have: have, needQuery: have}

	for attempt := 0; attempt <= c.cfg.Resumes; attempt++ {
		if attempt > 0 {
			c.cfg.Wait(ctx, backoff(attempt))

			if ctx.Err() != nil {
				return "", ctx.Err()
			}
		}

		if retry, berr := r.begin(ctx); berr != nil {
			return "", berr
		} else if retry {
			continue
		}

		id, done, serr := c.sendFrom(ctx, cred.Client, r.token, r.sess.URI, f, size, r.needQuery, u.Report)
		r.needQuery = true

		switch {
		case serr == nil && done:
			_ = u.Sessions.Clear(u.Key)

			return id, nil
		case serr == nil:
			r.lastErr = errors.New("youtube: the upload ended before the whole file arrived")
		case errors.Is(serr, errSessionExpired):
			_ = u.Sessions.Clear(u.Key)
			r.have, r.lastErr = false, serr
		default:
			if ferr := r.failed(ctx, serr); ferr != nil {
				return "", ferr
			}
		}
	}

	if se, ok := social.AsError(r.lastErr); ok {
		return "", se
	}

	return "", social.NewError(social.KindUnavailable, publish.NetworkYouTube, "", 0, r.lastErr)
}

func backoff(attempt int) time.Duration { return time.Duration(1<<min(attempt, 5)) * time.Second }

var errSessionExpired = errors.New("youtube: the upload session expired")

// retryable says whether an upload failure is one the protocol says to resume from: no answer, or a 5xx.
func retryable(err error) bool {
	if gateError(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if se, ok := social.AsError(err); ok {
		return se.Kind == social.KindUnavailable
	}

	return true
}

// startSession is the POST that makes the upload session and answers its address.
func (c *Connector) startSession(
	ctx context.Context,
	cl connections.Doer,
	token string,
	p publish.Post,
	size int64,
) (string, error) {
	kids := p.MadeForKids != nil && *p.MadeForKids

	body, err := json.Marshal(map[string]any{
		"snippet": map[string]any{
			"title": strings.TrimSpace(
				p.Title,
			),
			"description": p.Description,
			"tags":        p.Tags,
			"categoryId":  categoryPeople,
		},
		"status": map[string]any{
			"privacyStatus": string(p.EffectiveVisibility()), "selfDeclaredMadeForKids": kids,
			"containsSyntheticMedia": p.AIDisclosure,
		},
	})
	if err != nil {
		return "", err
	}

	q := url.Values{"uploadType": {"resumable"}, "part": {"snippet,status"}}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.cfg.UploadBase+"/youtube/v3/videos?"+q.Encode(),
		bytes.NewReader(body),
	)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Length", strconv.FormatInt(size, 10))
	req.Header.Set("X-Upload-Content-Type", "video/mp4")

	res, err := cl.Do(req)
	if err != nil {
		return "", transportError(err)
	}

	defer func() { _ = res.Body.Close() }()

	data, _ := io.ReadAll(io.LimitReader(res.Body, maxBody))

	if res.StatusCode != http.StatusOK {
		return "", classify(res.StatusCode, data, res.Header)
	}

	loc := res.Header.Get("Location")
	if loc == "" {
		return "", social.NewError(social.KindUnavailable, publish.NetworkYouTube, "noSession", res.StatusCode, nil)
	}

	return loc, nil
}

// sendFrom asks the network how much of the file it has and sends the rest. It returns the id of the video
// when the network says the upload is complete.
func (c *Connector) sendFrom(
	ctx context.Context, cl connections.Doer, token, uri string, f *os.File, size int64, query bool,
	report func(float64, string),
) (string, bool, error) {
	var offset int64

	if query {
		var id string

		var done bool

		var err error

		offset, id, done, err = c.queryOffset(ctx, cl, token, uri, size)
		if err != nil || done {
			return id, done, err
		}
	}

	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", false, err
	}

	pr := &progressReader{r: f, sent: offset, total: size, report: report}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uri, pr)
	if err != nil {
		return "", false, err
	}

	req.ContentLength = size - offset
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "video/mp4")
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, size-1, size))

	res, err := cl.Do(req)
	if err != nil {
		return "", false, transportError(err)
	}

	defer func() { _ = res.Body.Close() }()

	data, _ := io.ReadAll(io.LimitReader(res.Body, maxBody))

	return c.finish(res, data)
}

// finish reads the answer to the PUT of the file.
func (c *Connector) finish(res *http.Response, data []byte) (string, bool, error) {
	switch res.StatusCode {
	case http.StatusOK, http.StatusCreated:
		var v videoResource
		if json.Unmarshal(data, &v) != nil || v.ID == "" {
			return "", false, social.NewError(
				social.KindUnavailable,
				publish.NetworkYouTube,
				"noVideo",
				res.StatusCode,
				nil,
			)
		}

		return v.ID, true, nil
	case http.StatusPermanentRedirect:
		return "", false, nil
	case http.StatusNotFound:
		return "", false, errSessionExpired
	default:
		return "", false, classify(res.StatusCode, data, res.Header)
	}
}

// queryOffset is the empty PUT that asks how much of the file the network holds (SDD-020 section 2).
// When the network says the upload is already complete, it returns the video.
func (c *Connector) queryOffset(
	ctx context.Context, cl connections.Doer, token, uri string, size int64,
) (int64, string, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, uri, http.NoBody)
	if err != nil {
		return 0, "", false, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Range", fmt.Sprintf("bytes */%d", size))
	req.ContentLength = 0

	res, err := cl.Do(req)
	if err != nil {
		return 0, "", false, transportError(err)
	}

	defer func() { _ = res.Body.Close() }()

	data, _ := io.ReadAll(io.LimitReader(res.Body, maxBody))

	if res.StatusCode != http.StatusPermanentRedirect {
		id, done, ferr := c.finish(res, data)

		return 0, id, done, ferr
	}

	return rangeEnd(res.Header.Get("Range")), "", false, nil
}

// rangeEnd reads "bytes=0-999" as 1000, the first byte still to send; no header is 0.
func rangeEnd(h string) int64 {
	_, last, ok := strings.Cut(strings.TrimPrefix(h, "bytes="), "-")
	if !ok {
		return 0
	}

	n, err := strconv.ParseInt(strings.TrimSpace(last), 10, 64)
	if err != nil || n < 0 {
		return 0
	}

	return n + 1
}

// thumbnail sets the cover of a video that was uploaded.
func (c *Connector) thumbnail(ctx context.Context, cl connections.Doer, token, id, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}

	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil {
		return err
	}

	mime := "image/jpeg"
	if strings.HasSuffix(strings.ToLower(path), ".png") {
		mime = "image/png"
	}

	q := url.Values{"videoId": {id}, "uploadType": {"media"}}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.cfg.UploadBase+"/youtube/v3/thumbnails/set?"+q.Encode(),
		f,
	)
	if err != nil {
		return err
	}

	req.ContentLength = st.Size()
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", mime)

	res, err := cl.Do(req)
	if err != nil {
		return transportError(err)
	}

	defer func() { _ = res.Body.Close() }()

	data, _ := io.ReadAll(io.LimitReader(res.Body, maxBody))

	if res.StatusCode != http.StatusOK {
		return classify(res.StatusCode, data, res.Header)
	}

	return nil
}

type videoStatus struct {
	Items []struct {
		Status struct {
			UploadStatus    string `json:"uploadStatus"`
			FailureReason   string `json:"failureReason"`
			RejectionReason string `json:"rejectionReason"`
		} `json:"status"`
		ProcessingDetails struct {
			ProcessingStatus string `json:"processingStatus"`
		} `json:"processingDetails"`
	} `json:"items"`
}

// Status implements social.Connector: videos.list for the state of one video (1 unit of quota).
func (c *Connector) Status(ctx context.Context, cred connections.Credential, remoteID string) (social.Remote, error) {
	token, err := c.accessToken(ctx, cred)
	if err != nil {
		return social.Remote{}, err
	}

	return c.status(ctx, cred.Client, token, remoteID)
}

func (c *Connector) status(ctx context.Context, cl connections.Doer, token, id string) (social.Remote, error) {
	q := url.Values{"part": {"status,processingDetails"}, "id": {id}}

	res, err := c.get(ctx, cl, token, c.cfg.APIBase+"/youtube/v3/videos?"+q.Encode())
	if err != nil {
		return social.Remote{}, transportError(err)
	}

	defer func() { _ = res.Body.Close() }()

	data, _ := io.ReadAll(io.LimitReader(res.Body, maxBody))

	if res.StatusCode != http.StatusOK {
		return social.Remote{}, classify(res.StatusCode, data, res.Header)
	}

	var v videoStatus
	if json.Unmarshal(data, &v) != nil || len(v.Items) == 0 {
		return social.Remote{State: publish.StateFailed, Reason: "videoNotFound"}, nil
	}

	st := v.Items[0].Status

	switch st.UploadStatus {
	case "processed":
		if v.Items[0].ProcessingDetails.ProcessingStatus == "failed" {
			return social.Remote{State: publish.StateFailed, Reason: "processingFailed"}, nil
		}

		return social.Remote{State: publish.StatePublished}, nil
	case "failed":
		return social.Remote{State: publish.StateFailed, Reason: orReason(st.FailureReason, "failed")}, nil
	case "rejected":
		return social.Remote{State: publish.StateFailed, Reason: orReason(st.RejectionReason, "rejected")}, nil
	case "deleted":
		return social.Remote{State: publish.StateFailed, Reason: "deleted"}, nil
	default:
		return social.Remote{State: publish.StateProcessing}, nil
	}
}

func orReason(r, fallback string) string {
	if r == "" {
		return fallback
	}

	return r
}

// settle reads the state of a new video up to Polls times: YouTube is usually done within seconds for a
// short video, and when it is not the result stays processing and PublishStatus asks again.
func (c *Connector) settle(ctx context.Context, cred connections.Credential, token, id string) (publish.State, error) {
	state := publish.StateProcessing

	for i := range c.cfg.Polls {
		if i > 0 {
			c.cfg.Wait(ctx, c.cfg.PollEvery)

			if ctx.Err() != nil {
				return state, ctx.Err()
			}
		}

		r, err := c.status(ctx, cred.Client, token, id)
		if err != nil {
			return state, err
		}

		if r.State == publish.StatePublished {
			return publish.StatePublished, nil
		}

		if r.State == publish.StateFailed {
			return publish.StateFailed, social.NewError(social.KindRejected, publish.NetworkYouTube, r.Reason, 0, nil)
		}
	}

	return state, nil
}
