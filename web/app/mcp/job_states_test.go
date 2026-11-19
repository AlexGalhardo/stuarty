package mcp

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// A tool that answers a job declares every state the job can be in, so a client that checks the
// structured result against the outputSchema accepts a job that waits for a person (SDD-015
// section 5): an agent reads the state of its own request with job_get.
func TestJobToolsDeclareEveryJobState(t *testing.T) {
	t.Parallel()

	states := []string{
		service.JobQueued, service.JobWaitingApproval, service.JobRunning,
		service.JobSucceeded, service.JobFailed, service.JobCancelled,
	}

	for _, name := range []string{"job_start", "job_get"} {
		raw, err := definitionFiles.ReadFile("definitions/" + name + ".json")
		if err != nil {
			t.Fatal(err)
		}

		var def struct {
			OutputSchema struct {
				Properties struct {
					State struct {
						Enum []string `json:"enum"`
					} `json:"state"`
				} `json:"properties"`
			} `json:"outputSchema"`
		}

		if err := json.Unmarshal(raw, &def); err != nil {
			t.Fatal(err)
		}

		for _, s := range states {
			if !slices.Contains(def.OutputSchema.Properties.State.Enum, s) {
				t.Errorf("the outputSchema of %s does not allow the job state %q", name, s)
			}
		}
	}
}
