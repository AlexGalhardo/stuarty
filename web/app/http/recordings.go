package httpapi

import (
	"errors"
	"io"

	"github.com/AlexGalhardo/stuarty/service"

	"github.com/AlexGalhardo/vukem/api"
	"github.com/AlexGalhardo/vukem/errs"
	"github.com/AlexGalhardo/vukem/web"
)

// The routes of the screen recording (SDD-025). They carry no rule: the Service decides who may start a capture
// (a person in a privileged browser session) and core/recording holds the states and the limits (ADR-0008).

type recordingSourceIn struct {
	Kind string `json:"kind"           validate:"required" doc:"display or window."`
	ID   string `json:"id"             validate:"required" doc:"An id from GET /recording/sources."`
	Name string `json:"name,omitempty"                     doc:"The title the person saw in the picker; optional, plain text."`
}

type recordingStartIn struct {
	ProjectID string            `json:"project_id"    validate:"required" doc:"The project the recording will join once a person adds it."`
	Source    recordingSourceIn `json:"source"        validate:"required"`
	FPS       int               `json:"fps,omitempty"                     doc:"24, 30 (the default) or 60."`

	Microphone  string `json:"microphone,omitempty"   doc:"A microphone id from GET /recording/sources, or default; empty records no microphone."`
	SystemAudio bool   `json:"system_audio,omitempty" doc:"Record the sound the computer plays on a track of its own."`
	Camera      string `json:"camera,omitempty"       doc:"A camera id from GET /recording/sources; empty records no camera."`
	Events      bool   `json:"events,omitempty"       doc:"Save the pointer position and the clicks (never a key) beside the recording."`
}

type recordingEventsIn struct {
	ID     string `json:"id"                validate:"required" doc:"A recording id, rec_ and 10 characters."`
	FromMS int64  `json:"from_ms,omitempty"                     doc:"Only events from this time on, in milliseconds of recording time."`
	Kind   string `json:"kind,omitempty"                        doc:"clicks (the default) or all."`
	Limit  int    `json:"limit,omitempty"                       doc:"At most this many events, 1 to 5000; default 1000."`
}

type recordingRefIn struct {
	ID string `json:"id" validate:"required" doc:"A recording id, rec_ and 10 characters."`
}

type recordingListIn struct {
	ID string `json:"id" validate:"required" doc:"The project."`
}

type recordingPlaceIn struct {
	ID         string `json:"id"                    validate:"required" doc:"A recording a person added to the project."`
	TimelineID string `json:"timeline_id,omitempty"                     doc:"The timeline of the project; required when the project has several."`
}

func (h *apiHandler) recordingSources(c *web.Ctx, _ noIn) (service.RecordingSourcesResult, error) {
	res, err := h.Service.RecordingSources(c.Context(), service.RecordingSourcesRequest{})

	return deref(res), err
}

func (h *apiHandler) recordingStart(c *web.Ctx, in recordingStartIn) (service.Recording, error) {
	res, err := h.Service.RecordingStart(c.Context(), service.RecordingStartRequest{
		ProjectID: in.ProjectID, FPS: in.FPS,
		Source:     service.RecordingSource{Kind: in.Source.Kind, ID: in.Source.ID, Name: in.Source.Name},
		Microphone: in.Microphone, SystemAudio: in.SystemAudio, Camera: in.Camera, Events: in.Events,
	})

	return deref(res), err
}

func (h *apiHandler) recordingPause(c *web.Ctx, in recordingRefIn) (service.Recording, error) {
	res, err := h.Service.RecordingPause(c.Context(), service.RecordingRef{ID: in.ID})

	return deref(res), err
}

func (h *apiHandler) recordingResume(c *web.Ctx, in recordingRefIn) (service.Recording, error) {
	res, err := h.Service.RecordingResume(c.Context(), service.RecordingRef{ID: in.ID})

	return deref(res), err
}

func (h *apiHandler) recordingEvents(c *web.Ctx, in recordingEventsIn) (service.RecordingEventsResult, error) {
	res, err := h.Service.RecordingEvents(
		c.Context(),
		service.RecordingEventsRequest{ID: in.ID, FromMS: in.FromMS, Kind: in.Kind, Limit: in.Limit},
	)

	return deref(res), err
}

func (h *apiHandler) recordingStop(c *web.Ctx, in recordingRefIn) (service.Recording, error) {
	res, err := h.Service.RecordingStop(c.Context(), service.RecordingRef{ID: in.ID})

	return deref(res), err
}

func (h *apiHandler) recordingCancel(c *web.Ctx, in recordingRefIn) (service.Recording, error) {
	res, err := h.Service.RecordingCancel(c.Context(), service.RecordingRef{ID: in.ID})

	return deref(res), err
}

func (h *apiHandler) recordingGet(c *web.Ctx, in recordingRefIn) (service.Recording, error) {
	res, err := h.Service.RecordingGet(c.Context(), service.RecordingRef{ID: in.ID})

	return deref(res), err
}

