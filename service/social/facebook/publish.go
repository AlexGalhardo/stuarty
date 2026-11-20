package facebook

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/meta"
)

// Publish implements social.Connector: the three steps of the Reels API of SDD-020 section 19. The upload is
// started, the file is sent to the address Facebook gave (from the byte Facebook says it holds, after a
// failure), and the Reel is finished as published. What Facebook holds between two calls (the video id, the
// address and whether the file arrived) is in the session file, so the same post again continues where this
// one stopped.
func (c *Connector) Publish(
	ctx context.Context, cred connections.Credential, u social.Upload,
) (social.Published, error) {
	ch, err := c.Chosen(cred)
	if err != nil {
		return social.Published{}, err
	}

	f, size, err := meta.OpenFile(publish.NetworkFacebook, u.Path)
	if err != nil {
		return social.Published{}, err
	}

	defer func() { _ = f.Close() }()

	sess, err := c.sent(ctx, cred.Client, ch, f, size, &u)
	if err != nil {
		return social.Published{}, err
	}

	if u.Report != nil {
		u.Report(0.9, "uploaded")
	}

	if ferr := c.finish(ctx, cred.Client, ch, sess.ID, &u.Post); ferr != nil {
		return social.Published{}, ferr
	}

	_ = u.Sessions.Clear(u.Key)

	out := social.Published{RemoteID: sess.ID, URL: ReelURL + url.PathEscape(sess.ID), State: publish.StateProcessing}

	state, perr := c.settle(ctx, cred.Client, ch, sess.ID)
	if perr != nil {
		if se, ok := social.AsError(
			perr,
		); ok && se.Kind == social.KindRejected || meta.GateError(perr) ||
			ctx.Err() != nil {
			return out, perr
		}
		// The Reel exists; only the reading of its state failed. It stays processing and PublishStatus asks
		// again.
		return out, nil
	}

	out.State = state

	if u.Report != nil {
		u.Report(1, string(state))
	}

	return out, nil
}

// sent makes sure an upload exists and holds the whole file: from the session when one is kept, and new
// otherwise. A failed send is resumed, with a pause, up to Resumes times.
func (c *Connector) sent(
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

	return social.Session{}, social.NewError(social.KindUnavailable, publish.NetworkFacebook, "", 0, last)
}

// sender is the state of the sending of one file: the upload it goes to, if there is one yet.
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

// attempt starts the upload when there is none and sends the file from the byte Facebook holds. A file
// Facebook rejected, or an upload it no longer knows, drops the session; any other failure resumes.
func (s *sender) attempt(ctx context.Context) error {
	var offset int64

	if s.have {
		offset = s.held(ctx)
	} else {
		made, err := s.c.start(ctx, s.cl, s.ch, s.size)
		if err != nil {
			return err
		}

		s.sess, s.have = made, true

		if err := s.u.Sessions.Save(s.u.Key, s.sess); err != nil {
			return err
		}
	}

	if offset < s.size {
		err := s.c.Upload(ctx, s.cl, s.sess.URI, s.ch.Token, s.f, s.size, offset, s.u.Report)
		if err != nil {
			if se, ok := social.AsError(
				err,
			); ok &&
				(se.Kind == social.KindRejected || se.Reason == meta.ReasonMediaExpired) {
				_ = s.u.Sessions.Clear(s.u.Key)
				s.have = false
			}

			return err
		}
	}

	s.sess.Uploaded = true

	return s.u.Sessions.Save(s.u.Key, s.sess)
}

// held is how many bytes of the file Facebook says it has (the byte count of the uploading phase of the status), the offset to
// resume from; 0 when it cannot be read, which sends the file from the start.
func (s *sender) held(ctx context.Context) int64 {
	v, err := s.c.video(ctx, s.cl, s.ch.Token, s.sess.ID)
	if err != nil {
		return 0
	}

	n, err := v.Status.Uploading.Bytes.Int64()
	if err != nil || n < 0 || n > s.size {
		return 0
	}

	return n
}

// start is step 1: the upload of a Reel, with its video id and the address to send the file to.
func (c *Connector) start(
	ctx context.Context, cl connections.Doer, ch meta.Chosen, size int64,
) (social.Session, error) {
	var out struct {
		VideoID   string `json:"video_id"`
		UploadURL string `json:"upload_url"`
	}

	err := c.Call(ctx, cl, http.MethodPost, url.PathEscape(ch.ID)+"/video_reels",
		url.Values{"upload_phase": {"start"}}, ch.Token, &out)
	if err != nil {
		return social.Session{}, err
	}

	if out.VideoID == "" || out.UploadURL == "" {
		return social.Session{}, social.NewError(social.KindUnavailable, publish.NetworkFacebook, "no_upload", 0, nil)
	}

	return social.Session{URI: out.UploadURL, ID: out.VideoID, Size: size, Created: c.Config().Now().UnixMilli()}, nil
}

// finish is step 3: the Reel is published, with its title, its description and the disclosure when the post
// carries it. The description is the text of the post without its title, which Facebook takes apart.
func (c *Connector) finish(
	ctx context.Context, cl connections.Doer, ch meta.Chosen, videoID string, p *publish.Post,
) error {
	params := url.Values{
		"upload_phase": {"finish"}, "video_id": {videoID}, "video_state": {"PUBLISHED"},
		"title": {strings.TrimSpace(p.Title)}, "description": {description(p)},
	}

	if p.AIDisclosure {
		params.Set("is_ai_generated", "true")
	}

	var out struct {
		Success bool `json:"success"`
	}

	if err := c.Call(
		ctx,
		cl,
		http.MethodPost,
		url.PathEscape(ch.ID)+"/video_reels",
		params,
		ch.Token,
		&out,
	); err != nil {
		return err
	}

	if !out.Success {
		return social.NewError(social.KindUnavailable, publish.NetworkFacebook, "not_finished", 0, nil)
	}

	return nil
}

// description is the description and the tags as hashtags, without the title.
func description(p *publish.Post) string {
	return strings.TrimSpace(publish.Caption(&publish.Post{Description: p.Description, Tags: p.Tags}))
}

// settle polls the Reel until Facebook says it is ready. It returns published, or processing when the
// ceiling came first. A Reel Facebook failed to process is rejected.
func (c *Connector) settle(
	ctx context.Context, cl connections.Doer, ch meta.Chosen, videoID string,
) (publish.State, error) {
	cfg := c.Config()

	for poll := range cfg.Polls {
		if poll > 0 {
			cfg.Wait(ctx, cfg.PollEvery)

			if ctx.Err() != nil {
				return publish.StateProcessing, ctx.Err()
			}
		}

		v, err := c.video(ctx, cl, ch.Token, videoID)
		if err != nil {
			if !meta.Retryable(err) {
				return publish.StateProcessing, err
			}

			continue
		}

		switch v.Status.Video {
		case videoReady:
			return publish.StatePublished, nil
		case videoError:
			return publish.StateFailed, social.NewError(
				social.KindRejected, publish.NetworkFacebook, meta.ReasonProcessingFailed, 0, nil)
		case videoUploadFailed:
			return publish.StateFailed, social.NewError(
				social.KindRejected, publish.NetworkFacebook, meta.ReasonUploadFailed, 0, nil)
		case videoExpired:
			return publish.StateFailed, social.NewError(
				social.KindRejected, publish.NetworkFacebook, meta.ReasonMediaExpired, 0, nil)
		case videoProcessing, videoUploadComplete:
		}
	}

	return publish.StateProcessing, nil
}
