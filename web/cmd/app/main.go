// Command app is the vukem-video binary: the HTTP server by default, or a
// command such as migrate, route:list or about (vukem <command> delegates here).
package main

import (
	"os"

	vukem "github.com/AlexGalhardo/vukem"

	"github.com/AlexGalhardo/vukem-video/web/bootstrap"
)

func main() {
	// open runs before the application is built: see opener.
	if len(os.Args) > 1 && os.Args[1] == "open" {
		os.Exit(runOpen(os.Args[2:], os.Stdout, os.Stderr))
	}
	os.Exit(vukem.Main(bootstrap.New))
}
