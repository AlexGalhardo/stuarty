package httpapi

import (
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/connections"

	"github.com/AlexGalhardo/vukem/web"
)

type connectionRefIn struct {
	ID string `json:"id" validate:"required" doc:"A connection id, con_ and 10 characters."`
}

type connectionCreateIn struct {
	Kind     string   `json:"kind"            validate:"required" doc:"The kind of connection; test is the only one built in."`
	Label    string   `json:"label"           validate:"required" doc:"1 to 60 characters, shown to the person."`
	Endpoint string   `json:"endpoint"        validate:"required" doc:"The URL the connector calls first: https, or http for a loopback host."`
	Hosts    []string `json:"hosts,omitempty"                     doc:"Further hosts the connection may reach, at most 16; the host of the endpoint is always allowed."`
	Accepts  []string `json:"accepts"         validate:"required" doc:"What the destination may receive: text, image, video, audio, metadata, document."`
	Secret   string   `json:"secret"          validate:"required" doc:"The credential of the destination, 1 to 16384 bytes. It goes into the vault and is in no answer."`
}

func (h *apiHandler) connectionsList(c *web.Ctx, _ noIn) (service.ConnectionListResult, error) {
	res, err := h.Service.ConnectionList(c.Context(), service.ConnectionListRequest{})

	return deref(res), err
}

func (h *apiHandler) connectionsGet(c *web.Ctx, in connectionRefIn) (connections.Connection, error) {
	res, err := h.Service.ConnectionGet(c.Context(), service.ConnectionRef{ID: in.ID})

	return deref(res), err
}

func (h *apiHandler) connectionsCreate(c *web.Ctx, in connectionCreateIn) (connections.Connection, error) {
	res, err := h.Service.ConnectionCreate(c.Context(), service.ConnectionCreateRequest{
		Kind: in.Kind, Label: in.Label, Endpoint: in.Endpoint, Hosts: in.Hosts, Accepts: in.Accepts, Secret: in.Secret,
	})

	return deref(res), err
}

func (h *apiHandler) connectionsRemove(c *web.Ctx, in connectionRefIn) (connections.Connection, error) {
	res, err := h.Service.ConnectionRemove(c.Context(), service.ConnectionRef{ID: in.ID})

	return deref(res), err
}

func (h *apiHandler) connectionsTest(c *web.Ctx, in connectionRefIn) (service.ConnectionTestResult, error) {
	res, err := h.Service.ConnectionTest(c.Context(), service.ConnectionRef{ID: in.ID})

	return deref(res), err
}
