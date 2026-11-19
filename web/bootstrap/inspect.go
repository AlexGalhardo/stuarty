package bootstrap

import (
	"net/http"
	"time"

	"github.com/AlexGalhardo/stuarty/service"
	"github.com/AlexGalhardo/stuarty/web/app/auth"
	httpapi "github.com/AlexGalhardo/stuarty/web/app/http"

	vukem "github.com/AlexGalhardo/vukem"
)

// noService stands in for the Service of an application that only lists its
// routes: nothing serves, so no method is ever called.
type noService struct{ service.Service }

// Inspect builds the application with every route of the owner installed over
// placeholder dependencies, for the commands that describe the application and
// never serve it (route:list). The routes are the ones serve registers.
func Inspect(opts ...vukem.Option) (*vukem.App, error) {
	return NewWith(&httpapi.Deps{
		Service: noService{}, Token: "inspect", Sessions: auth.NewSessions(time.Now), Launch: auth.NewLaunch(time.Now),
		Now: time.Now, MCP: http.NotFoundHandler(),
	}, opts...)
}
