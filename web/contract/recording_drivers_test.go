package contract

import (
	"encoding/json"
	"strconv"
	"strings"
)

// recordingCommand is the command line of a recording step (SDD-025); ok is false for any other step.
func recordingCommand(s step) (name string, argv []string, ok bool, err error) {
	var a struct {
		ID        string `json:"recording_id"`
		ProjectID string `json:"project_id"`
		FPS       int    `json:"fps"`
		Source    struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"source"`
	}

	switch s.Call {
	case "RecordingSources", "RecordingStart", "RecordingStop", "RecordingCancel", "RecordingGet", "RecordingList",
		"RecordingConfirm", "RecordingDismiss", "RecordingPlace", "RecordingPause", "RecordingResume", "RecordingEvents":
	default:
		return "", nil, false, nil
	}

	if len(s.Args) > 0 {
		if err := json.Unmarshal(s.Args, &a); err != nil {
			return "", nil, true, err
		}
	}

	switch s.Call {
	case "RecordingSources":
		return "recording:sources", nil, true, nil
	case "RecordingStart":
		if a.ID != "" {
			return "recording:start", []string{"--recording", a.ID}, true, nil
		}

		argv = []string{"--project", a.ProjectID, "--kind", a.Source.Kind, "--source", a.Source.ID}
		if a.Source.Name != "" {
			argv = append(argv, "--name", a.Source.Name)
		}

		if a.FPS != 0 {
			argv = append(argv, "--fps", strconv.Itoa(a.FPS))
		}

		return "recording:start", argv, true, nil
	case "RecordingList":
		return "recording:list", []string{"--project", a.ProjectID}, true, nil
	}

	names := map[string]string{
		"RecordingStop":    "recording:stop",
		"RecordingPause":   "recording:pause",
		"RecordingResume":  "recording:resume",
		"RecordingEvents":  "recording:events",
		"RecordingCancel":  "recording:cancel",
		"RecordingGet":     "recording:show",
		"RecordingConfirm": "recording:confirm",
		"RecordingDismiss": "recording:dismiss",
		"RecordingPlace":   "recording:place",
	}

	return names[s.Call], []string{a.ID}, true, nil
}

// recordingAgentCommand is the command line of a step of the agent flow of a recording (SDD-025 section 19); ok is
// false for any other step. The ranges of a proposal go in on standard input.
func recordingAgentCommand(s step) (name string, argv []string, stdin string, ok bool, err error) {
	var a struct {
		ID          string            `json:"recording_id"`
		ProposalID  string            `json:"proposal_id"`
		ProjectID   string            `json:"project_id"`
		Kind        string            `json:"kind"`
		AtMS        int               `json:"at_ms"`
		EndMS       int               `json:"end_ms"`
		Note        string            `json:"note"`
		Notes       string            `json:"notes"`
		Accept      []bool            `json:"accept"`
		Ranges      []json.RawMessage `json:"ranges"`
		FPS         int               `json:"fps"`
		Microphone  string            `json:"microphone"`
		Events      bool              `json:"events"`
		SourceBlock struct {
			Kind string `json:"kind"`
			ID   string `json:"id"`
		} `json:"source"`
	}

	switch s.Call {
	case "RecordingPrepare", "RecordingMark", "RecordingProposeCuts", "RecordingProposalDecide":
	default:
		return "", nil, "", false, nil
	}

	if len(s.Args) > 0 {
		if err := json.Unmarshal(s.Args, &a); err != nil {
			return "", nil, "", true, err
		}
	}

	switch s.Call {
	case "RecordingPrepare":
		argv = []string{"--project", a.ProjectID, "--kind", a.SourceBlock.Kind, "--source", a.SourceBlock.ID}

		if a.FPS != 0 {
			argv = append(argv, "--fps", strconv.Itoa(a.FPS))
		}

		if a.Microphone != "" {
			argv = append(argv, "--microphone", a.Microphone)
		}

		if a.Events {
			argv = append(argv, "--events")
		}

		if a.Notes != "" {
			argv = append(argv, "--notes", a.Notes)
		}

		return "recording:prepare", argv, "", true, nil
	case "RecordingMark":
		argv = []string{a.ID, "--kind", a.Kind, "--at-ms", strconv.Itoa(a.AtMS)}

		if a.EndMS != 0 {
			argv = append(argv, "--end-ms", strconv.Itoa(a.EndMS))
		}

		if a.Note != "" {
			argv = append(argv, "--note", a.Note)
		}

		return "recording:mark", argv, "", true, nil
	case "RecordingProposeCuts":
		list, merr := json.Marshal(a.Ranges)
		if merr != nil {
			return "", nil, "", true, merr
		}

		return "recording:propose", []string{a.ID, "--ranges", "-"}, string(list), true, nil
	}

	words := make([]string, len(a.Accept))
	for i, yes := range a.Accept {
		words[i] = "no"
		if yes {
			words[i] = "yes"
		}
	}

	return "recording:decide", []string{a.ID, a.ProposalID, "--accept", strings.Join(words, ",")}, "", true, nil
}
