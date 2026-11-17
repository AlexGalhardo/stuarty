package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/service"
)

// strictDryRun refuses a JSON body whose dry_run is not a JSON boolean. The binder of the framework merges the
// body over the query string and coerces text, so "yes" or "true" as a string would otherwise pass as true
// where the schema says boolean (VV-210). The query alias ?dry_run=true is text by nature and is not touched.
func strictDryRun(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body == nil || r.Body == http.NoBody || r.Method == http.MethodGet ||
			!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			next.ServeHTTP(w, r)
			return
		}

		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, web.DefaultMaxBody))
		if err != nil {
			// The framework reports the oversized or broken body itself, from what it can still read.
			r.Body = io.NopCloser(io.MultiReader(bytes.NewReader(raw), errReader{err}))
			next.ServeHTTP(w, r)

			return
		}

		r.Body = io.NopCloser(bytes.NewReader(raw))

		var top map[string]json.RawMessage
		if json.Unmarshal(raw, &top) == nil {
			if v, ok := top["dry_run"]; ok && len(v) > 0 && v[0] == '"' {
				web.Fail(w, r, &service.Error{
					Code: command.CodeValidation, Path: "dry_run",
					Message: "dry_run must be true or false, not text.",
					Hint:    "send the JSON boolean true or false",
				})

				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }
