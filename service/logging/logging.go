// Package logging is the structured log of the product (ADR-0026 item 2): one
// line per event on standard error and in a rotating file under the workspace
// `logs/` folder. A line carries the request or job identifier, the actor and
// the project. Text that came from media (file names, transcripts) is logged
// only at debug level, and then with the control and bidirectional characters
// replaced (SDD-007 section 11 rule 1, T-25). The package depends on the
// standard library only: the service must not import vukem.
package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// File is the name of the current log file inside the logs folder.
const File = "stuarty.log"

// Defaults of the rotating file: it is rotated at 10 MiB and the 3 newest old
// files are kept as stuarty.log.1 (the newest) to stuarty.log.3.
const (
	DefaultMaxBytes int64 = 10 << 20
	DefaultKeep           = 3
)

// Options of New.
type Options struct {
	// Stderr receives every line; nil writes nothing there.
	Stderr io.Writer
	// Dir is the logs folder; empty means no file.
	Dir string
	// MaxBytes is the size at which the file is rotated; 0 is DefaultMaxBytes.
	MaxBytes int64
	// Keep is the number of old files kept; 0 is DefaultKeep.
	Keep int
	// Level is the minimum level; the zero value is info.
	Level slog.Level
}

// New returns the logger and the closer of its file. Every string of a line is
// sanitized, so a name that holds a line feed or an escape sequence cannot forge
// another line.
func New(o Options) (*slog.Logger, io.Closer, error) {
	var writers []io.Writer

	var closer io.Closer = nopCloser{}

	if o.Stderr != nil {
		writers = append(writers, o.Stderr)
	}

	if o.Dir != "" {
		f, err := openRotating(o.Dir, o.MaxBytes, o.Keep)
		if err != nil {
			return nil, nil, err
		}

		writers = append(writers, f)
		closer = f
	}

	if len(writers) == 0 {
		return slog.New(slog.DiscardHandler), closer, nil
	}

	inner := slog.NewTextHandler(io.MultiWriter(writers...), &slog.HandlerOptions{Level: o.Level})

	return slog.New(&sanitizing{next: inner}), closer, nil
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

// sanitizing cleans the message and every attribute before the handler writes
// them.
type sanitizing struct{ next slog.Handler }

func (h *sanitizing) Enabled(ctx context.Context, l slog.Level) bool { return h.next.Enabled(ctx, l) }

func (h *sanitizing) Handle(ctx context.Context, r slog.Record) error {
	clean := slog.NewRecord(r.Time, r.Level, Sanitize(r.Message), r.PC)

	r.Attrs(func(a slog.Attr) bool {
		clean.AddAttrs(sanitizeAttr(a))

		return true
	})

	return h.next.Handle(ctx, clean)
}

func (h *sanitizing) WithAttrs(attrs []slog.Attr) slog.Handler {
	clean := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		clean[i] = sanitizeAttr(a)
	}

	return &sanitizing{next: h.next.WithAttrs(clean)}
}

func (h *sanitizing) WithGroup(name string) slog.Handler {
	return &sanitizing{next: h.next.WithGroup(Sanitize(name))}
}

// Who are the attributes every line of a request or a job carries: the request
// or job identifier, the actor and the project. Empty values are left out.
func Who(requestID, jobID, actor, project string) []slog.Attr {
	var attrs []slog.Attr

	for _, kv := range [...][2]string{
		{"request_id", requestID}, {"job_id", jobID}, {"actor", actor}, {"project", project},
	} {
		if kv[1] != "" {
			attrs = append(attrs, slog.String(kv[0], kv[1]))
		}
	}

	return attrs
}

// Media logs text that came from media (a file name, a transcript) at debug
// level only: at any other level the logger drops it, and the text never
// reaches a handler. The text is sanitized by the logger itself.
func Media(ctx context.Context, log *slog.Logger, msg string, attrs ...slog.Attr) {
	if !log.Enabled(ctx, slog.LevelDebug) {
		return
	}

	log.LogAttrs(ctx, slog.LevelDebug, msg, attrs...)
}

// rotating is a file that is rotated when a write would take it past max bytes:
// stuarty.log becomes stuarty.log.1, that one .2, and so on up to keep
// old files; the oldest is removed.
type rotating struct {
	mu   sync.Mutex
	dir  string
	max  int64
	keep int
	f    *os.File
	size int64
}

func openRotating(dir string, maxBytes int64, keep int) (*rotating, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}

	if keep <= 0 {
		keep = DefaultKeep
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("logging: %w", err)
	}

	r := &rotating{dir: dir, max: maxBytes, keep: keep}
	if err := r.open(); err != nil {
		return nil, err
	}

	return r, nil
}

func (r *rotating) path(n int) string {
	if n == 0 {
		return filepath.Join(r.dir, File)
	}

	return filepath.Join(r.dir, fmt.Sprintf("%s.%d", File, n))
}

func (r *rotating) open() error {
	f, err := os.OpenFile(r.path(0), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("logging: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		_ = f.Close()

		return fmt.Errorf("logging: %w", err)
	}

	r.f, r.size = f, info.Size()

	return nil
}

// rotate closes the file (Windows cannot rename an open one), shifts the old
// files and opens a new one.
func (r *rotating) rotate() error {
	if err := r.f.Close(); err != nil {
		return fmt.Errorf("logging: %w", err)
	}

	_ = os.Remove(r.path(r.keep))

	for n := r.keep - 1; n >= 0; n-- {
		if err := os.Rename(r.path(n), r.path(n+1)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("logging: %w", err)
		}
	}

	return r.open()
}

func (r *rotating) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.size > 0 && r.size+int64(len(p)) > r.max {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}

	n, err := r.f.Write(p)
	r.size += int64(n)

	return n, err
}

func (r *rotating) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	return r.f.Close()
}
