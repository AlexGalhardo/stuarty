// Package config is the boot-time validation of the configuration (SDD-007
// section 4.5): the one function that decides whether the process may start,
// and the facts the adapters need from it (the mode, the address, the public
// origin). It reads environment variables through a function so a test needs no
// process environment, and asks the machine through probes a test replaces.
package config

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/internal/outbound"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// Modes of SDD-000 section 10.
const (
	ModeLocal   = workspace.ModeLocal
	ModeNetwork = workspace.ModeNetwork
)

// Variables read here (SDD-000 section 10; the last group is vukem's).
const (
	EnvMode           = "STUARTY_MODE"
	EnvAddr           = "STUARTY_ADDR"
	EnvImportRoots    = "STUARTY_IMPORT_ROOTS"
	EnvTranscribeURL  = "STUARTY_TRANSCRIBE_URL"
	EnvTranscribeKey  = "STUARTY_TRANSCRIBE_KEY"
	EnvSandbox        = "STUARTY_SANDBOX"
	EnvAllowRoot      = "STUARTY_INSECURE_ALLOW_ROOT"
	EnvFFmpeg         = "STUARTY_FFMPEG"
	EnvFFprobe        = "STUARTY_FFPROBE"
	envAppURL         = "APP_URL"
	envAppKey         = "APP_KEY"
	envTLSCert        = "TLS_CERT_FILE"
	envTLSKey         = "TLS_KEY_FILE"
	envTrustedProxies = "TRUSTED_PROXIES"
	envTrustProto     = "TRUST_FORWARDED_PROTO"

	defaultHost = "127.0.0.1"
	defaultPort = "7878"

	// minAppKeyBytes is the vukem rule for APP_KEY.
	minAppKeyBytes = 32
)

// Row names one line of the table of SDD-007 section 4.5. Tests and the log use
// it; the text for the operator is Problem.Message.
type Row string

// The twelve rows of SDD-007 section 4.5, in the order of the table.
const (
	RowAddrNotLoopback Row = "addr_not_loopback"
	RowUnknownMode     Row = "unknown_mode"
	RowAppURL          Row = "app_url"
	RowNoTLS           Row = "no_tls"
	RowAppKey          Row = "app_key"
	RowRoot            Row = "root"
	RowImportRoot      Row = "import_root"
	RowTranscribe      Row = "transcribe_endpoint"
	RowTokenFile       Row = "token_file"
	RowSandbox         Row = "sandbox"
	RowInvalidLimit    Row = "invalid_limit"
	RowFFmpegPath      Row = "ffmpeg_path"
)

// Problem is one reason the process must not start. The message names the
// variable and the fix.
type Problem struct {
	Row      Row
	Variable string
	Message  string
}

// ExitStatus is the exit status of a process that refuses to start (SDD-007
// section 4.5).
const ExitStatus = 1

// Probes are the questions the validation asks the machine. A nil field is the
// real one.
type Probes struct {
	// Privileged reports whether the process runs as root or Administrator.
	Privileged func() bool
	// Stat is os.Stat.
	Stat func(name string) (fs.FileInfo, error)
	// TokenFile checks the permissions of the token file of the workspace and
	// returns workspace.ErrTokenPermissions when it is too loose.
	TokenFile func() error
	// Sandbox reports whether bwrap is present and can create a namespace.
	Sandbox func() error
}

// Env is what Validate reads.
type Env struct {
	Getenv func(string) string
	// Workspace is the workspace directory ("" skips the checks that need it).
	Workspace string
	Probes    Probes
}

// Resolved is what the adapters take from a valid configuration.
type Resolved struct {
	Mode string
	// Addr is the address the server binds, "host:port".
	Addr string
	// AppURL is the public origin in network mode, without a trailing slash.
	AppURL string
	// LoopbackHTTP is the container-on-own-machine case (SDD-007 section
	// 4.3 item 4): plain HTTP, reachable only as localhost.
	LoopbackHTTP bool
	// TrustForwardedProto is TRUST_FORWARDED_PROTO.
	TrustForwardedProto bool
	// TrustedProxies is TRUSTED_PROXIES, parsed.
	TrustedProxies []netip.Prefix
}

// Network reports whether the configuration is network mode.
func (r Resolved) Network() bool { return r.Mode == ModeNetwork }

// ModeOf is the mode named by STUARTY_MODE; empty is local. The value is
// not checked here: Validate refuses an unknown one.
func ModeOf(getenv func(string) string) string {
	if m := strings.TrimSpace(getenv(EnvMode)); m != "" {
		return m
	}

	return ModeLocal
}

