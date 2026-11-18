package httpapi

import (
	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/vukem-video/service"
)

type keyCreateIn struct {
	Name          string   `json:"name"                      validate:"required" doc:"1 to 60 characters."`
	Scopes        []string `json:"scopes"                    validate:"required" doc:"A non-empty subset of your scopes."`
	ProjectID     string   `json:"project_id,omitempty"                          doc:"Limits the key to one project; not with admin."`
	ExpiresInDays int      `json:"expires_in_days,omitempty"                     doc:"1 to 365, default 90; 1 to 30 with admin."`
}

type keyRevokeIn struct {
	KeyID string `json:"key_id" validate:"required"`
}

// keysCreate is POST /keys. The secret is in the answer once and never again,
// so the answer is never cached (SDD-007 section 4.2).
func (h *apiHandler) keysCreate(c *web.Ctx, in keyCreateIn) (service.KeyCreateResult, error) {
	res, err := h.Service.KeyCreate(c.Context(), service.KeyCreateRequest{
		Name: in.Name, Scopes: in.Scopes, ProjectID: in.ProjectID, ExpiresInDays: in.ExpiresInDays,
	})

	c.SetHeader("Cache-Control", "no-store")

	return deref(res), err
}