func (h *apiHandler) recordingList(c *web.Ctx, in recordingListIn) (service.RecordingListResult, error) {
	res, err := h.Service.RecordingList(c.Context(), service.RecordingListRequest{ProjectID: in.ID})

	return deref(res), err
}

func (h *apiHandler) recordingConfirm(c *web.Ctx, in recordingRefIn) (service.Recording, error) {
	res, err := h.Service.RecordingConfirm(c.Context(), service.RecordingRef{ID: in.ID})

	return deref(res), err
}

func (h *apiHandler) recordingDismiss(c *web.Ctx, in recordingRefIn) (service.Recording, error) {
	res, err := h.Service.RecordingDismiss(c.Context(), service.RecordingRef{ID: in.ID})

	return deref(res), err
}

func (h *apiHandler) recordingPlace(c *web.Ctx, in recordingPlaceIn) (service.EditResult, error) {
	res, err := h.Service.RecordingPlace(
		c.Context(),
		service.RecordingPlaceRequest{ID: in.ID, TimelineID: in.TimelineID},
	)

	return deref(res), err
}

// The upload of a recording a browser captured (SDD-025 section 18). The routes carry no rule: the Service decides
// who may upload, in what order, how much and what the bytes become; the chunk route only raises the body limit
// for itself and hands the body on.

type recordingUploadBeginIn struct {
	ProjectID string `json:"project_id"       validate:"required"                doc:"The project the recording will join once a person adds it."`
	Container string `json:"container"        validate:"required,oneof=webm mp4" doc:"What the browser records: webm or mp4."`
	Camera    bool   `json:"camera,omitempty"                                    doc:"A second track, the camera, will be uploaded."`
	Label     string `json:"label,omitempty"                                     doc:"Optional plain text about what was shared."`
}

type recordingUploadChunkIn struct {
	ID        string      `json:"id"                   validate:"required" doc:"A recording id, rec_ and 10 characters."`
	Track     string      `json:"track"                validate:"required" doc:"screen or camera."`
	Index     int         `json:"index"                                    doc:"The chunk number on the track, from 0, in order."`
	SHA256    string      `json:"sha256"               validate:"required" doc:"The SHA-256 of the body, lowercase hexadecimal."`
	ElapsedMS int64       `json:"elapsed_ms,omitempty"                     doc:"The clock of the recorder, in milliseconds; informational."`
	Body      api.RawBody `json:"-"`
}

type recordingUploadCountIn struct {
	Kind   string `json:"kind"   validate:"required" doc:"screen or camera."`
	Chunks int    `json:"chunks"                     doc:"How many chunks the browser sent on the track."`
}

type recordingUploadFinishIn struct {
	ID        string                   `json:"id"                   validate:"required" doc:"A recording id, rec_ and 10 characters."`
	Tracks    []recordingUploadCountIn `json:"tracks,omitempty"                         doc:"The chunks sent on each track; the server checks them against what it stored."`
	ElapsedMS int64                    `json:"elapsed_ms,omitempty"                     doc:"The clock of the recorder, in milliseconds."`
}

func (h *apiHandler) recordingUploadBegin(c *web.Ctx, in recordingUploadBeginIn) (service.Recording, error) {
	res, err := h.Service.RecordingUploadBegin(c.Context(), service.RecordingUploadBeginRequest{
		ProjectID: in.ProjectID, Container: in.Container, Camera: in.Camera, Label: in.Label,
	})

	return deref(res), err
}

func (h *apiHandler) recordingUploadChunk(
	c *web.Ctx, in recordingUploadChunkIn,
) (service.RecordingUploadChunkResult, error) {
	res, err := h.Service.RecordingUploadChunk(c.Context(), service.RecordingUploadChunkRequest{
		ID: in.ID, Track: in.Track, Index: in.Index, SHA256: in.SHA256, ElapsedMS: in.ElapsedMS,
		Body: sizedBody{in.Body}, Size: c.Request().ContentLength,
	})

	return deref(res), err
}

// sizedBody reports the over-limit error of the framework as the one of the Service, so the answer is the
// E_PAYLOAD_TOO_LARGE of the product whichever side noticed the size.
type sizedBody struct{ r io.Reader }

func (b sizedBody) Read(p []byte) (int, error) {
	n, err := b.r.Read(p)
	if errors.Is(err, errs.ErrRequestTooLarge) {
		return n, service.ErrUploadBodyTooLarge
	}

	return n, err
}

func (h *apiHandler) recordingUploadFinish(c *web.Ctx, in recordingUploadFinishIn) (service.Recording, error) {
	counts := make([]service.RecordingUploadCount, len(in.Tracks))
	for i, t := range in.Tracks {
		counts[i] = service.RecordingUploadCount{Kind: t.Kind, Chunks: t.Chunks}
	}

	res, err := h.Service.RecordingUploadFinish(c.Context(), service.RecordingUploadFinishRequest{
		ID: in.ID, Tracks: counts, ElapsedMS: in.ElapsedMS,
	})

	return deref(res), err
}
