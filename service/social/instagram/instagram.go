// Package instagram is the Instagram connector (SDD-020 part 2): a Reel goes up through the resumable
// upload of the Instagram API with Facebook Login, to a professional account the person chose after signing
// in with their own Meta app. Everything it knows of Instagram is in this package and in service/social/meta;
// every request goes through the gated client of its connection, and no public address of the file is ever
// created or sent.
package instagram

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/publish"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
	"github.com/AlexGalhardo/stuarty/service/social/meta"
)

// The scopes the sign-in asks for (SDD-020 section 15): the Page permissions the Instagram account is
// reached through, and the two Instagram ones.
var scopes = []string{"pages_show_list", "pages_read_engagement", "instagram_basic", "instagram_content_publish"}

// Connector is the Instagram connector. The sign-in, the account and the Graph client are those of meta.Base.
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
		Network: publish.NetworkInstagram, Name: "Instagram", Scopes: scopes, Fields: "id,username",
		Offer: func(p meta.Page) (string, string, bool) {
			if p.IGID == "" {
				return "", "", false
			}

			title := "@" + p.IGUsername
			if p.Name != "" {
				title += " (" + p.Name + ")"
			}

			return p.IGID, title, true
		},
	})}
}

// Limits are the limits of an Instagram Reel, read on 2026-11-19 (SDD-020 sections 15 and 18). The length,
// the frame rate, the picture and the codecs are those of the export preset instagram-reels.
func Limits() publish.Limits {
	return publish.Limits{
		// Instagram has no title: it is the first line of the caption, which CaptionMax bounds as a whole.
		TitleMax: 2200, DescriptionMaxBytes: 8800, TagsMaxChars: 2200,
		FileMaxBytes: 300_000_000, Formats: []string{"mp4", "mov"},
		Visibilities: []publish.Visibility{publish.Public},
		Preset:       "instagram-reels", VideoCodecs: []string{"h264", "hevc"}, AudioCodecs: []string{"aac"},
		CaptionMax: 2200, HashtagsMax: 30, DailyPosts: 100,
		Source: "https://developers.facebook.com/docs/instagram-platform/instagram-graph-api/reference/ig-user/media",
		Read:   "2026-11-19",
	}
}

// Describe implements social.Connector.
func (c *Connector) Describe() social.Description {
	cfg := c.Config()

	return social.Description{
		Network: publish.NetworkInstagram, Name: "Instagram", Publishes: []string{"video"},
		Limits: Limits(), Endpoint: cfg.GraphBase, Hosts: cfg.Hosts(), Scopes: scopes,
		Guide:     "https://developers.facebook.com/docs/instagram-platform/content-publishing",
		GuideRead: "2026-11-19", AIDisclosure: true, Hints: hints,
	}
}

// hints are what a person does about an error of Instagram, by the kind of the error (SDD-020 section 19).
var hints = map[string]string{
	social.KindQuota: "Instagram lets an app publish 100 posts in a moving 24 hours and limits how often it may ask; " +
		"wait and publish the same post again, and the file that went up is not sent twice",
	social.KindAuth: "sign in to Instagram again from the Connections page and accept every permission; the token " +
		"stops working when the password changes, when you remove the app or when you lose your role on the Page",
	social.KindNotEligible: "check that the account is an Instagram professional account (Business or Creator) " +
		"linked to a Facebook Page, and that the Instagram app shows no restriction under Settings, Account status; " +
		"then publish again",
	social.KindInvalid: "fix what the reason names and publish again: a Reel is 3 seconds to 15 minutes, at most " +
		"300 MB, H.264 or HEVC with AAC, with a caption of at most 2,200 characters and 30 hashtags",
	social.KindRejected: "Instagram could not process the video; export it again with the Instagram Reels preset " +
		"and publish again",
	social.KindUnavailable: "try again in a few minutes; when Instagram was still processing the video, publishing " +
		"the same post again continues without sending the file again",
}

// Validate implements social.Connector.
func (c *Connector) Validate(p publish.Post) []publish.Issue { return publish.Validate(p, Limits()) }

// Status implements social.Connector: a published Reel is found by its id. A Reel is published by Publish
// only when Instagram finished processing it, so this connector has no processing state.
func (c *Connector) Status(ctx context.Context, cred connections.Credential, remoteID string) (social.Remote, error) {
	ch, err := c.Chosen(cred)
	if err != nil {
		return social.Remote{}, err
	}

	var out struct {
		ID string `json:"id"`
	}

	q := url.Values{"fields": {"id,permalink"}}

	if cerr := c.Call(ctx, cred.Client, http.MethodGet, url.PathEscape(remoteID), q, ch.Token, &out); cerr != nil {
		return social.Remote{}, cerr
	}

	if out.ID == "" {
		return social.Remote{}, social.NewError(social.KindUnavailable, publish.NetworkInstagram, "no_media", 0, nil)
	}

	return social.Remote{State: publish.StatePublished}, nil
}

// caption is the text of the Reel: the title, the description and the tags as hashtags (SDD-020 section 18).
func caption(p *publish.Post) string { return strings.TrimSpace(publish.Caption(p)) }
