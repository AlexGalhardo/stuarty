package providers

import (
	"context"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
)

// videoFake offers the capabilities a test names, with the methods of video.
type videoFake struct {
	fake
	caps []string
}

func (v *videoFake) Descriptor() Descriptor {
	d := v.fake.Descriptor()
	d.Capabilities = v.caps

	return d
}

func (*videoFake) VideoLimits(string) provider.VideoLimits { return provider.VideoLimits{} }

func (*videoFake) Submit(
	context.Context, connections.Credential, provider.VideoRequest, *VideoFrame,
) (string, error) {
	return "task-1", nil
}

func (*videoFake) Poll(context.Context, connections.Credential, string) (VideoStatus, error) {
	return VideoStatus{State: VideoDone}, nil
}

func (*videoFake) Fetch(context.Context, connections.Credential, string, int64) (GeneratedFile, error) {
	return GeneratedFile{}, nil
}

func (*videoFake) Cancel(context.Context, connections.Credential, string) (bool, error) {
	return true, nil
}

// claimsVideo says it makes video and has none of its methods.
type claimsVideo struct{ fake }

func (c *claimsVideo) Descriptor() Descriptor {
	d := c.fake.Descriptor()
	d.Capabilities = []string{provider.CapabilityText, provider.CapabilityVideo}

	return d
}

func TestRegisterChecksTheVideoCapability(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	err := e.reg.Register(&claimsVideo{id: "liar-video"})
	if err == nil || !strings.Contains(err.Error(), "does not implement VideoGenerator") {
		t.Fatalf("a provider that claims video with no methods must fail, got %v", err)
	}

	if err := e.reg.Register(&videoFake{id: "clipper", caps: []string{
		provider.CapabilityText, provider.CapabilityVideo,
	}}); err != nil {
		t.Fatal(err)
	}

	if _, ok := e.reg.Video("clipper"); !ok {
		t.Error("the provider makes video")
	}

	if _, ok := e.reg.Video("nobody"); ok {
		t.Error("an unknown provider makes no video")
	}

	// The methods exist, but the descriptor offers text only: not a video provider.
	if err := e.reg.Register(&videoFake{id: "quiet-video", caps: []string{provider.CapabilityText}}); err != nil {
		t.Fatal(err)
	}

	if _, ok := e.reg.Video("quiet-video"); ok {
		t.Error("a provider whose descriptor has no video capability is not a video provider")
	}
}
