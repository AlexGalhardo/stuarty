package providers

import (
	"context"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
)

// SourceImage is the picture sent to a provider to be edited: its bytes and its format, which
// provider.CheckImage named ("png", "jpeg" or "webp").
type SourceImage struct {
	Data   []byte
	Format string
}

// ImageEditor is a provider that edits a picture it is sent (SDD-016 section 2.4). The estimate of an edit is
// the price per image of the person's table, so the adapter has no method for it.
type ImageEditor interface {
	Provider
	// EditLimits are what the provider documents for a model. It needs no network.
	EditLimits(model string) provider.EditLimits
	// RemoveBackground sends one picture and returns the same picture with a transparent background. No file
	// of the result is larger than maxBytes.
	RemoveBackground(
		ctx context.Context, c connections.Credential, req provider.EditRequest, src SourceImage, maxBytes int64,
	) (ImageResult, error)
}

func isImageEditor(p Provider) bool {
	_, ok := p.(ImageEditor)

	return ok
}

// Editor returns the image editor whose id is kind, when its descriptor says it edits images.
func (r *Registry) Editor(kind string) (ImageEditor, bool) {
	p, ok := r.Provider(kind)
	if !ok || !slices.Contains(p.Descriptor().Capabilities, provider.CapabilityImageEdit) {
		return nil, false
	}

	g, ok := p.(ImageEditor)

	return g, ok
}
