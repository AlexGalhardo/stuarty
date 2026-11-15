package cli

// The events command of SDD-003 section 5.4: the server-sent events of a
// project (SDD-006) as NDJSON. The events live in the hub of the process that
// owns the workspace, so the command needs a server; it is not a Service method
// (SDD-003 section 7.1, exception E-1).

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

// EventsQuery is what the stream of a project is opened with.
type EventsQuery struct {
	ProjectID   string
	LastEventID string // the Last-Event-ID header; wins over Since
	Since       string // a revision; empty when not given
}

// EventOpener opens the stream and returns its body.
type EventOpener func(ctx context.Context, q EventsQuery) (io.ReadCloser, error)

const (
	sseMaxLine     = 1 << 20
	reconnectAfter = time.Second
)

var eventsCmd = &spec{
	name:    "events",
	summary: "print the events of a project as they arrive",
	usage:   []string{"vukem-video events --project <id> [--from-id n] [--since n] [--count n]"},
	example: "vukem-video events --project prj_7k2m9x4q1b --since 4 --count 3",
	project: true,
	stream:  true,
	flags: []flagDef{
		{Name: "from-id", Kind: kindString, Usage: "resume after this event id (the Last-Event-ID)"},
		{Name: "since", Kind: kindInt, Usage: "replay the revisions after this one"},
		{Name: "count", Kind: kindInt, Default: "0", Usage: "stop after n events (0: until interrupted)"},
	},
	run: runEvents,
}

// eventLine is one printed event.
type eventLine struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Data json.RawMessage `json:"data"`
}

func runEvents(ctx context.Context, c *call) (*output, error) {
	if c.events == nil {
		return nil, &service.Error{
			Code: command.CodeInvalidState, Message: "no server answers for this workspace, so there are no events.",
			Hint: "run vukem-video serve, then repeat the command",
		}
	}

	q := EventsQuery{ProjectID: c.projectID, LastEventID: c.args.str("from-id")}
	if c.args.has("since") {
		q.Since = c.args.str("since")
	}

	limit := c.args.integer("count")
	seen := 0

	for ctx.Err() == nil {
		body, err := c.events(ctx, q)
		if err != nil {
			if ctx.Err() == nil {
				return nil, err
			}

			break // interrupted: a normal end
		}

		last, n, done := c.rt.copyEvents(ctx, body, limit-seen, limit > 0)
		_ = body.Close()

		seen += n

		if last != "" {
			q.LastEventID = last
		}

		if done {
			break
		}

		// The server closed the stream: resume after the last event, as SSE says.
		select {
		case <-ctx.Done():
		case <-time.After(reconnectAfter):
		}
	}

	return nil, nil //nolint:nilnil // a stream command prints its own output
}

// copyEvents prints the events of one stream and returns the id of the last
// one, how many it printed and whether the count was reached.
func (rt *Runtime) copyEvents(ctx context.Context, r io.Reader, left int, counted bool) (string, int, bool) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), sseMaxLine)

	var (
		cur  eventLine
		data []string
		last string
		n    int
	)

	for sc.Scan() && ctx.Err() == nil {
		line := sc.Text()

		if line != "" {
			// A comment (the heartbeat) starts with a colon.
			if k, v, ok := strings.Cut(line, ":"); ok && k != "" {
				v = strings.TrimPrefix(v, " ")

				switch k {
				case "id":
					cur.ID = v
				case "event":
					cur.Name = v
				case "data":
					data = append(data, v)
				}
			}

			continue
		}

		// A block with no event name is the preamble or a heartbeat.
		if cur.Name != "" {
			cur.Data = json.RawMessage(strings.Join(data, "\n"))
			if !json.Valid(cur.Data) {
				cur.Data = json.RawMessage("null")
			}

			if raw, err := json.Marshal(cur); err == nil {
				fmt.Fprintf(rt.Stdout, "%s\n", raw)
			}

			n++

			if cur.ID != "" {
				last = cur.ID
			}

			if counted && n >= left {
				return last, n, true
			}
		}

		cur, data = eventLine{}, nil
	}

	return last, n, false
}

// doer is the part of an HTTP client the stream needs; the client itself is
// built by internal/outbound.
type doer interface {
	Do(req *http.Request) (*http.Response, error)
}

// httpEvents is the EventOpener over the HTTP API of a server.
func httpEvents(base, token string, hc doer) EventOpener {
	return func(ctx context.Context, q EventsQuery) (io.ReadCloser, error) {
		u := strings.TrimRight(base, "/") + "/api/v1/projects/" + url.PathEscape(q.ProjectID) + "/events"
		if q.Since != "" {
			u += "?" + url.Values{"since": {q.Since}}.Encode()
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
		if err != nil {
			return nil, fmt.Errorf("build the request: %w", err)
		}

		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Authorization", "Bearer "+token)

		if q.LastEventID != "" {
			req.Header.Set("Last-Event-ID", q.LastEventID)
		}

		resp, err := hc.Do(req)
		if err != nil {
			return nil, &service.Error{
				Code: service.CodeWorkspaceLocked, Retryable: true, Message: "the server did not answer.",
				Hint: "the owner of the workspace is not answering; retry in a few seconds",
			}
		}

		if resp.StatusCode != http.StatusOK {
			defer resp.Body.Close()

			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))

			var doc service.ErrorDocument
			if json.Unmarshal(raw, &doc) != nil || doc.Code == "" {
				return nil, &service.Error{
					Code:    command.CodeInternal,
					Message: fmt.Sprintf("the server answered %d without an error envelope.", resp.StatusCode),
				}
			}

			return nil, &service.Error{
				Code: doc.Code, Message: doc.Message, Hint: doc.Hint, Retryable: doc.Retryable, Path: doc.Path,
			}
		}

		return resp.Body, nil
	}
}
