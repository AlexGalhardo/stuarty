package cli

// Commands whose Service methods do not exist yet. Each is declared so that
// --help, the command list and the flag rules cover it, and each fails with a
// message that names the task that delivers it. Replace the stub with a
// handler when the method lands; the tests in pending_test.go mark what to
// enable.

func pendingSpec(name, summary, task string, usage ...string) *spec {
	return &spec{name: name, summary: summary, usage: usage, pending: task, project: true}
}

var (
	mediaImport = pendingSpec("media:import", "import media files into a project", "P1-SRV-004",
		"vukem-video media:import <path>... --project <id> [--asset-id ast_...]")
	mediaInspect = pendingSpec("media:inspect", "show what is known about an asset", "P1-SRV-004",
		"vukem-video media:inspect <asset id> --project <id> [--sections probe,transcript,silences,loudness]")
	mediaList = pendingSpec("media:list", "list the assets of a project", "P1-SRV-004",
		"vukem-video media:list --project <id> [--cursor c] [--limit n]")
	view = pendingSpec("view", "render a frame or a contact sheet of a project", "P1-RND-005",
		"vukem-video view --project <id> [--mode frame|sheet] [--at t]")
	export = pendingSpec("export", "export a project to a file", "P1-RND-004",
		"vukem-video export --project <id> [--start t] [--end t]")
	jobShow = pendingSpec("job:show", "show a job", "P1-SRV-004",
		"vukem-video job:show <job id> [--wait] [--timeout d]")
	jobList = pendingSpec("job:list", "list the jobs of a project", "P1-SRV-004",
		"vukem-video job:list --project <id> [--state s] [--cursor c] [--limit n]")
	events = pendingSpec("events", "print the events of a project as they arrive", "P1-SRV-006",
		"vukem-video events --project <id> [--from-id n] [--since n] [--count n]")
)
