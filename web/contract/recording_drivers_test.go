package contract

import (
	"encoding/json"
	"strconv"
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
		"RecordingConfirm", "RecordingDismiss", "RecordingPlace":
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
		"RecordingCancel":  "recording:cancel",
		"RecordingGet":     "recording:show",
		"RecordingConfirm": "recording:confirm",
		"RecordingDismiss": "recording:dismiss",
		"RecordingPlace":   "recording:place",
	}

	return names[s.Call], []string{a.ID}, true, nil
}
