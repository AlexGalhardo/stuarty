//go:build darwin

package shell

import "os/exec"

// watchdog runs the server under a watcher: macOS has no parent-death signal, so a background
// subshell polls the shell and the server, and ends the server (SIGTERM, which it handles like
// Stop) once the shell is gone. exec keeps the server on the pid of sh, which is the pid that
// runtime.json must show. The watcher leaves when the server exits first, so a reused pid is
// never signalled.
const watchdog = `s=$$; p=$PPID
( while kill -0 "$p" 2>/dev/null && kill -0 "$s" 2>/dev/null; do sleep 1; done
  kill -0 "$p" 2>/dev/null || kill -TERM "$s" 2>/dev/null ) >/dev/null 2>&1 </dev/null &
exec "$0" "$@"`

// prepareCommand wraps the server in the watchdog (VV-006).
func prepareCommand(cmd *exec.Cmd) {
	cmd.Path = "/bin/sh"
	cmd.Args = append([]string{"/bin/sh", "-c", watchdog}, cmd.Args...)
}
