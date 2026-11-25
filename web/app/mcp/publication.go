package mcp

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/service"
	pub "github.com/AlexGalhardo/stuarty/service/publish"
)

// publicationMembers are the members each action of the publication tool uses (SDD-020 section 29): the others
// are refused, and the required ones must be there. The tool has no action that approves, sends now, retries,
// changes or cancels: a person does those, and nothing here reaches a network (ADR-0032).
var publicationMembers = map[string]struct{ required, optional []string }{
	"create": {nil, []string{"project_id", "title", "text", "export", "thumbnail", "post", "targets"}},
	"schedule": {
		[]string{"id"}, []string{"target_id", "at", "zone", "late_window_minutes"},
	},
	"show": {[]string{"id"}, nil},
	"list": {nil, []string{"project_id", "state", "network", "from", "until", "zone", "limit", "offset"}},
}

func checkPublicationMembers(action string, raw json.RawMessage) error {
	var given map[string]json.RawMessage
	if err := json.Unmarshal(raw, &given); err != nil {
		return &service.Error{Code: command.CodeValidation, Message: "the arguments are not valid."}
	}

	want, known := publicationMembers[action]
	if !known {
		return &service.Error{
			Code:    command.CodeValidation,
			Path:    "action",
			Message: "action " + action + " is not a publication action.",
			Hint:    "use create, schedule, show or list; sending now, retrying, changing and cancelling are for a person",
		}
	}

	for name := range given {
		if name != "action" && !slices.Contains(want.required, name) && !slices.Contains(want.optional, name) {
			return &service.Error{
				Code: command.CodeValidation, Path: name, Message: name + " is not used by the action " + action + ".",
			}
		}
	}

	for _, name := range want.required {
		if _, ok := given[name]; !ok {
			return &service.Error{
				Code: command.CodeValidation, Path: name, Message: name + " is required by the action " + action + ".",
			}
		}
	}

	return nil
}

type publicationArgs struct {
	Action            string            `json:"action"`
	ID                string            `json:"id"`
	TargetID          string            `json:"target_id"`
	ProjectID         string            `json:"project_id"`
	Title             string            `json:"title"`
	Text              string            `json:"text"`
	Export            string            `json:"export"`
	Thumbnail         string            `json:"thumbnail"`
	Post              string            `json:"post"`
	Targets           []pub.TargetInput `json:"targets"`
	At                string            `json:"at"`
	Zone              string            `json:"zone"`
	LateWindowMinutes *int              `json:"late_window_minutes"`
	State             string            `json:"state"`
	Network           string            `json:"network"`
	From              string            `json:"from"`
	Until             string            `json:"until"`
	Limit             int               `json:"limit"`
	Offset            int               `json:"offset"`
}

// publicationTool plans and reads the calendar of publications (SDD-020 section 29). It is a thin adapter over
// the Service: the caller checks, the scopes and the request for approval of a scheduled target are the
// Service's, the same as on HTTP and in the CLI.
func publicationTool(ctx context.Context, svc service.Service, raw json.RawMessage) (*toolOutput, error) {
	in, err := decode[publicationArgs](raw)
	if err != nil {
		return nil, err
	}

	if err := checkPublicationMembers(in.Action, raw); err != nil {
		return nil, err
	}

	var res *service.Publication

	switch in.Action {
	case "create":
		res, err = svc.PublicationCreate(ctx, service.PublicationCreateRequest{
			ProjectID: in.ProjectID, Title: in.Title, Text: in.Text, Export: in.Export, Thumbnail: in.Thumbnail,
			Post: in.Post, Targets: in.Targets,
		})
	case "schedule":
		res, err = svc.PublicationSchedule(ctx, service.PublicationScheduleRequest{
			ID: in.ID, TargetID: in.TargetID, At: in.At, Zone: in.Zone, LateWindowMinutes: in.LateWindowMinutes,
		})
	case "show":
		res, err = svc.PublicationGet(ctx, service.PublicationGetRequest{ID: in.ID})
	default:
		list, lerr := svc.PublicationList(ctx, service.PublicationListRequest{
			ProjectID: in.ProjectID, State: in.State, Network: in.Network, From: in.From, To: in.Until, Zone: in.Zone,
			Limit: in.Limit, Offset: in.Offset,
		})
		if lerr != nil {
			return nil, lerr
		}

		return &toolOutput{value: list, text: service.PublicationListText(list)}, nil
	}

	if err != nil {
		return nil, err
	}

	return &toolOutput{value: res, text: service.PublicationText(res)}, nil
}
