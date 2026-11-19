package config

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// fakeInfo is a file the Stat probe reports.
type fakeInfo struct {
	fs.FileInfo
	mode fs.FileMode
}

func (f fakeInfo) Mode() fs.FileMode { return f.mode }
func (f fakeInfo) IsDir() bool       { return f.mode.IsDir() }

type machine struct {
	files      map[string]fs.FileMode
	privileged bool
	token      error
	sandbox    error
}

func (m *machine) probes() Probes {
	return Probes{
		Privileged: func() bool { return m.privileged },
		Stat: func(name string) (fs.FileInfo, error) {
			mode, ok := m.files[filepath.Clean(name)]
			if !ok {
				return nil, fs.ErrNotExist
			}

			return fakeInfo{mode: mode}, nil
		},
		TokenFile: func() error { return m.token },
		Sandbox:   func() error { return m.sandbox },
	}
}

func abs(elem ...string) string {
	root := string(filepath.Separator)
	if vol := filepath.VolumeName(os.TempDir()); vol != "" {
		root = vol + root
	}

	return filepath.Join(append([]string{root}, elem...)...)
}

// goodKey is 32 bytes of base64.
var goodKey = "base64:" + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("k", 32)))

// network is a configuration that passes in network mode.
func network() map[string]string {
	return map[string]string{
		EnvMode: "network", EnvAddr: "0.0.0.0:7878", "APP_URL": "https://video.example.com",
		"TLS_CERT_FILE": "/etc/c.pem", "TLS_KEY_FILE": "/etc/k.pem", "APP_KEY": goodKey,
	}
}

func override(m map[string]string, k, v string) map[string]string {
	m[k] = v

	return m
}

func run(vars map[string]string, m *machine) (Resolved, []Problem) {
	if m == nil {
		m = &machine{}
	}

	return Validate(Env{
		Getenv:    func(k string) string { return vars[k] },
		Workspace: abs("data", "ws"),
		Probes:    m.probes(),
	})
}

func TestValidateAcceptsAGoodConfiguration(t *testing.T) {
	if _, ps := run(network(), nil); len(ps) != 0 {
		t.Fatalf("network: %v", ps)
	}

	if res, ps := run(map[string]string{}, nil); len(ps) != 0 || res.Mode != ModeLocal || res.Addr != "127.0.0.1:7878" {
		t.Fatalf("local: %+v %v", res, ps)
	}
}

