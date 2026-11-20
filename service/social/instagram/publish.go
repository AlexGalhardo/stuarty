package instagram

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/meta"
)

// The container states of Instagram (SDD-020 section 15).
const (
	statusFinished   = "FINISHED"
	statusInProgress = "IN_PROGRESS"
	statusPublished  = "PUBLISHED"
	statusError      = "ERROR"
	statusExpired    = "EXPIRED"
)

// Publish implements social.Connector: the two steps at Instagram, inside one call (SDD-020 section 19).
// The day's limit is read first, the container is made and the file is sent to the address Instagram gave,
// the container is polled until it is FINISHED, and then published. What Instagram holds between two calls
// (the container and whether the file reached it) is kept in the session file, so the same post again
// continues where this one stopped and never sends a finished file twice.
func (c *Connector) Publish(
	ctx context.Context, cred connections.Credential, u social.Upload,
) (social.Published, error) {
	ch, err := c.Chosen(cred)
	if err != nil {
		return social.Published{}, err
	}

	f, size, err := meta.OpenFile(publish.NetworkInstagram, u.Path)
	if err != nil {
		return social.Published{}, err
	}

	defer func() { _ = f.Close() }()

	sess, err := c.container(ctx, cred.Client, ch, f, size, &u)
	if err != nil {
		return social.Published{}, err
	}

	if u.Report != nil {
		u.Report(0.9, "uploaded")
	}

	state, err := c.settle(ctx, cred.Client, ch, sess.ID)
	if err != nil {
		if se, ok := social.AsError(err); ok && se.Kind == social.KindRejected {
			_ = u.Sessions.Clear(u.Key)
		}

		return social.Published{}, err
	}

	if state == statusPublished {
		_ = u.Sessions.Clear(u.Key)

		return social.Published{RemoteID: sess.ID, State: publish.StatePublished}, nil
	}

	var out struct {
		ID string `json:"id"`
	}

	if cerr := c.Call(ctx, cred.Client, http.MethodPost, url.PathEscape(ch.ID)+"/media_publish",
		url.Values{"creation_id": {sess.ID}}, ch.Token, &out); cerr != nil {
		return social.Published{}, cerr
	}

	if out.ID == "" {
		return social.Published{}, social.NewError(social.KindUnavailable, publish.NetworkInstagram, "no_media", 0, nil)
	}

	_ = u.Sessions.Clear(u.Key)

	if u.Report != nil {
		u.Report(1, "published")
	}

	return social.Published{
		RemoteID: out.ID, URL: c.permalink(ctx, cred.Client, ch.Token, out.ID), State: publish.StatePublished,
	}, nil
}

// container makes sure a container exists and holds the whole file: from the session when one is kept, and
// new otherwise. A failed send is sent again, with a pause, up to Resumes times.
func (c *Connector) container(
	ctx context.Context, cl connections.Doer, ch meta.Chosen, f *os.File, size int64, u *social.Upload,
) (social.Session, error) {
	sess, have, err := u.Sessions.Load(u.Key)
	if err != nil {
		return social.Session{}, err
	}

	if have && sess.Size != size {
		_ = u.Sessions.Clear(u.Key)
		have = false
	}

	if have && sess.Uploaded {
		return sess, nil
	}

	cfg := c.Config()
	s := &sender{c: c, cl: cl, ch: ch, f: f, size: size, u: u, sess: sess, have: have}

	var last error

	for attempt := 0; attempt <= cfg.Resumes; attempt++ {
		if attempt > 0 {
			cfg.Wait(ctx, meta.Backoff(attempt))

			if ctx.Err() != nil {
				return social.Session{}, ctx.Err()
			}
		}

		err = s.attempt(ctx)
		if err == nil {
			return s.sess, nil
		}

		if !meta.Retryable(err) {
			return social.Session{}, err
		}

		last = err
	}

	if se, ok := social.AsError(last); ok {
		return social.Session{}, se
	}

	return social.Session{}, social.NewError(social.KindUnavailable, publish.NetworkInstagram, "", 0, last)
}

// sender is the state of the sending of one file: the container it goes to, if there is one yet.
type sender struct {
	c    *Connector
	cl   connections.Doer
	ch   meta.Chosen
	f    *os.File
	size int64
	u    *social.Upload

	sess social.Session
	have bool
}

// attempt makes the container when there is none and sends the file to it, from the start: the page shows
// the upload with offset 0. A rejected file drops the session; a container Instagram no longer knows is made
// again on the next attempt, and any other failure sends to the same one.
func (s *sender) attempt(ctx context.Context) error {
	if !s.have {
		if err := s.c.checkLimit(ctx, s.cl, s.ch); err != nil {
			return err
		}

		made, err := s.c.create(ctx, s.cl, s.ch, &s.u.Post, s.size)
		if err != nil {
			return err
		}

		s.sess, s.have = made, true

		if err := s.u.Sessions.Save(s.u.Key, s.sess); err != nil {
			return err
		}
	}

	err := s.c.Upload(ctx, s.cl, s.sess.URI, s.ch.Token, s.f, s.size, 0, s.u.Report)
	if err == nil {
		s.sess.Uploaded = true

		return s.u.Sessions.Save(s.u.Key, s.sess)
	}

	if se, ok := social.AsError(err); ok && (se.Kind == social.KindRejected || se.Reason == meta.ReasonMediaExpired) {
		_ = s.u.Sessions.Clear(s.u.Key)
		s.have = false
	}

	return err
}

