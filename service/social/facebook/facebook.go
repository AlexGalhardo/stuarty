// Package facebook is the Facebook connector (SDD-020 part 2): a Reel goes up to a Page the person chose
// after signing in with their own Meta app, through the three steps of the Reels API (start, upload, finish).
// Everything it knows of Facebook is in this package and in service/social/meta; every request goes through
// the gated client of its connection, and no public address of the file is ever created or sent.
package facebook

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/meta"
)

// The scopes the sign-in asks for (SDD-020 section 15).
var scopes = []string{"pages_show_list", "pages_read_engagement", "pages_manage_posts"}

// ReelURL is the address a Reel is watched at, with its id appended. It is a choice of the product, not a
// field of the API: the answers of the Reels API carry no permalink.
const ReelURL = "https://www.facebook.com/reel/"

// Connector is the Facebook connector. The sign-in, the account and the Graph client are those of meta.Base.
type Connector struct {
	*meta.Base
}

var (
	_ social.Connector = (*Connector)(nil)
	_ social.Chooser   = (*Connector)(nil)
)

// New returns the connector; a zero field of cfg takes its default.
func New(cfg meta.Config) *Connector {
	return &Connector{Base: meta.NewBase(cfg, meta.Kind{
		Network: publish.NetworkFacebook, Name: "Facebook", Scopes: scopes, Fields: "id,name",
		Offer: func(p meta.Page) (string, string, bool) { return p.ID, p.Name, p.ID != "" },
	})}
}

// Limits are the limits of a Facebook Reel on a Page, read on 2026-11-19 (SDD-020 sections 15 and 18). The
// length, the frame rate and the picture are those of the export preset facebook-reels. The pages state no
// largest file and no longest description: the product takes at most 1 GiB, and the 2,200 characters and
// 30 hashtags of Instagram, which are stricter than anything a Page is known to refuse.
func Limits() publish.Limits {
	return publish.Limits{
		TitleMax: 2200, DescriptionMaxBytes: 8800, TagsMaxChars: 2200,
		FileMaxBytes: 1 << 30, Formats: []string{"mp4"},
		Visibilities: []publish.Visibility{publish.Public},
		Preset:       "facebook-reels", VideoCodecs: []string{"h264", "hevc"}, AudioCodecs: []string{"aac"},
		CaptionMax: 2200, HashtagsMax: 30, DailyPosts: 30,
		Source: "https://developers.facebook.com/docs/video-api/guides/reels-publishing", Read: "2026-11-19",
	}
}

// Describe implements social.Connector.
func (c *Connector) Describe() social.Description {
	cfg := c.Config()

	return social.Description{
		Network: publish.NetworkFacebook, Name: "Facebook", Publishes: []string{"video"},
		Limits: Limits(), Endpoint: cfg.GraphBase, Hosts: cfg.Hosts(), Scopes: scopes,
		Guide:     "https://developers.facebook.com/docs/video-api/guides/reels-publishing",
		GuideRead: "2026-11-19", AIDisclosure: true, Hints: hints,
	}
}

// hints are what a person does about an error of Facebook, by the kind of the error (SDD-020 section 19).
var hints = map[string]string{
	social.KindQuota: "Facebook lets an app publish 30 Reels to a Page in a moving 24 hours and limits how often it " +
		"may ask; wait and publish the same post again, and the file that went up is not sent twice",
	social.KindAuth: "sign in to Facebook again from the Connections page and accept every permission; the token " +
		"stops working when the password changes, when you remove the app or when you lose your role on the Page",
	social.KindNotEligible: "check that you can create content on the Page (Page access, Facebook Business Suite) " +
		"and that the Page has no restriction; then publish again",
	social.KindInvalid: "fix what the reason names and publish again: a Page Reel is 3 to 90 seconds, 9:16, at least " +
		"540 by 960, 24 to 60 frames a second, H.264 or HEVC with AAC",
	social.KindRejected: "Facebook could not process the video; export it again with the Facebook Reels preset " +
		"and publish again",
	social.KindUnavailable: "try again in a few minutes; publishing the same post again continues the upload where " +
		"it stopped",
}

// Validate implements social.Connector.
func (c *Connector) Validate(p publish.Post) []publish.Issue { return publish.Validate(p, Limits()) }

// The states of a video of Facebook (SDD-020 section 15).
const (
	videoReady          = "ready"
	videoProcessing     = "processing"
	videoUploadComplete = "upload_complete"
	videoUploadFailed   = "upload_failed"
	videoExpired        = "expired"
	videoError          = "error"
)

type videoStatus struct {
	Status struct {
		Video     string `json:"video_status"`
		Uploading struct {
			Bytes json.Number `json:"bytes_transfered"` //nolint:misspell // Meta spells the member so
		} `json:"uploading_phase"`
	} `json:"status"`
}

// video reads the status of an upload.
func (c *Connector) video(
	ctx context.Context, cl connections.Doer, token, id string,
) (videoStatus, error) {
	var out videoStatus

	err := c.Call(ctx, cl, http.MethodGet, url.PathEscape(id), url.Values{"fields": {"status"}}, token, &out)

	return out, err
}

// Status implements social.Connector: ready is published, an upload or processing under way is processing,
// and an error, a failed upload or an expired one is failed with a stable reason.
func (c *Connector) Status(ctx context.Context, cred connections.Credential, remoteID string) (social.Remote, error) {
	ch, err := c.Chosen(cred)
	if err != nil {
		return social.Remote{}, err
	}

	v, err := c.video(ctx, cred.Client, ch.Token, remoteID)
	if err != nil {
		return social.Remote{}, err
	}

	switch v.Status.Video {
	case videoReady:
		return social.Remote{State: publish.StatePublished}, nil
	case videoError:
		return social.Remote{State: publish.StateFailed, Reason: meta.ReasonProcessingFailed}, nil
	case videoUploadFailed:
		return social.Remote{State: publish.StateFailed, Reason: meta.ReasonUploadFailed}, nil
	case videoExpired:
		return social.Remote{State: publish.StateFailed, Reason: meta.ReasonMediaExpired}, nil
	case videoProcessing, videoUploadComplete:
	}

	return social.Remote{State: publish.StateProcessing}, nil
}
