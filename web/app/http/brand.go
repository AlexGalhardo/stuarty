package httpapi

import (
	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/web"
)

type fontImportIn struct {
	Path string `json:"path" validate:"required" doc:"An absolute path to a TrueType or OpenType file on the machine that runs the server."`
}

type brandKitSetIn struct {
	Logo   string                       `json:"logo,omitempty"   doc:"sha256: and 64 hex digits of an image asset of the workspace."`
	Colors map[string]string            `json:"colors,omitempty" doc:"Name to #RRGGBB or #RRGGBBAA; at most 32."`
	Fonts  map[string]service.BrandFont `json:"fonts,omitempty"  doc:"Family to its files; at most 16 families, each file already in the font store."`
}

func (h *apiHandler) fontImport(c *web.Ctx, in fontImportIn) (service.FontImportResult, error) {
	res, err := h.Service.FontImport(c.Context(), service.FontImportRequest{Path: in.Path})

	return deref(res), err
}

func (h *apiHandler) brandKitGet(c *web.Ctx, _ noIn) (service.BrandKit, error) {
	res, err := h.Service.BrandKitGet(c.Context(), service.BrandKitGetRequest{})

	return deref(res), err
}

func (h *apiHandler) brandKitSet(c *web.Ctx, in brandKitSetIn) (service.BrandKit, error) {
	res, err := h.Service.BrandKitSet(c.Context(), service.BrandKitSetRequest{
		Logo: in.Logo, Colors: in.Colors, Fonts: in.Fonts,
	})

	return deref(res), err
}