// create is step 1: the container of a Reel with the resumable upload, and the address to send the file to.
// The caption, and the disclosure when the post carries it, are part of the container.
func (c *Connector) create(
	ctx context.Context, cl connections.Doer, ch meta.Chosen, p *publish.Post, size int64,
) (social.Session, error) {
	params := url.Values{
		"media_type": {"REELS"}, "upload_type": {"resumable"}, "caption": {caption(p)}, "share_to_feed": {"true"},
	}

	if p.AIDisclosure {
		params.Set("is_ai_generated", "true")
	}

	var out struct {
		ID  string `json:"id"`
		URI string `json:"uri"`
	}

	if err := c.Call(ctx, cl, http.MethodPost, url.PathEscape(ch.ID)+"/media", params, ch.Token, &out); err != nil {
		return social.Session{}, err
	}

	if out.ID == "" {
		return social.Session{}, social.NewError(
			social.KindUnavailable,
			publish.NetworkInstagram,
			"no_container",
			0,
			nil,
		)
	}

	cfg := c.Config()

	uri := out.URI
	if uri == "" {
		uri = strings.TrimRight(cfg.UploadBase, "/") + "/ig-api-upload/" + cfg.Version + "/" + url.PathEscape(out.ID)
	}

	return social.Session{URI: uri, ID: out.ID, Size: size, Created: cfg.Now().UnixMilli()}, nil
}

// settle polls the container until Instagram says it may be published. It returns FINISHED, or PUBLISHED
// for a container an earlier attempt published already. A failure of Instagram to process is rejected; a
// container still processing after the last poll is an error the person can try again, with the session
// kept.
func (c *Connector) settle(
	ctx context.Context, cl connections.Doer, ch meta.Chosen, container string,
) (string, error) {
	cfg := c.Config()

	for poll := range cfg.Polls {
		if poll > 0 {
			cfg.Wait(ctx, cfg.PollEvery)

			if ctx.Err() != nil {
				return "", ctx.Err()
			}
		}

		var out struct {
			Code string `json:"status_code"`
		}

		err := c.Call(ctx, cl, http.MethodGet, url.PathEscape(container),
			url.Values{"fields": {"status_code"}}, ch.Token, &out)
		if err != nil {
			// A failure to read the state is not a verdict on the video: keep asking until the ceiling, unless
			// the gate or the person said stop, or Instagram refused the account.
			if !meta.Retryable(err) {
				return "", err
			}

			continue
		}

		switch out.Code {
		case statusFinished, statusPublished:
			return out.Code, nil
		case statusError:
			return "", social.NewError(
				social.KindRejected,
				publish.NetworkInstagram,
				meta.ReasonProcessingFailed,
				0,
				nil,
			)
		case statusExpired:
			return "", social.NewError(social.KindRejected, publish.NetworkInstagram, meta.ReasonMediaExpired, 0, nil)
		case statusInProgress:
		}
	}

	return "", social.NewError(social.KindUnavailable, publish.NetworkInstagram, meta.ReasonStillProcessing, 0, nil)
}

// permalink reads the address of a published Reel. It is a courtesy: a failure leaves the address empty
// and the Reel is published all the same.
func (c *Connector) permalink(ctx context.Context, cl connections.Doer, token, mediaID string) string {
	var out struct {
		Permalink string `json:"permalink"`
	}

	err := c.Call(ctx, cl, http.MethodGet, url.PathEscape(mediaID), url.Values{"fields": {"permalink"}}, token, &out)
	if err != nil {
		return ""
	}

	return out.Permalink
}

// checkLimit reads how many posts the account published in the last 24 hours (content_publishing_limit) and
// refuses before any upload when the day is used up. A failure to read it is not a reason to stop: Instagram
// refuses the container itself with the same limit.
func (c *Connector) checkLimit(ctx context.Context, cl connections.Doer, ch meta.Chosen) error {
	var out struct {
		Data []struct {
			Usage  int `json:"quota_usage"`
			Config struct {
				Total int `json:"quota_total"`
			} `json:"config"`
		} `json:"data"`
	}

	err := c.Call(ctx, cl, http.MethodGet, url.PathEscape(ch.ID)+"/content_publishing_limit",
		url.Values{"fields": {"quota_usage,config"}}, ch.Token, &out)
	if err != nil {
		if meta.GateError(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}

		return nil
	}

	total := Limits().DailyPosts

	for _, d := range out.Data {
		if d.Config.Total > 0 {
			total = d.Config.Total
		}

		if d.Usage >= total {
			return social.NewError(social.KindQuota, publish.NetworkInstagram, meta.ReasonPublishingLimit, 0, nil)
		}
	}

	return nil
}
