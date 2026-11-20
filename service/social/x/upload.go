package x

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// Processing states of a media after the upload (SDD-020 section 32).
const (
	mediaPending    = "pending"
	mediaInProgress = "in_progress"
	mediaSucceeded  = "succeeded"
	mediaFailed     = "failed"
)

// mediaMarker is the address kept in the session file of an upload: X gives none, and the session is keyed
// by the media id, which is no credential.
const mediaMarker = "x-media"

type processing struct {
	State          string `json:"state"`
	CheckAfterSecs int    `json:"check_after_secs"`
}

type mediaAnswer struct {
	Data struct {
		ID         string      `json:"id"`
		Processing *processing `json:"processing_info"`
	} `json:"data"`
}

type postAnswer struct {
	Data struct {
		ID string `json:"id"`
	} `json:"data"`
}

// Publish implements social.Connector: the chunked upload of the video, then the one request that creates the
// post (SDD-020 section 36). The post is paid and cannot be undone, so that request is never repeated on a
// doubt, and a media that was uploaded is kept in the session so that publishing again does not send it again.
func (c *Connector) Publish(
	ctx context.Context, cred connections.Credential, u social.Upload,
) (social.Published, error) {
	token, err := c.accessToken(ctx, cred)
	if err != nil {
		return social.Published{}, err
	}

	f, err := os.Open(u.Path)
	if err != nil {
		return social.Published{}, fmt.Errorf("x: the file to upload cannot be opened: %w", err)
	}

	defer func() { _ = f.Close() }()

	st, err := f.Stat()
	if err != nil {
		return social.Published{}, err
	}

	mediaID, err := c.media(ctx, cred.Client, token, f, st.Size(), &u)
	if err != nil {
		return social.Published{}, err
	}

	if u.Report != nil {
		u.Report(0.9, "posting")
	}

	id, err := c.post(ctx, cred.Client, token, mediaID, &u.Post)
	if err != nil {
		return social.Published{}, err
	}

	_ = u.Sessions.Clear(u.Key)

	if u.Report != nil {
		u.Report(1, "published")
	}

	return social.Published{RemoteID: id, URL: c.cfg.PostURL + url.PathEscape(id), State: publish.StatePublished}, nil
}

// media makes sure a finished media exists: the one the session keeps, when X still holds it, and a new
// upload otherwise. It returns the media id to attach to the post.
func (c *Connector) media(
	ctx context.Context, cl connections.Doer, token string, f *os.File, size int64, u *social.Upload,
) (string, error) {
	sess, have, lerr := u.Sessions.Load(u.Key)
	if lerr == nil && have && sess.Uploaded && sess.Size == size && sess.ID != "" {
		switch rerr := c.ready(ctx, cl, token, sess.ID, nil, true); {
		case rerr == nil:
			return sess.ID, nil
		case isMediaGone(rerr):
			_ = u.Sessions.Clear(u.Key)
		default:
			return "", rerr
		}
	}

	id, err := c.initialize(ctx, cl, token, size)
	if err != nil {
		return "", err
	}

	if err := c.appendAll(ctx, cl, token, id, f, size, u); err != nil {
		return "", err
	}

	proc, err := c.finalize(ctx, cl, token, id)
	if err != nil {
		return "", err
	}

	// The whole file is at X: from here a retry polls the media and posts, and never uploads again.
	if serr := u.Sessions.Save(
		u.Key,
		social.Session{URI: mediaMarker, ID: id, Uploaded: true, Size: size},
	); serr != nil {
		return "", serr
	}

	if err := c.ready(ctx, cl, token, id, proc, false); err != nil {
		if se, ok := social.AsError(err); ok && se.Kind == social.KindRejected {
			_ = u.Sessions.Clear(u.Key)
		}

		return "", err
	}

	return id, nil
}

// isMediaGone says whether an error says X no longer holds the media (it expired, or was never there).
func isMediaGone(err error) bool {
	se, ok := social.AsError(err)

	return ok && se.Reason == ReasonMediaExpired
}

func (c *Connector) initialize(ctx context.Context, cl connections.Doer, token string, size int64) (string, error) {
	body, err := json.Marshal(map[string]any{
		"media_type": "video/mp4", "total_bytes": size, "media_category": "tweet_video",
	})
	if err != nil {
		return "", err
	}

	var out mediaAnswer
	if err := c.call(
		ctx,
		cl,
		http.MethodPost,
		"/2/media/upload/initialize",
		token,
		"application/json",
		body,
		&out,
	); err != nil {
		return "", err
	}

	if out.Data.ID == "" {
		return "", social.NewError(social.KindUnavailable, publish.NetworkX, "no_media", 0, nil)
	}

	return out.Data.ID, nil
}

// appendAll sends the file in segments, in order. A segment that fails with a transient error is sent again,
// with a pause, up to Resumes times: the same index again replaces the segment.
func (c *Connector) appendAll(
	ctx context.Context, cl connections.Doer, token, id string, f *os.File, size int64, u *social.Upload,
) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}

	buf := make([]byte, c.cfg.SegmentBytes)
	sent := int64(0)

	for index := 0; sent < size; index++ {
		n, err := io.ReadFull(f, buf)
		if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
			return err
		}

		if n == 0 {
			break
		}

		if err := c.appendSegment(ctx, cl, token, id, index, buf[:n]); err != nil {
			return err
		}

		sent += int64(n)

		if u.Report != nil {
			u.Report(0.85*float64(sent)/float64(size), "uploading")
		}
	}

	return nil
}

