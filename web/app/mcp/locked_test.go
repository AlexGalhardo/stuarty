package mcp

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/stuarty/core/command"
	"github.com/AlexGalhardo/stuarty/core/timeline"
	"github.com/AlexGalhardo/stuarty/internal/testfs"
	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/service/workspace"
	"github.com/AlexGalhardo/stuarty/web/app/store"
)

type closedGate struct{}

func (closedGate) Check() error { return errors.New("locked") }

// TestSecurity_T107_EveryToolOfALockedStudioAnswersELocked: a tool is a service operation with a name, so a
// locked studio refuses it with E_LOCKED and the hint to unlock; only command_help, which reads nothing of the
// user's, answers (SDD-007 section 3.9 item 6).
func TestSecurity_T107_EveryToolOfALockedStudioAnswersELocked(t *testing.T) {
	t.Parallel()

	dir, err := workspace.Resolve(func(string) string { return filepath.Join(t.TempDir(), "ws") })
	if err != nil {
		t.Fatal(err)
	}

	if err := dir.EnsureLayout(); err != nil {
		t.Fatal(err)
	}

	st, err := store.Open(context.Background(), dir, store.Options{SchemaVersion: timeline.CurrentSchemaVersion})
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = st.Close()

		testfs.Released(t, dir.Path("db.sqlite"))
	})

	var n atomic.Int64

	svc, err := service.New(service.Options{
		Store: st, Registry: command.Default(), Gate: closedGate{},
		NewIDs: func(prefix string) string { return fmt.Sprintf("%s_%010d", prefix, n.Add(1)) },
	})
	if err != nil {
		t.Fatal(err)
	}

	cs := connect(t, svc, Options{}, "agent")

	const project = "prj_0000000001"

	locked := map[string]any{
		"project_list":   map[string]any{},
		"project_create": map[string]any{"name": "x"},
		"project_get":    map[string]any{"project_id": project},
		"project_trash":  map[string]any{"project_id": project},
		"history":        map[string]any{"project_id": project, "action": "list"},
		"job_get":        map[string]any{"job_id": "job_0000000001"},
		"job_cancel":     map[string]any{"job_id": "job_0000000001"},
		"lint":           map[string]any{"project_id": project},
		"media_inspect":  map[string]any{"project_id": project, "asset_id": "ast_0000000001"},
		"template":       map[string]any{"action": "list"},
		"connection":     map[string]any{},
		"library":        map[string]any{"action": "list", "kind": "memory"},
		"image":          map[string]any{"action": "list"},
		"project_item":   map[string]any{"action": "brief", "project_id": project},
	}

	for tool, args := range locked {
		res := call(t, cs, tool, args)
		if !res.IsError || !strings.Contains(textOf(t, res), "E_LOCKED") {
			t.Errorf("%s while locked: error=%v %s", tool, res.IsError, textOf(t, res))
		}
	}

	if res := call(t, cs, "command_help", map[string]any{}); res.IsError {
		t.Errorf("command_help while locked: %s", textOf(t, res))
	}
}
