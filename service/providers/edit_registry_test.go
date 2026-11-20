package providers

import (
	"context"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
)

// editFake says it edits images and has the methods.
type editFake struct {
	fake
	caps []string
}

func (m *editFake) Descriptor() Descriptor {
	d := m.fake.Descriptor()
	d.Capabilities = m.caps

	return d
}

func (*editFake) EditLimits(string) provider.EditLimits { return provider.EditLimits{} }

func (*editFake) RemoveBackground(
	context.Context, connections.Credential, provider.EditRequest, SourceImage, int64,
) (ImageResult, error) {
	return ImageResult{}, nil
}

func TestRegisterChecksTheImageEditCapability(t *testing.T) {
	t.Parallel()

	e := newEnv(t)

	// It claims the capability and has no method of it.
	err := e.reg.Register(&mediaFake{id: "liar", caps: []string{provider.CapabilityText, provider.CapabilityImageEdit}})
	if err == nil || !strings.Contains(err.Error(), "does not implement ImageEditor") {
		t.Fatalf("a provider that claims image_edit with no methods must fail, got %v", err)
	}

	if err := e.reg.Register(&editFake{id: "editor", caps: []string{provider.CapabilityImageEdit}}); err != nil {
		t.Fatal(err)
	}

	if _, ok := e.reg.Editor("editor"); !ok {
		t.Error("the provider edits images")
	}

	if _, ok := e.reg.Editor("nobody"); ok {
		t.Error("an unknown provider edits nothing")
	}

	// The methods exist, but the descriptor does not offer the capability.
	if err := e.reg.Register(&editFake{id: "quiet", caps: []string{provider.CapabilityText}}); err != nil {
		t.Fatal(err)
	}

	if _, ok := e.reg.Editor("quiet"); ok {
		t.Error("a provider whose descriptor has no image_edit capability is not an image editor")
	}
}
