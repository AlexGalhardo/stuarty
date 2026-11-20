package servicetest

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/service"
)

// SDD-005 section 8 step 3 and SDD-001 section 11 rule 5: a stored document with a schema_version
// higher than this build supports is refused by every load with V_SCHEMA_VERSION, both numbers and
// the instruction to upgrade, never E_INTERNAL. Nothing is written.
func TestNewerDocumentIsRefusedByEveryLoad(t *testing.T) {
	e := newEnv(t)
	project, _ := e.withTrack()
	newer := timeline.CurrentSchemaVersion + 1

	_, err := e.store.DB().ExecContext(
		e.as(ui),
		"UPDATE timelines SET document = replace(document, ?, ?) WHERE id = ?",
		fmt.Sprintf(`"schema_version":%d`, timeline.CurrentSchemaVersion),
		fmt.Sprintf(`"schema_version":%d`, newer),
		project,
	)
	if err != nil {
		t.Fatal(err)
	}

	revision := e.operationCount(project)

	loads := map[string]func() error{
		"get": func() error {
			_, err := e.svc.ProjectGet(e.as(ui), service.ProjectGetRequest{ProjectID: project})

			return err
		},
		"edit": func() error {
			_, err := e.edit(ui, project, 0, op("track.add", `{"kind":"visual","name":"x"}`))

			return err
		},
		"plan": func() error {
			_, err := e.svc.PlanWindow(e.as(ui), service.PlanWindowRequest{ProjectID: project, From: 0, Until: 10})

			return err
		},
	}

	for name, load := range loads {
		t.Run(name, func(t *testing.T) {
			se, ok := errors.AsType[*service.Error](load())
			if !ok {
				t.Fatal("want a *service.Error")
			}

			if se.Code == command.CodeInternal || se.Retryable {
				t.Fatalf("code = %s retryable = %v, want a refusal that is not E_INTERNAL", se.Code, se.Retryable)
			}

			if se.Details["code"] != "V_SCHEMA_VERSION" {
				t.Errorf("details = %v, want code V_SCHEMA_VERSION", se.Details)
			}

			for _, want := range []string{strconv.Itoa(newer), strconv.Itoa(timeline.CurrentSchemaVersion)} {
				if !strings.Contains(se.Message, want) {
					t.Errorf("message = %q, want the version %s", se.Message, want)
				}
			}

			if !strings.Contains(se.Hint, "update") && !strings.Contains(se.Hint, "upgrade") {
				t.Errorf("hint = %q, want the instruction to upgrade", se.Hint)
			}
		})
	}

	if got := e.operationCount(project); got != revision {
		t.Errorf("operations = %d, want %d: nothing is written", got, revision)
	}
}
