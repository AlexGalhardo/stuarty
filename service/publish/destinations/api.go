package destinations

import (
	"context"
	"errors"

	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/social"
)

// apiPath is where the read API serves a published post (SDD-020 section 47).
const apiPath = "/api/v1/posts/"

// PostPath is the route of a published post.
func PostPath(slug string) string { return apiPath + slug }

type apiDestination struct{ cfg Config }

func newAPI(cfg Config) *apiDestination { return &apiDestination{cfg: cfg} }

func (a *apiDestination) Describe() Description {
	return Description{Kind: KindAPI, Name: "Blog: read API", Outbound: false, Hints: hints(map[string]string{
		social.KindInvalid: "another post already uses this slug in the read API: change the slug of one of them, then send again",
	})}
}

// ValidateConfig accepts the empty settings only: the destination has none.
func (a *apiDestination) ValidateConfig(secret []byte, _ string) error {
	var none struct{}
	if err := strictJSON(secret, &none); err != nil {
		return errors.New("the settings of this destination are {}")
	}

	return nil
}

func (a *apiDestination) Test(context.Context, connections.Credential) (connections.TestResult, error) {
	return connections.TestResult{
		OK:      true,
		Message: "nothing to reach: approved posts are published to this server's read API.",
	}, nil
}

// Send publishes the package in the workspace, where the routes of the read API read it. It makes no request.
func (a *apiDestination) Send(ctx context.Context, in *SendInput) (Result, error) {
	if err := in.Package.Check(); err != nil {
		return Result{}, fail(social.KindInvalid, KindAPI, "bad_package", 0, err)
	}

	if in.Publish == nil {
		return Result{}, fail(
			social.KindInvalid,
			KindAPI,
			"no_store",
			0,
			errors.New("the server has no place to publish to"),
		)
	}

	if err := in.Publish(ctx, in.Package); err != nil {
		if errors.Is(err, ErrSlugTaken) {
			return Result{}, fail(social.KindInvalid, KindAPI, "slug_taken", 0, err)
		}

		return Result{}, err
	}

	return Result{RemoteID: in.Package.Slug, URL: PostPath(in.Package.Slug), State: in.Package.Status}, nil
}
