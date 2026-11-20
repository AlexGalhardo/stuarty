package google

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/core/provider"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/providers"
	"github.com/AlexGalhardo/stuarty/service/providers/wire"
)

const (
	// Model is the model of the Gemini API that makes video (SDD-016 section 2.3).
	Model = "gemini-omni-1.2-flash"

	// resolution is the one the price of the table is meant for: the documented default, 720p.
	resolution = "720p"

	// answerSlack is what an answer may hold besides the clip, in bytes.
	answerSlack = 256 << 10

	// pollAnswer is the most read of a poll: the answer of a finished interaction carries the clip inline.
	pollAnswer = 128 << 20
)

// VideoLimits implements providers.VideoGenerator with what the documentation of the Gemini API says of the
// Omni models: 3 to 15 whole seconds, 16:9 or 9:16, and an image as the first frame. The provider is the judge
// of any other model.
func (*Adapter) VideoLimits(model string) provider.VideoLimits {
	if strings.HasPrefix(model, "gemini-omni") {
		return provider.VideoLimits{MinSeconds: 3, MaxSeconds: 15, Aspects: []string{"16:9", "9:16"}}
	}

	return provider.VideoLimits{}
}

type part struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mime_type,omitempty"`
	Data     string `json:"data,omitempty"`
}

type responseFormat struct {
	Type        string `json:"type"`
	AspectRatio string `json:"aspect_ratio"`
	Resolution  string `json:"resolution"`
	Duration    string `json:"duration"`
}

type interactionRequest struct {
	Model          string         `json:"model"`
	Input          any            `json:"input"`
	ResponseFormat responseFormat `json:"response_format"`
	Background     bool           `json:"background"`
	Store          bool           `json:"store"`
}

type apiError struct {
	Code string `json:"code"`
}

type interaction struct {
	ID     string     `json:"id"`
	Status string     `json:"status"`
	Errors []apiError `json:"errors"`
	Steps  []struct {
		Type    string `json:"type"`
		Content []struct {
			Type     string `json:"type"`
			MimeType string `json:"mime_type"`
			Data     string `json:"data"`
		} `json:"content"`
	} `json:"steps"`
}

// Submit implements providers.VideoGenerator: POST /interactions with background set, so the answer is the id of
// the interaction and the generation goes on at the provider. Without a first frame the input is the prompt; with
// one it is the image and then the text. The id is the task id.
func (*Adapter) Submit(
	ctx context.Context,
	c connections.Credential,
	req provider.VideoRequest,
	frame *providers.VideoFrame,
) (string, error) {
	var input any = req.Prompt

	if frame != nil {
		input = []part{
			{Type: "image", MimeType: "image/" + frame.Format, Data: base64.StdEncoding.EncodeToString(frame.Data)},
			{Type: "text", Text: req.Prompt},
		}
	}

	body := interactionRequest{
		Model: req.Model, Input: input, Background: true, Store: true,
		ResponseFormat: responseFormat{
			Type: "video", AspectRatio: req.Aspect, Resolution: resolution, Duration: strconv.Itoa(req.Seconds) + "s",
		},
	}

	var ans interaction

	call := wire.Call{Method: http.MethodPost, Path: "/interactions", Header: headers(c), Body: body}
	if err := wire.JSON(ctx, c, call, &ans); err != nil {
		return "", err
	}

	if !provider.ValidTaskID(ans.ID) {
		return "", errUnreadable()
	}

	return ans.ID, nil
}

func taskPath(taskID string) (string, error) {
	if !provider.ValidTaskID(taskID) {
		return "", &providers.Error{Kind: providers.KindBadRequest}
	}

	return "/interactions/" + url.PathEscape(taskID), nil
}

