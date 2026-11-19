package shell

import (
	"os"
	"strconv"
	"strings"

	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// portFile remembers the loopback port of the last run. The browser engine
// keeps localStorage (theme, first-run tips) per origin and an origin includes
// the port, so a new random port on every launch would lose those preferences.
const portFile = "desktop-port"

// rememberedPort reads the port of the last run; 0 when there is none or it is
// not a valid unprivileged port.
func rememberedPort(ws workspace.Dir) int {
	raw, err := os.ReadFile(ws.Path(portFile))
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || port < 1024 || port > 65535 {
		return 0
	}
	return port
}

func rememberPort(ws workspace.Dir, port int) {
	// A failed write only costs the next launch a new origin.
	_ = os.WriteFile(ws.Path(portFile), []byte(strconv.Itoa(port)+"\n"), 0o600)
}

// pickStablePort prefers the remembered port while it is free and falls back
// to any free port.
func pickStablePort(ws workspace.Dir) (int, error) {
	if want := rememberedPort(ws); want != 0 {
		if port, err := pickPort(want); err == nil {
			return port, nil
		}
	}
	port, err := pickPort(0)
	if err == nil {
		rememberPort(ws, port)
	}
	return port, err
}
