package shell

import (
	"net"
	"strconv"
	"testing"

	"github.com/AlexGalhardo/vukem-video/service/workspace"
)

func TestPickStablePortReusesTheLastPort(t *testing.T) {
	ws := workspace.Dir(t.TempDir())
	first, err := pickStablePort(ws)
	if err != nil {
		t.Fatal(err)
	}
	second, err := pickStablePort(ws)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("port changed between runs: %d then %d", first, second)
	}
}

func TestPickStablePortFallsBackWhenTaken(t *testing.T) {
	ws := workspace.Dir(t.TempDir())
	first, err := pickStablePort(ws)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(first)))
	if err != nil {
		t.Skip("port was taken meanwhile")
	}
	defer func() { _ = ln.Close() }()
	other, err := pickStablePort(ws)
	if err != nil {
		t.Fatal(err)
	}
	if other == first {
		t.Fatal("a taken port was returned")
	}
}

func TestRememberedPortIgnoresGarbage(t *testing.T) {
	ws := workspace.Dir(t.TempDir())
	rememberPort(ws, 80)
	if got := rememberedPort(ws); got != 0 {
		t.Fatalf("got %d, want 0 for a privileged port", got)
	}
}