func (c *Connector) appendSegment(
	ctx context.Context, cl connections.Doer, token, id string, index int, data []byte,
) error {
	var body bytes.Buffer

	w := multipart.NewWriter(&body)
	_ = w.WriteField("segment_index", strconv.Itoa(index))

	part, err := w.CreateFormFile("media", "segment")
	if err != nil {
		return err
	}

	if _, werr := part.Write(data); werr != nil {
		return werr
	}

	if cerr := w.Close(); cerr != nil {
		return cerr
	}

	var last error

	for attempt := 0; attempt <= c.cfg.Resumes; attempt++ {
		if attempt > 0 {
			c.cfg.Wait(ctx, backoff(attempt))

			if ctx.Err() != nil {
				return ctx.Err()
			}
		}

		last = c.call(ctx, cl, http.MethodPost, "/2/media/upload/"+url.PathEscape(id)+"/append", token,
			w.FormDataContentType(), body.Bytes(), nil)
		if last == nil {
			return nil
		}

		if !transient(last) {
			return last
		}
	}

	return last
}

func (c *Connector) finalize(ctx context.Context, cl connections.Doer, token, id string) (*processing, error) {
	var out mediaAnswer
	if err := c.call(
		ctx,
		cl,
		http.MethodPost,
		"/2/media/upload/"+url.PathEscape(id)+"/finalize",
		token,
		"",
		nil,
		&out,
	); err != nil {
		return nil, err
	}

	return out.Data.Processing, nil
}

// ready waits until X has processed the media: the processing_info of the finalize answer, when it has one,
// then the status, after the seconds X asked for, at most Polls times. A finalize answer with no
// processing_info means the media is ready. A media that a session kept is asked about first (ask).
func (c *Connector) ready(
	ctx context.Context, cl connections.Doer, token, id string, proc *processing, ask bool,
) error {
	if proc == nil && !ask {
		return nil
	}

	for poll := 0; ; poll++ {
		if proc != nil {
			switch proc.State {
			case mediaSucceeded:
				return nil
			case mediaFailed:
				return social.NewError(social.KindRejected, publish.NetworkX, ReasonProcessingFailed, 0, nil)
			}
		}

		if poll >= c.cfg.Polls {
			return social.NewError(social.KindUnavailable, publish.NetworkX, ReasonStillProcessing, 0, nil)
		}

		if proc != nil {
			c.cfg.Wait(ctx, c.pause(proc))

			if ctx.Err() != nil {
				return ctx.Err()
			}
		}

		var out mediaAnswer

		q := url.Values{"command": {"STATUS"}, "media_id": {id}}
		if err := c.call(ctx, cl, http.MethodGet, "/2/media/upload?"+q.Encode(), token, "", nil, &out); err != nil {
			return err
		}

		if out.Data.Processing == nil {
			return nil
		}

		proc = out.Data.Processing
	}
}

// pause is how long X asked to wait before the next status request, within bounds.
func (c *Connector) pause(p *processing) time.Duration {
	d := time.Duration(p.CheckAfterSecs) * time.Second
	if d <= 0 {
		d = c.cfg.PollEvery
	}

	return min(d, 30*time.Second)
}

// post creates the post. It is called once: a failure that leaves it unknown whether X posted is
// outcome_unknown and is not retried by the scheduler, since a post is paid and cannot be undone.
func (c *Connector) post(
	ctx context.Context, cl connections.Doer, token, mediaID string, p *publish.Post,
) (string, error) {
	req := map[string]any{
		"text":  publish.Caption(p),
		"media": map[string]any{"media_ids": []string{mediaID}},
	}

	if p.AIDisclosure {
		req["made_with_ai"] = true
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	var out postAnswer

	if err := c.call(ctx, cl, http.MethodPost, "/2/tweets", token, "application/json", body, &out); err != nil {
		if se, ok := social.AsError(err); ok && se.Kind == social.KindUnavailable {
			return "", social.NewError(social.KindRejected, publish.NetworkX, ReasonOutcomeUnknown, se.Status, err)
		}

		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return "", social.NewError(social.KindRejected, publish.NetworkX, ReasonOutcomeUnknown, 0, err)
		}

		return "", err
	}

	if out.Data.ID == "" {
		return "", social.NewError(social.KindRejected, publish.NetworkX, ReasonOutcomeUnknown, http.StatusCreated, nil)
	}

	return out.Data.ID, nil
}

// call makes one request to the API and decodes a success answer into out. A failure is a connector error;
// the refusal of the gate passes through.
func (c *Connector) call(
	ctx context.Context, cl connections.Doer, method, path, token, contentType string, body []byte, out any,
) error {
	var rd io.Reader = http.NoBody
	if body != nil {
		rd = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.cfg.APIBase+path, rd)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	res, err := cl.Do(req)
	if err != nil {
		return transportError(err)
	}

	defer func() { _ = res.Body.Close() }()

	data, _ := io.ReadAll(io.LimitReader(res.Body, maxBody))

	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return classify(res.StatusCode, data, res.Header, c.cfg.Now())
	}

	if out != nil {
		if jerr := json.Unmarshal(data, out); jerr != nil && len(bytes.TrimSpace(data)) > 0 {
			return social.NewError(social.KindUnavailable, publish.NetworkX, "unreadable_answer", res.StatusCode, nil)
		}
	}

	return nil
}

// transient says whether a failure of a request is worth sending it again for: no answer, or a kind that says
// to try again later. The refusal of the gate and a cancelled context are not.
func transient(err error) bool {
	if gateError(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	if se, ok := social.AsError(err); ok {
		return se.Kind == social.KindUnavailable
	}

	return true
}

// backoff is the pause before the attempt-th try, doubling up to 32 seconds.
func backoff(attempt int) time.Duration { return time.Duration(1<<min(attempt, 5)) * time.Second }
