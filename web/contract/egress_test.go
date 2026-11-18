package contract

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/AlexGalhardo/vukem-video/internal/outbound"
)

// TestSecurity_T10_DefaultRunOpensNoOutboundConnection (SDD-007 section 10.2): with every
// VUKEM_VIDEO_* variable unset, the prototype scenario (create, import, edit, lint, view, export,
// undo) makes no dial through any outbound purpose.
func TestSecurity_T10_DefaultRunOpensNoOutboundConnection(t *testing.T) {
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "VUKEM_VIDEO_") {
			t.Setenv(name, "") // restored when the test ends; empty is unset for the configuration
		}
	}

	var dials atomic.Int32

	restore := outbound.SetDialRecorder(func(p outbound.Purpose, address string) error {
		dials.Add(1)
		t.Errorf("outbound dial for %s to %s", p, address)

		return outbound.ErrNotLoopback
	})
	defer restore()

	sameThroughEverySurface(t, "prototype", "prj_7k2m9x4q1b")

	if n := dials.Load(); n != 0 {
		t.Fatalf("%d outbound dials in a default run, want 0", n)
	}
}

// TestSecurity_T10_EgressScanner (SDD-007 section 10.2 item 3): the verdict of the no-egress job on
// strace logs. A connect to a non-loopback address fails, an empty trace is void, and loopback and
// Unix sockets pass.
func TestSecurity_T10_EgressScanner(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Fatalf("bash is required to run the scanner: %v", err)
	}

	const (
		loopback4 = `1234 connect(7, {sa_family=AF_INET, sin_port=htons(8080), sin_addr=inet_addr("127.0.0.1")}, 16) = 0` + "\n"
		loopback6 = `1234 connect(8, {sa_family=AF_INET6, sin6_port=htons(8080), sin6_flowinfo=htonl(0), inet_pton(AF_INET6, "::1", &sin6_addr), sin6_scope_id=0}, 28) = 0` + "\n"
		unixSock  = `1234 connect(9, {sa_family=AF_UNIX, sun_path="/run/x.sock"}, 110) = 0` + "\n"
		public4   = `1235 connect(7, {sa_family=AF_INET, sin_port=htons(443), sin_addr=inet_addr("93.184.216.34")}, 16) = -1 EINPROGRESS` + "\n"
		public6   = `1235 sendto(7, "x", 1, 0, {sa_family=AF_INET6, sin6_port=htons(53), inet_pton(AF_INET6, "2001:4860:4860::8888", &sin6_addr)}, 28) = 1` + "\n"
		mapped    = `1236 connect(7, {sa_family=AF_INET6, inet_pton(AF_INET6, "::ffff:10.0.0.1", &sin6_addr)}, 28) = 0` + "\n"
	)

	cases := []struct {
		name  string
		logs  []string
		clean bool
	}{
		{"loopback only", []string{loopback4 + loopback6}, true},
		{"loopback and unix", []string{loopback4 + unixSock}, true},
		{"public IPv4 among loopback", []string{loopback4, loopback4 + public4}, false},
		{"public IPv6 in a child", []string{loopback4, public6}, false},
		{"mapped non-loopback IPv4", []string{loopback4 + mapped}, false},
		{"empty trace", []string{""}, false},
		{"unix only", []string{unixSock}, false},
	}

	script, err := filepath.Abs("../../scripts/no-egress-scan.sh")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			args := []string{script}

			for i, content := range tc.logs {
				p := filepath.Join(dir, fmt.Sprintf("t%d.log", i))
				if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}

				args = append(args, p)
			}

			out, err := exec.Command(bash, args...).CombinedOutput()
			if tc.clean && err != nil {
				t.Fatalf("scanner failed a clean trace: %v\n%s", err, out)
			}

			if !tc.clean && err == nil {
				t.Fatalf("scanner accepted a trace that must fail:\n%s", out)
			}
		})
	}
}