// Addr is the address the server binds: STUARTY_ADDR, else HOST and PORT
// (the variables vukem reads), else 127.0.0.1:7878.
func Addr(getenv func(string) string) (host, port string) {
	if a := getenv(EnvAddr); a != "" {
		h, p, err := net.SplitHostPort(a)
		if err != nil {
			// Not host:port: an unusable address, which is never loopback.
			return a, ""
		}

		return h, p
	}

	host, port = defaultHost, defaultPort

	if h := getenv("HOST"); h != "" {
		host = h
	}

	if p := getenv("PORT"); p != "" {
		port = p
	}

	return host, port
}

// IsLoopbackHost tells a bind host or a URL host that only this machine can
// reach: localhost or a loopback IP. An empty host binds every interface.
func IsLoopbackHost(host string) bool {
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip, err := netip.ParseAddr(host)

	return err == nil && ip.Unmap().IsLoopback()
}

// LoopbackURL reports whether appURL is exactly http://localhost:<port> or
// http://127.0.0.1:<port> (SDD-007 section 4.3 item 4).
func LoopbackURL(appURL string) bool {
	u, err := url.Parse(appURL)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
		(u.Path != "" && u.Path != "/") {
		return false
	}

	port := u.Port()
	if n, perr := strconv.Atoi(port); perr != nil || n < 1 || n > 65535 {
		return false
	}

	h := u.Hostname()

	return h == "localhost" || h == "127.0.0.1"
}

// LimitVariables are the environment limits that SDD-007 section 9.1 names. In
// network mode one of them set to zero or a negative number is a boot error.
func LimitVariables() []string {
	return []string{
		"STUARTY_MAX_EXPORTS", "STUARTY_MAX_EXPORTS_PER_KEY", "STUARTY_MAX_INGESTS",
		"STUARTY_MAX_TRANSCRIBES", "STUARTY_MAX_TRANSCRIBES_PER_KEY", "STUARTY_MAX_IMPORT_BYTES",
		"STUARTY_MAX_IMPORT_SECONDS", "STUARTY_MAX_IMPORT_PIXELS", "STUARTY_MAX_TRANSCRIBE_SECONDS",
		"STUARTY_MAX_EXPORT_BYTES", "STUARTY_MIN_FREE_BYTES", "STUARTY_CACHE_MAX_BYTES",
		"STUARTY_RATE_LIMIT_PER_MINUTE", "STUARTY_RATE_LIMIT_SESSION_PER_MINUTE",
		"STUARTY_RATE_LIMIT_HEAVY_PER_MINUTE",
	}
}

// CheckLimit reads one limit variable. use is false when it is unset, or zero
// or negative in local mode (the default applies). In network mode a value that
// is not a positive number is an "invalid limit" error that names the variable;
// a value that is not a number is one in both modes.
func CheckLimit(name, raw string, network bool) (use bool, err error) {
	if strings.TrimSpace(raw) == "" {
		return false, nil
	}

	v, perr := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if perr != nil {
		return false, fmt.Errorf("invalid limit: %s=%q is not a number: set a positive number or unset it", name, raw)
	}

	if v <= 0 {
		if network {
			return false, fmt.Errorf(
				"invalid limit: %s=%q: network mode needs a positive number: set one or unset it for the default",
				name, raw,
			)
		}

		return false, nil
	}

	return true, nil
}

// Validate is the one place that decides whether the process may start. It
// returns every problem it finds, one per row of SDD-007 section 4.5 that
// applies, so the operator fixes them in one pass; none is a warning. The
// Resolved value is meaningful when there is no problem.
func Validate(env Env) (Resolved, []Problem) {
	env.Probes = env.Probes.withDefaults(env)

	v := &validator{env: env}

	res := Resolved{Mode: ModeOf(env.Getenv)}

	if res.Mode != ModeLocal && res.Mode != ModeNetwork {
		v.add(RowUnknownMode, EnvMode, fmt.Sprintf(
			"unknown mode %q: set %s to local or network", res.Mode, EnvMode,
		))

		return res, v.problems
	}

	host, port := Addr(env.Getenv)
	res.Addr = net.JoinHostPort(host, port)

	network := res.Network()

	if !network && !IsLoopbackHost(host) {
		v.add(RowAddrNotLoopback, EnvAddr, fmt.Sprintf(
			"bind address %q (STUARTY_ADDR) is not loopback: set %s=network and read the self-hosting guide, "+
				"or bind 127.0.0.1", res.Addr, EnvMode,
		))
	}

	if network {
		v.network(&res)
	}

	v.common(network)

	return res, v.problems
}