// Poll implements providers.VideoGenerator: GET /interactions/{id}. in_progress is pending, completed is done,
// and failed, cancelled and incomplete are failed, with a refusal when the provider names a safety block.
// requires_action and requires_continuation never end a generation of one request and are an unreadable answer.
func (*Adapter) Poll(ctx context.Context, c connections.Credential, taskID string) (providers.VideoStatus, error) {
	path, err := taskPath(taskID)
	if err != nil {
		return providers.VideoStatus{}, err
	}

	var ans interaction

	call := wire.Call{Method: http.MethodGet, Path: path, Header: headers(c), MaxAnswer: pollAnswer}
	if err := wire.JSON(ctx, c, call, &ans); err != nil {
		return providers.VideoStatus{}, err
	}

	switch ans.Status {
	case "in_progress":
		return providers.VideoStatus{State: providers.VideoPending}, nil
	case "completed":
		return providers.VideoStatus{State: providers.VideoDone}, nil
	case "failed", "cancelled", "incomplete":
		return providers.VideoStatus{State: providers.VideoFailed, Err: failure(ans.Status, ans.Errors)}, nil
	}

	return providers.VideoStatus{}, errUnreadable()
}

// refusalMarks are the words of an error code with which the provider says it blocked the prompt or the clip
// under its content policy.
var refusalMarks = []string{"safety", "blocked", "policy", "prohibited", "filter", "violat"}

func failure(status string, errs []apiError) error {
	for _, e := range errs {
		code := strings.ToLower(e.Code)

		if slices.ContainsFunc(refusalMarks, func(m string) bool { return strings.Contains(code, m) }) {
			return errRefused()
		}
	}

	if status == "cancelled" {
		return &providers.Error{Kind: providers.KindUnavailable, Status: http.StatusOK, Code: "cancelled"}
	}

	return &providers.Error{Kind: providers.KindUnavailable, Status: http.StatusOK}
}

// Fetch implements providers.VideoGenerator: GET /interactions/{id} of a completed interaction, whose
// model_output step holds the clip inline as base64 (the documentation says a later read of an interaction
// returns it inline whatever the delivery was). A finished interaction with no clip in it is a refusal of the
// output; one that names the clip by URI only is the same, because that URI is for the Files API and the key
// is sent to the API host only.
func (*Adapter) Fetch(
	ctx context.Context,
	c connections.Credential,
	taskID string,
	maxBytes int64,
) (providers.GeneratedFile, error) {
	path, err := taskPath(taskID)
	if err != nil {
		return providers.GeneratedFile{}, err
	}

	var ans interaction

	call := wire.Call{
		Method: http.MethodGet, Path: path, Header: headers(c), MaxAnswer: maxBytes/3*4 + 8 + answerSlack,
	}
	if err := wire.JSON(ctx, c, call, &ans); err != nil {
		return providers.GeneratedFile{}, err
	}

	if ans.Status != "completed" {
		return providers.GeneratedFile{}, errUnreadable()
	}

	for _, s := range ans.Steps {
		if s.Type != "model_output" {
			continue
		}

		for _, ct := range s.Content {
			if ct.Type != "video" || ct.Data == "" {
				continue
			}

			data, derr := base64.StdEncoding.DecodeString(ct.Data)
			if derr != nil || int64(len(data)) > maxBytes {
				return providers.GeneratedFile{}, errUnreadable()
			}

			return providers.GeneratedFile{Data: data}, nil
		}
	}

	return providers.GeneratedFile{}, errRefused()
}

// Cancel implements providers.VideoGenerator: POST /interactions/{id}/cancel. The documentation says the status
// becomes cancelled; it does not say that nothing is charged for what ran, so the answer is never a
// confirmation and the cost already incurred stays the estimate (SDD-016 section 6.4).
func (*Adapter) Cancel(ctx context.Context, c connections.Credential, taskID string) (bool, error) {
	path, err := taskPath(taskID)
	if err != nil {
		return false, err
	}

	var ans interaction

	call := wire.Call{Method: http.MethodPost, Path: path + "/cancel", Header: headers(c)}

	return false, wire.JSON(ctx, c, call, &ans)
}

func errUnreadable() error {
	return &providers.Error{Kind: providers.KindUnreadable, Status: http.StatusOK}
}

func errRefused() error { return &providers.Error{Kind: providers.KindRefused, Status: http.StatusOK} }