// SDD-007 section 4.5 and section 15 check 11: one case per row of the table;
// each refuses with exit status 1 and names the variable.
func TestSecurity_T34_UnsafeConfigurationRefusesBoot(t *testing.T) {
	cases := []struct {
		name     string
		row      Row
		variable string
		vars     map[string]string
		machine  *machine
	}{
		{
			"addr not loopback in local mode", RowAddrNotLoopback, EnvAddr,
			map[string]string{EnvAddr: "0.0.0.0:7878"},
			nil,
		},
		{"unknown mode", RowUnknownMode, EnvMode, map[string]string{EnvMode: "lan"}, nil},
		{"app url missing", RowAppURL, "APP_URL", override(network(), "APP_URL", ""), nil},
		{"app url not https", RowAppURL, "APP_URL", override(network(), "APP_URL", "http://video.example.com"), nil},
		{"no tls", RowNoTLS, "TLS_CERT_FILE", override(network(), "TLS_KEY_FILE", ""), nil},
		{"app key not base64", RowAppKey, "APP_KEY", override(network(), "APP_KEY", "not base64!"), nil},
		{"app key short", RowAppKey, "APP_KEY", override(network(), "APP_KEY", "base64:c2hvcnQ="), nil},
		{"root", RowRoot, EnvAllowRoot, network(), &machine{privileged: true}},
		{"import root relative", RowImportRoot, EnvImportRoots, map[string]string{EnvImportRoots: "media"}, nil},
		{
			"import root missing", RowImportRoot, EnvImportRoots,
			map[string]string{EnvImportRoots: abs("srv", "media")},
			nil,
		},
		{
			"import root is a filesystem root", RowImportRoot, EnvImportRoots,
			map[string]string{EnvImportRoots: abs()},
			&machine{files: map[string]fs.FileMode{abs(): fs.ModeDir}},
		},
		{
			"import root inside the workspace", RowImportRoot, EnvImportRoots,
			map[string]string{EnvImportRoots: abs("data", "ws", "in")},
			&machine{files: map[string]fs.FileMode{abs("data", "ws", "in"): fs.ModeDir}},
		},
		{
			"transcribe http not loopback", RowTranscribe, EnvTranscribeURL,
			map[string]string{EnvTranscribeURL: "http://stt.example.com/v1"},
			nil,
		},
		{
			"transcribe userinfo", RowTranscribe, EnvTranscribeURL,
			map[string]string{EnvTranscribeURL: "https://u:p@stt.example.com/v1"},
			nil,
		},
		{
			"transcribe key without url", RowTranscribe, EnvTranscribeKey,
			map[string]string{EnvTranscribeKey: "secret"},
			nil,
		},
		{
			"loose token file", RowTokenFile, "token",
			map[string]string{},
			&machine{token: workspace.ErrTokenPermissions},
		},
		{
			"bwrap unavailable", RowSandbox, EnvSandbox,
			map[string]string{EnvSandbox: "bwrap"},
			&machine{sandbox: errors.New("bwrap is not on the PATH")},
		},
		{
			"limit zero in network mode", RowInvalidLimit, "STUARTY_MAX_EXPORTS",
			override(network(), "STUARTY_MAX_EXPORTS", "0"), nil,
		},
		{
			"limit negative in network mode", RowInvalidLimit, "STUARTY_MAX_IMPORT_BYTES",
			override(network(), "STUARTY_MAX_IMPORT_BYTES", "-5"), nil,
		},
		{"ffmpeg relative", RowFFmpegPath, EnvFFmpeg, map[string]string{EnvFFmpeg: "ffmpeg"}, nil},
		{"ffprobe not a file", RowFFmpegPath, EnvFFprobe, map[string]string{EnvFFprobe: abs("bin", "ffprobe")}, nil},
		{
			"ffmpeg is a directory", RowFFmpegPath, EnvFFmpeg,
			map[string]string{EnvFFmpeg: abs("bin")},
			&machine{files: map[string]fs.FileMode{abs("bin"): fs.ModeDir}},
		},
	}

	rows := map[Row]bool{}

	for _, c := range cases {
		rows[c.row] = true

		t.Run(c.name, func(t *testing.T) {
			_, ps := run(c.vars, c.machine)

			var found *Problem

			for i := range ps {
				if ps[i].Row == c.row {
					found = &ps[i]
				}
			}

			if found == nil {
				t.Fatalf("no %s problem in %v", c.row, ps)
			}

			if !strings.Contains(found.Message, c.variable) || found.Variable != c.variable {
				t.Errorf("message %q does not name %s", found.Message, c.variable)
			}

			var out bytes.Buffer
			if code := Report(&out, ps); code != 1 || out.Len() == 0 {
				t.Errorf("exit status = %d, output %q", code, out.String())
			}
		})
	}

	if len(rows) != 12 {
		t.Errorf("the table covers %d rows of SDD-007 section 4.5, want 12", len(rows))
	}
}

func TestOtherCasesOfTheRowsPass(t *testing.T) {
	good := &machine{files: map[string]fs.FileMode{
		abs("srv", "media"): fs.ModeDir, abs("bin", "ffmpeg"): 0o755,
	}}

	vars := map[string]string{
		EnvImportRoots: abs("srv", "media"), EnvFFmpeg: abs("bin", "ffmpeg"),
		EnvTranscribeURL: "http://127.0.0.1:9000/v1", EnvSandbox: "bwrap",
	}

	if _, ps := run(vars, good); len(ps) != 0 {
		t.Errorf("problems: %v", ps)
	}

	// A proxy that carries the scheme replaces the TLS files.
	proxied := override(network(), "TLS_CERT_FILE", "")
	proxied["TRUSTED_PROXIES"] = "10.0.0.0/8, 192.168.1.1"
	proxied["TRUST_FORWARDED_PROTO"] = "true"

	res, ps := run(proxied, nil)
	if len(ps) != 0 || len(res.TrustedProxies) != 2 || !res.TrustForwardedProto {
		t.Errorf("proxied: %+v %v", res, ps)
	}

	// Root is allowed when the operator says so.
	allowed := override(network(), EnvAllowRoot, "true")
	if _, ps := run(allowed, &machine{privileged: true}); len(ps) != 0 {
		t.Errorf("allow root: %v", ps)
	}
}