type validator struct {
	env      Env
	problems []Problem
}

func (v *validator) add(row Row, variable, msg string) {
	v.problems = append(v.problems, Problem{Row: row, Variable: variable, Message: msg})
}

func (v *validator) network(res *Resolved) {
	get := v.env.Getenv

	appURL := strings.TrimSpace(get(envAppURL))
	res.LoopbackHTTP = LoopbackURL(appURL)
	res.TrustForwardedProto, _ = strconv.ParseBool(strings.TrimSpace(get(envTrustProto)))
	res.TrustedProxies = ParseProxies(get(envTrustedProxies))

	u, err := url.Parse(appURL)
	httpsOrigin := err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil &&
		strings.Trim(u.Path, "/") == "" && u.RawQuery == "" && u.Fragment == ""

	switch {
	case !httpsOrigin && !res.LoopbackHTTP:
		v.add(RowAppURL, envAppURL, "network mode needs APP_URL=https://<your host> (the address people type); "+
			"for a container used only on its own machine use APP_URL=http://localhost:<port>")
	default:
		res.AppURL = strings.TrimSuffix(appURL, "/")
	}

	viaProxy := len(res.TrustedProxies) > 0 && res.TrustForwardedProto
	ownTLS := get(envTLSCert) != "" && get(envTLSKey) != ""

	if httpsOrigin && !ownTLS && !viaProxy {
		v.add(RowNoTLS, envTLSCert, "network mode needs TLS: set TLS_CERT_FILE and TLS_KEY_FILE, or put a proxy in "+
			"front and set TRUSTED_PROXIES and TRUST_FORWARDED_PROTO=true; there is no plain-HTTP option")
	}

	if !validAppKey(get(envAppKey)) {
		v.add(RowAppKey, envAppKey, "APP_KEY missing, not base64 or shorter than 32 bytes: set APP_KEY to a "+
			"random secret of at least 32 bytes in the form base64:<value> (vukem key:generate)")
	}

	if allow, _ := strconv.ParseBool(get(EnvAllowRoot)); !allow && v.env.Probes.Privileged() {
		v.add(RowRoot, EnvAllowRoot, "refusing to run as root or Administrator in network mode: run as an "+
			"unprivileged user, or set STUARTY_INSECURE_ALLOW_ROOT=true if you accept the risk")
	}

	for _, name := range LimitVariables() {
		if _, err := CheckLimit(name, get(name), true); err != nil {
			v.add(RowInvalidLimit, name, err.Error())
		}
	}
}

// common holds the rows that apply in both modes.
func (v *validator) common(network bool) {
	get := v.env.Getenv

	v.importRoots()

	if raw := strings.TrimSpace(get(EnvTranscribeURL)); raw != "" {
		if _, err := outbound.ParseEndpoint(raw); err != nil {
			v.add(RowTranscribe, EnvTranscribeURL, fmt.Sprintf(
				"unsafe transcription endpoint in %s: use https, or http only for a loopback host, "+
					"with no user name, password, query or fragment (%v)", EnvTranscribeURL, err,
			))
		}
	} else if get(EnvTranscribeKey) != "" {
		v.add(RowTranscribe, EnvTranscribeKey, "unsafe transcription endpoint: STUARTY_TRANSCRIBE_KEY is set "+
			"without STUARTY_TRANSCRIBE_URL: set the URL, or unset the key so it is never sent anywhere")
	}

	if !network && v.env.Probes.TokenFile != nil {
		if err := v.env.Probes.TokenFile(); errors.Is(err, workspace.ErrTokenPermissions) {
			v.add(RowTokenFile, "token", "token file permissions are too loose: only its owner may read the "+
				"token file in the workspace: chmod 600 it, or run stuarty token:rotate")
		}
	}

	if strings.EqualFold(strings.TrimSpace(get(EnvSandbox)), "bwrap") {
		if err := v.env.Probes.Sandbox(); err != nil {
			v.add(RowSandbox, EnvSandbox, fmt.Sprintf(
				"sandbox requested but unavailable (%v): install bubblewrap and allow user namespaces, "+
					"or unset STUARTY_SANDBOX", err,
			))
		}
	}

	for _, name := range []string{EnvFFmpeg, EnvFFprobe} {
		if p := get(name); p != "" && !v.regularAbsFile(p) {
			v.add(RowFFmpegPath, name, fmt.Sprintf(
				"invalid FFmpeg path in %s: it must be an absolute path to an existing regular file", name,
			))
		}
	}
}

