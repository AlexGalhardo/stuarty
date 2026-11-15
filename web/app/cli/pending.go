package cli

// Commands whose Service methods do not exist yet. Each is declared so that
// --help, the command list and the flag rules cover it, and each fails with a
// message that names the task that delivers it. Replace the stub with a
// handler when the method lands.

func pendingSpec(name, summary, task string, usage ...string) *spec {
	return &spec{name: name, summary: summary, usage: usage, pending: task, project: true}
}

var events = pendingSpec("events", "print the events of a project as they arrive", "P1-SRV-006",
	"vukem-video events --project <id> [--from-id n] [--since n] [--count n]")
