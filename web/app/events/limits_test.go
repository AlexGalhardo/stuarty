package events

import (
	"errors"
	"testing"

	"github.com/AlexGalhardo/stuarty/service"
)

// SDD-006 section 9.4: a key has at most 16 streams across all its projects;
// the 17th is E_QUEUE_FULL with details.limit = "streams", value and ceiling, and a
// person (no key) is not counted per key.
func TestStreamsPerKeyAreLimited(t *testing.T) {
	t.Parallel()

	e := New(nil, Options{MaxPerProject: 100, MaxPerProcess: 100})

	for i := range MaxPerKey {
		if err := e.acquire("prj_"+string(rune('a'+i)), "key:k1"); err != nil {
			t.Fatalf("stream %d: %v", i+1, err)
		}
	}

	se, ok := errors.AsType[*service.Error](e.acquire("prj_z", "key:k1"))
	if !ok || se.Code != service.CodeQueueFull || se.Details["limit"] != "streams" ||
		se.Details["value"] != MaxPerKey || se.Details["ceiling"] != MaxPerKey {
		t.Fatalf("the 17th stream of a key = %+v", se)
	}

	if err := e.acquire("prj_z", "key:k2"); err != nil {
		t.Errorf("another key is limited by the first: %v", err)
	}

	if err := e.acquire("prj_z", ""); err != nil {
		t.Errorf("a person is limited per key: %v", err)
	}

	e.release("prj_a", "key:k1")

	if err := e.acquire("prj_z", "key:k1"); err != nil {
		t.Errorf("a released stream did not free its place: %v", err)
	}
}

// SDD-007 section 9.1: 32 streams for one project and 128 for the process; the next one is
// E_QUEUE_FULL naming the limit, the streams open and the ceiling.
func TestStreamsPerProjectAndPerProcessAreLimited(t *testing.T) {
	e := New(nil, Options{})

	for i := range MaxPerProject {
		if err := e.acquire("prj_a", ""); err != nil {
			t.Fatalf("stream %d of a project: %v", i+1, err)
		}
	}

	se, ok := errors.AsType[*service.Error](e.acquire("prj_a", ""))
	if !ok || se.Code != service.CodeQueueFull || se.Details["limit"] != "streams" ||
		se.Details["value"] != MaxPerProject || se.Details["ceiling"] != MaxPerProject {
		t.Fatalf("the 33rd stream of a project = %+v", se)
	}

	for i := MaxPerProject; i < MaxPerProcess; i++ {
		if err := e.acquire("prj_"+string(rune('a'+i/MaxPerProject)), ""); err != nil {
			t.Fatalf("stream %d of the process: %v", i+1, err)
		}
	}

	se, ok = errors.AsType[*service.Error](e.acquire("prj_z", ""))
	if !ok || se.Details["value"] != MaxPerProcess || se.Details["ceiling"] != MaxPerProcess {
		t.Fatalf("the 129th stream of the process = %+v", se)
	}
}
