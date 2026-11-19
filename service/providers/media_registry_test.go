package providers

import (
	"context"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
)

// mediaFake offers the capabilities a test names, with the methods of each.
type mediaFake struct {
	fake
	caps []string
}

func (m *mediaFake) Descriptor() Descriptor {
	d := m.fake.Descriptor()
	d.Capabilities = m.caps

	return d
}

func (*mediaFake) ImageLimits(string) provider.ImageLimits { return provider.ImageLimits{} }

func (*mediaFake) GenerateImages(
	context.Context, connections.Credential, provider.ImageRequest, int64,
) (ImageResult, error) {
	return ImageResult{}, nil
}

func (*mediaFake) VoiceLimits(string) provider.VoiceLimits { return provider.VoiceLimits{} }

func (*mediaFake) Voices(context.Context, connections.Credential) ([]Voice, error) { return nil, nil }

func (*mediaFake) GenerateVoice(
	context.Context, connections.Credential, provider.VoiceRequest, int64,
) (VoiceResult, error) {
	return VoiceResult{}, nil
}

// onlyText says it makes images and speech and has the methods of neither.
type onlyText struct{ fake }

func (o *onlyText) Descriptor() Descriptor {
	d := o.fake.Descriptor()
	d.Capabilities = []string{provider.CapabilityText, provider.CapabilityImage, provider.CapabilityVoice}

	return d
}

func TestRegisterChecksTheImageAndVoiceCapabilities(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	for _, caps := range [][]string{{provider.CapabilityImage}, {provider.CapabilityVoice}} {
		err := e.reg.Register(&onlyText{id: "liar-" + caps[0]})
		if err == nil || !strings.Contains(err.Error(), "does not implement") {
			t.Errorf("%v: a provider that claims a capability with no methods must fail, got %v", caps, err)
		}
	}

	if err := e.reg.Register(&mediaFake{id: "maker", caps: []string{
		provider.CapabilityText, provider.CapabilityImage, provider.CapabilityVoice,
	}}); err != nil {
		t.Fatal(err)
	}

	if _, ok := e.reg.Image("maker"); !ok {
		t.Error("the provider makes images")
	}

	if _, ok := e.reg.Voice("maker"); !ok {
		t.Error("the provider makes speech")
	}

	if _, ok := e.reg.Image("nobody"); ok {
		t.Error("an unknown provider makes nothing")
	}
}

func TestACapabilityIsWhatTheDescriptorSays(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	// The methods exist, but the descriptor offers text only: the provider is not an image or a voice provider.
	if err := e.reg.Register(&mediaFake{id: "quiet", caps: []string{provider.CapabilityText}}); err != nil {
		t.Fatal(err)
	}

	if _, ok := e.reg.Image("quiet"); ok {
		t.Error("a provider whose descriptor has no image capability is not an image provider")
	}

	if _, ok := e.reg.Voice("quiet"); ok {
		t.Error("a provider whose descriptor has no voice capability is not a voice provider")
	}
}