// P4-SRV-003: TLS ended at a reverse proxy needs both halves: the proxies that may speak for the
// client and the trust in their forwarded scheme. Either one alone is the "network mode needs TLS"
// refusal, and the same configuration with the TLS files passes.
func TestReverseProxyNeedsBothTrustedProxiesAndForwardedProto(t *testing.T) {
	for name, vars := range map[string]map[string]string{
		"proxies without the forwarded scheme": {"TRUSTED_PROXIES": "10.0.0.0/8"},
		"the forwarded scheme without proxies": {"TRUST_FORWARDED_PROTO": "true"},
		"the forwarded scheme set to false":    {"TRUSTED_PROXIES": "10.0.0.0/8", "TRUST_FORWARDED_PROTO": "false"},
	} {
		cfg := override(network(), "TLS_CERT_FILE", "")
		cfg["TLS_KEY_FILE"] = ""

		maps.Copy(cfg, vars)

		_, ps := run(cfg, nil)

		found := false

		for _, p := range ps {
			found = found || p.Row == RowNoTLS
		}

		if !found {
			t.Errorf("%s: no %s problem in %v", name, RowNoTLS, ps)
		}
	}

	both := override(network(), "TLS_CERT_FILE", "")
	both["TLS_KEY_FILE"] = ""
	both["TRUSTED_PROXIES"], both["TRUST_FORWARDED_PROTO"] = "10.0.0.0/8", "true"

	if res, ps := run(both, nil); len(ps) != 0 || !res.TrustForwardedProto || len(res.TrustedProxies) != 1 {
		t.Errorf("proxied: %+v %v", res, ps)
	}
}

func TestLimitsInLocalModeMeanTheDefault(t *testing.T) {
	for _, raw := range []string{"0", "-3"} {
		use, err := CheckLimit("STUARTY_MAX_EXPORTS", raw, false)
		if err != nil || use {
			t.Errorf("local %q: use=%v err=%v", raw, use, err)
		}

		if _, err := CheckLimit("STUARTY_MAX_EXPORTS", raw, true); err == nil ||
			!strings.Contains(err.Error(), "invalid limit") ||
			!strings.Contains(err.Error(), "STUARTY_MAX_EXPORTS") {
			t.Errorf("network %q: %v", raw, err)
		}
	}

	if _, err := CheckLimit("STUARTY_MAX_EXPORTS", "many", false); err == nil {
		t.Error("a limit that is not a number was accepted")
	}

	if use, err := CheckLimit("STUARTY_MAX_EXPORTS", "3", true); !use || err != nil {
		t.Errorf("3: %v %v", use, err)
	}
}

func TestContainerOnOwnMachine(t *testing.T) {
	for url, want := range map[string]bool{
		"http://localhost:7878":      true,
		"http://127.0.0.1:7878":      true,
		"http://127.0.0.1:7878/":     true,
		"http://localhost":           false,
		"http://localhost:7878/x":    false,
		"http://192.168.1.5:7878":    false,
		"https://localhost:7878":     false,
		"http://user@localhost:7878": false,
		"http://localhost.:7878":     false,
		"http://[::1]:7878":          false,
		"http://localhost:7878?x=1":  false,
		"http://localhost:notaport":  false,
		"http://example.com:7878":    false,
	} {
		if got := LoopbackURL(url); got != want {
			t.Errorf("LoopbackURL(%q) = %v, want %v", url, got, want)
		}
	}

	vars := override(network(), "APP_URL", "http://localhost:7878")
	vars["TLS_CERT_FILE"], vars["TLS_KEY_FILE"] = "", ""

	res, ps := run(vars, nil)
	if len(ps) != 0 || !res.LoopbackHTTP || res.AppURL != "http://localhost:7878" {
		t.Errorf("container rule: %+v %v", res, ps)
	}
}

func TestAddrReadsTheProductVariableThenTheFrameworkOnes(t *testing.T) {
	get := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

	if h, p := Addr(get(map[string]string{EnvAddr: "0.0.0.0:9000", "HOST": "x"})); h != "0.0.0.0" || p != "9000" {
		t.Errorf("addr = %s:%s", h, p)
	}

	if h, p := Addr(get(map[string]string{"HOST": "::1", "PORT": "1"})); h != "::1" || p != "1" {
		t.Errorf("host/port = %s:%s", h, p)
	}

	if h, p := Addr(get(nil)); h != "127.0.0.1" || p != "7878" {
		t.Errorf("default = %s:%s", h, p)
	}

	for _, h := range []string{"", "0.0.0.0", "::", "192.168.1.5", "myhost"} {
		if IsLoopbackHost(h) {
			t.Errorf("%q is loopback", h)
		}
	}
}
