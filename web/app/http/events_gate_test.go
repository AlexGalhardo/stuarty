package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

// TestEventsTwoStreamsGetEveryRevisionOnceInOrderUnderUndo is SDD-006 section 11 check 4: 200
// batches from 8 goroutines, every fourth of them an Undo, and two streams that each receive
// revisions head+1..head+200 once and in order.
func TestEventsTwoStreamsGetEveryRevisionOnceInOrderUnderUndo(t *testing.T) {
	t.Parallel()

	h := newHarness(t)
	id := h.newProject("gate")
	head := h.commits(id, 0, 1)
	path := "/api/v1/projects/" + id + "/events?since=" + strconv.FormatInt(head, 10)
	a, b := h.stream(path, nil), h.stream(path, nil)

	const goroutines, each = 8, 25

	var wg sync.WaitGroup

	for g := range goroutines {
		wg.Go(func() {
			ctx := service.WithCaller(context.Background(), service.Caller{
				Actor: "local:writer" + strconv.Itoa(g), Scopes: service.AllScopes,
			})

			for i := range each {
				for {
					var err error

					if i%4 == 3 {
						_, err = h.svc.Undo(ctx, service.UndoRequest{ProjectID: id})
					} else {
						var cur *service.ProjectGetResult

						if cur, err = h.svc.ProjectGet(ctx, service.ProjectGetRequest{ProjectID: id}); err == nil {
							_, err = h.svc.Edit(ctx, service.EditRequest{
								ProjectID: id, BaseRevision: cur.Revision,
								Operations: []command.Operation{{Op: "text.add", Args: json.RawMessage(fmt.Sprintf(
									`{"track_id":%q,"start":"%ds","duration":"1s","text":"w%d"}`,
									h.track,
									g*each+i,
									g,
								))}},
							})
						}
					}

					if err == nil {
						break
					}

					if se, ok := errors.AsType[*service.Error](err); !ok || se.Code != command.CodeRevisionConflict {
						t.Error(err)

						return
					}
				}
			}
		})
	}

	wg.Wait()

	for name, s := range map[string]*sseClient{"a": a, "b": b} {
		got := s.revisions(goroutines * each)
		for i, rev := range got {
			if want := head + int64(i) + 1; rev != want {
				t.Fatalf("stream %s: event %d is revision %d, want %d", name, i, rev, want)
			}
		}
	}
}