// validAppKey is the vukem rule: standard base64, with an optional "base64:"
// prefix, that decodes to at least 32 bytes.
func validAppKey(raw string) bool {
	b, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(raw, "base64:"))

	return err == nil && len(b) >= minAppKeyBytes
}

func (v *validator) regularAbsFile(p string) bool {
	if !filepath.IsAbs(p) {
		return false
	}

	fi, err := v.env.Probes.Stat(p)

	return err == nil && fi.Mode().IsRegular()
}

func (v *validator) importRoots() {
	raw := v.env.Getenv(EnvImportRoots)

	for entry := range strings.SplitSeq(raw, string(os.PathListSeparator)) {
		if entry = strings.TrimSpace(entry); entry == "" {
			continue
		}

		if reason := v.badRoot(entry); reason != "" {
			v.add(RowImportRoot, EnvImportRoots, fmt.Sprintf(
				"invalid import root %q in %s: %s", entry, EnvImportRoots, reason,
			))
		}
	}
}

func (v *validator) badRoot(root string) string {
	if !filepath.IsAbs(root) {
		return "it is not an absolute path: use a full path"
	}

	root = filepath.Clean(root)
	if filepath.Dir(root) == root {
		return "it is a filesystem root: name a narrower directory"
	}

	fi, err := v.env.Probes.Stat(root)
	if err != nil || !fi.IsDir() {
		return "it is not an existing directory: create it or fix the path"
	}

	if ws := v.env.Workspace; ws != "" && insideOrEqual(ws, root) {
		return "it is the workspace or lies inside it: name a directory outside " + ws
	}

	return ""
}

// insideOrEqual reports whether path is dir or lies under it, after symlinks
// where they resolve. The comparison folds case on Windows and macOS, whose
// default volumes are case-insensitive.
func insideOrEqual(dir, path string) bool {
	resolve := func(p string) string {
		p = filepath.Clean(p)
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return r
		}

		return p
	}

	d, p := resolve(dir), resolve(path)
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		d, p = strings.ToLower(d), strings.ToLower(p)
	}

	return p == d || strings.HasPrefix(p, d+string(filepath.Separator))
}

// ParseProxies reads TRUSTED_PROXIES: addresses and CIDR ranges separated by
// commas. An entry that does not parse is skipped; vukem refuses it at boot.
func ParseProxies(raw string) []netip.Prefix {
	var out []netip.Prefix

	for entry := range strings.SplitSeq(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}

		if p, err := netip.ParsePrefix(entry); err == nil {
			out = append(out, p)

			continue
		}

		if a, err := netip.ParseAddr(entry); err == nil {
			a = a.Unmap()
			out = append(out, netip.PrefixFrom(a, a.BitLen()))
		}
	}

	return out
}

func (p Probes) withDefaults(env Env) Probes {
	if p.Privileged == nil {
		p.Privileged = privileged
	}

	if p.Stat == nil {
		p.Stat = os.Stat
	}

	if p.TokenFile == nil && env.Workspace != "" {
		p.TokenFile = func() error {
			_, err := workspace.Dir(env.Workspace).LoadToken()

			return err
		}
	}

	if p.Sandbox == nil {
		p.Sandbox = bwrapAvailable
	}

	return p
}

// Report writes the problems, one line each, and returns the exit status of
// the process: ExitStatus when there is any, 0 when there is none.
func Report(w io.Writer, problems []Problem) int {
	for _, p := range problems {
		_, _ = fmt.Fprintln(w, "stuarty: "+p.Message)
	}

	if len(problems) > 0 {
		return ExitStatus
	}

	return 0
}

// bwrapAvailable runs bwrap once with a namespace, which is the thing the
// sandbox needs and the only honest test of it.
func bwrapAvailable() error {
	if runtime.GOOS != "linux" {
		return errors.New("bwrap runs on Linux only")
	}

	path, err := exec.LookPath("bwrap")
	if err != nil {
		return errors.New("bwrap is not on the PATH")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Like every child (SDD-007 6.4): no application variable such as STUARTY_KEY reaches the probe.
	probe := exec.CommandContext(ctx, path, "--unshare-user", "--ro-bind", "/", "/", "true")
	probe.Dir = os.TempDir()
	probe.Env = []string{"PATH=" + os.Getenv("PATH")}
	if err := probe.Run(); err != nil {
		return errors.New("bwrap cannot create a namespace")
	}

	return nil
}
