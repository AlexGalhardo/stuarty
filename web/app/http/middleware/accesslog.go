package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/AlexGalhardo/vukem/web"

	"github.com/AlexGalhardo/vukem-video/core/timeline"
	"github.com/AlexGalhardo/vukem-video/service"
)

// projectsPrefix starts the paths that name a project.
const projectsPrefix = "/api/v1/projects/"

// AccessLog adds the actor and the project to the request line that vukem's
// logger writes (ADR-0026 item 2). That line carries the method, the path
// without its query, the status and the request identifier, and never a header:
// the query of /auth/launch, the Authorization header and the cookies are not in
// it. The project is taken only when the path segment is a well-formed
// identifier, so a crafted path cannot put text in the log. Run it after the
// authentication, which is what knows the actor.
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var attrs []slog.Attr

		if c, ok := service.CallerFrom(r.Context()); ok {
			attrs = append(attrs, slog.String("actor", c.Actor))
		}

		if id, ok := projectOf(r.URL.Path); ok {
			attrs = append(attrs, slog.String("project", id))
		}

		web.AddLogAttrs(r.Context(), attrs...)
		next.ServeHTTP(w, r)
	})
}

func projectOf(path string) (string, bool) {
	rest, ok := strings.CutPrefix(path, projectsPrefix)
	if !ok {
		return "", false
	}

	id, _, _ := strings.Cut(rest, "/")

	return id, timeline.ValidID(timeline.PrefixProject, id)
}
