package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/AlexGalhardo/stuarty/engine/ffmpeg"
	"github.com/AlexGalhardo/stuarty/engine/transcribe"
	"github.com/AlexGalhardo/stuarty/service/connections"
	"github.com/AlexGalhardo/stuarty/service/workspace"
)

// Modes of the server a Doctor report describes.
const (
	ModeLocal   = "local"
	ModeNetwork = "network"
)

// DoctorOptions is what the process adds to the report: facts only it knows.
type DoctorOptions struct {
	// Mode is ModeLocal or ModeNetwork; ModeLocal when empty.
	Mode string
	// StartedAt is when the process started.
	StartedAt time.Time
	// Environ lists the environment as KEY=value lines; os.Environ when nil.
	Environ func() []string
	// FFmpegPins maps "ffmpeg" and "ffprobe" to the SHA-256 the embedded manifest records for the
	// bundled copy on this platform. Empty until the release embeds one.
	FFmpegPins map[string]string
	// Integrity runs PRAGMA integrity_check and returns what it printed ("ok" when sound).
	Integrity func(ctx context.Context) (string, error)
	// Child is the policy the owner applied to FFmpeg and whisper.cpp (SDD-007 sections 6.5 and 6.7).
	Child ChildPolicy
	// GPURenderer is set when STUARTY_EXPORT_RENDERER=gpu asks for the GPU render host.
	GPURenderer GPURenderer
}

// GPURenderer says whether the GPU renderer was requested and where its host was found.
type GPURenderer struct {
	// Requested is true when the configured renderer is gpu.
	Requested bool
	// Host is the absolute path of stuarty-render; empty when it was not found.
	Host string
}

// ChildPolicy says what bounds the child processes of this owner.
type ChildPolicy struct {
	// Limits is true when prlimit(1) bounds every child (Linux).
	Limits bool
	// LimitsWhy is why the limits are unavailable, when they are.
	LimitsWhy string
	// Sandbox is the path of bwrap when STUARTY_SANDBOX=bwrap wraps every child; empty is none.
	Sandbox string
}

// DoctorRequest asks for the diagnostic report (SDD-003 section 2, SDD-011 section 8.6).
type DoctorRequest struct {
	// Full runs the checks; without it only the identity is answered.
	Full bool `json:"full,omitempty"`
	// Verify recomputes the SHA-256 of FFmpeg, ffprobe and the installed helpers.
	Verify bool `json:"verify,omitempty"`
	// GC runs the orphan sweep; only the owner of the machine may ask for it.
	GC bool `json:"gc,omitempty"`
	// Bundle writes exports/diagnostics/doctor-<UTC timestamp>.zip inside the workspace.
	Bundle bool `json:"bundle,omitempty"`
	// BundleProject adds the document of this project to the bundle: the explicit flag of RF-207.
	BundleProject string `json:"bundle_project,omitempty"`
}

// DoctorCheck is one line of the report.
type DoctorCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	Hint   string `json:"hint,omitempty"`
	// Optional marks a check whose failure does not stop the product from working (the transcription
	// helper and its model): it does not make the CLI exit non-zero (VV-110).
	Optional bool `json:"optional,omitempty"`
	// Status is what the text shows for the check: ok, warn (it passed with a hint), missing (an
	// optional check failed) or fail. A reader of the JSON sees the same as a reader of the text.
	Status string `json:"status"`
}

// status is the one place that decides the state of a check, for the text and for the JSON.
func (c *DoctorCheck) status() string {
	switch {
	case !c.OK && c.Optional:
		return "missing"
	case !c.OK:
		return "fail"
	case c.Hint != "":
		return "warn"
	default:
		return "ok"
	}
}

// DoctorResult is the report.
type DoctorResult struct {
	Version    string        `json:"version"`
	APIVersion string        `json:"api_version"`
	Mode       string        `json:"mode"`
	PID        int           `json:"pid"`
	StartedAt  time.Time     `json:"started_at"`
	Workspace  string        `json:"workspace,omitempty"`
	Checks     []DoctorCheck `json:"checks,omitempty"`
	// GC is what the sweep removed, when it was asked for.
	GC *SweepResult `json:"gc,omitempty"`
	// Bundle is the slash path of the bundle inside the workspace, when it was asked for.
	Bundle string `json:"bundle,omitempty"`
	Text   string `json:"text"`
}

// bundleStamp is the UTC timestamp in the name of a bundle, from the service clock.
func (s *svc) bundleStamp() string { return s.now().UTC().Format("20060102T150405Z") }

// secretVars are the variables whose value is never printed, bundled or logged
// (SDD-007 section 11). The check below is by name and by pattern, so a new
// secret named like these is covered before it is listed.
var secretVars = map[string]bool{
	"APP_KEY":                    true,
	"STUARTY_KEY":                true,
	"STUARTY_TRANSCRIBE_KEY":     true,
	"STUARTY_ADMIN_PASSWORD":     true,
	"OTEL_EXPORTER_OTLP_HEADERS": true,
	"STUARTY_TRANSCRIBE_TOKEN":   true,
}

// plainVars are the only variables whose value is printed: limits and switches that say nothing
// about the machine. Every other STUARTY_ variable prints "set".
var plainVars = map[string]bool{
	"STUARTY_MAX_IMPORT_BYTES": true, "STUARTY_MAX_EXPORT_BYTES": true, "STUARTY_MIN_FREE_BYTES": true,
	"STUARTY_MAX_INGESTS": true, "STUARTY_MAX_EXPORTS": true, "STUARTY_MAX_EXPORTS_PER_KEY": true,
	"STUARTY_MAX_TRANSCRIBES": true, "STUARTY_MAX_TRANSCRIBES_PER_KEY": true,
	"STUARTY_MAX_TRANSCRIBE_SECONDS": true, "STUARTY_WHISPER_MODEL": true, "STUARTY_ENCODER": true,
	"STUARTY_ALLOW_USER_X264": true, "STUARTY_EXPORT_RENDERER": true,
}

func isSecretVar(name string) bool {
	if secretVars[name] {
		return true
	}

	u := strings.ToUpper(name)

	return strings.Contains(u, "SECRET") || strings.Contains(u, "PASSWORD") || strings.Contains(u, "TOKEN") ||
		strings.HasSuffix(u, "_KEY") || strings.HasSuffix(u, "_HEADERS")
}

// secretValues are the values of every secret variable that is set: what the
// bundle must never contain.
func (s *svc) secretValues() []string {
	var out []string

	for _, kv := range s.environ() {
		name, value, ok := strings.Cut(kv, "=")
		if ok && value != "" && isSecretVar(name) {
			out = append(out, value)
		}
	}

	return out
}

func (s *svc) environ() []string {
	if s.doctor.Environ != nil {
		return s.doctor.Environ()
	}

	return os.Environ()
}

func (s *svc) Doctor(ctx context.Context, req DoctorRequest) (*DoctorResult, error) {
	c, err := authorize(ctx, "Doctor", "")
	if err != nil {
		return nil, err
	}

	mode := s.doctor.Mode
	if mode == "" {
		mode = ModeLocal
	}

	// Paths and environment details belong to the person running the machine: a browser session,
	// the operator or an admin key that is not limited to one project. The mode does not decide it:
	// a project-limited agent key exists in local mode too.
	privileged := humanForTools(c) && c.ProjectID == ""

	if (req.Verify || req.Bundle) && !privileged {
		return nil, newError(CodeForbidden, "only the person running the server can verify digests or write a bundle.").
			withHint("use the editor or the terminal of the server machine, or an admin key not limited to one project")
	}

	if req.GC && !c.Browser && !c.Operator {
		return nil, newError(CodeForbidden, "only the person running the server can run the sweep.").
			withHint("run it from the terminal of the server machine: stuarty doctor --gc")
	}

	res := &DoctorResult{
		Version: s.version, APIVersion: "v1", Mode: mode, PID: os.Getpid(), StartedAt: s.doctor.StartedAt,
	}
	// In local mode the folder is the one of the person who runs the editor, and every local caller can
	// read it from the token file anyway, so the editor session of a launch code sees it too (VV-129).
	// A key limited to one project never does.
	if mode == ModeLocal && c.ProjectID == "" && s.workspace != "" {
		res.Workspace = s.workspace
	}

	if req.Full || req.Verify || req.Bundle {
		// The checks hash files, read capabilities and run test encodes: one at a time, and a
		// second caller is told to wait instead of piling up work outside the FFmpeg limits.
		if !s.doctorMu.TryLock() {
			return nil, newError(CodeQueueFull, "a diagnostic run is already in progress.").
				withDetail("limit", "doctor").
				withHint("wait for it to finish and ask again")
		}

		defer s.doctorMu.Unlock()

		res.Checks = s.doctorChecks(ctx, privileged, req.Verify)
	}

	if req.GC {
		sweep, serr := s.Sweep(ctx)
		if serr != nil {
			return nil, internalError()
		}

		res.GC = &sweep
		res.Checks = append(res.Checks, DoctorCheck{Name: "workspace.sweep", OK: true, Detail: sweepText(sweep)})
	}

	for i := range res.Checks {
		res.Checks[i].Status = res.Checks[i].status()
	}

	res.Text = doctorText(res)

	if req.Bundle {
		if !privileged {
			return nil, newError(CodeForbidden, "the diagnostic bundle is for the person running the server.").
				withHint("use the local mode or an admin key")
		}

		name, berr := s.writeBundle(ctx, c, res, req.BundleProject)
		if berr != nil {
			return nil, berr
		}

		res.Bundle = name
		res.Text = doctorText(res)
	}

	return res, nil
}

func sweepText(r SweepResult) string {
	return fmt.Sprintf(
		"removed %d store files, %d artifact folders, %d export folders, %d frame folders, %d incoming files",
		r.StoreFiles,
		r.ArtifactDirs,
		r.ExportDirs,
		r.FrameDirs,
		r.IncomingFiles,
	)
}

func doctorText(r *DoctorResult) string {
	var b strings.Builder

	fmt.Fprintf(&b, "stuarty %s (%s, pid %d)\n", r.Version, r.Mode, r.PID)

	for _, c := range r.Checks {
		state := c.status()
		if state == "fail" {
			state = "FAIL"
		}

		fmt.Fprintf(&b, "%s %s: %s\n", state, c.Name, c.Detail)

		if c.Hint != "" {
			fmt.Fprintf(&b, "  hint: %s\n", c.Hint)
		}
	}

	if len(r.Checks) == 0 {
		b.WriteString("no check was run: use --full for FFmpeg, the workspace and the helpers\n")
	}

	if r.Bundle != "" {
		fmt.Fprintf(&b, "bundle: %s\n", r.Bundle)
	}

	return strings.TrimRight(b.String(), "\n")
}

// doctorChecks are the named checks of SDD-011 section 8.6 and the workspace ones of SDD-007.
func (s *svc) doctorChecks(ctx context.Context, privileged, verify bool) []DoctorCheck {
	var out []DoctorCheck

	out = append(out, s.ffmpegChecks(ctx, privileged)...)
	out = append(out, s.childPolicyChecks()...)
	out = append(out, s.rendererChecks(privileged)...)
	out = append(out, s.transcribeChecks()...)
	out = append(out, s.workspaceChecks(ctx, privileged)...)
	out = append(out, DoctorCheck{
		Name: "system", OK: true,
		Detail: fmt.Sprintf("%s %s, %d CPUs", runtime.GOOS, runtime.GOARCH, runtime.NumCPU()),
	})

	if privileged {
		out = append(out, s.environmentCheck(), s.vaultCheck())
	}

	if verify {
		out = append(out, s.verifyChecks()...)
	}

	return out
}

// ffmpegChecks are ffmpeg.binary, .license, .capabilities and .encoder, then ffmpeg.openh264 for the
// own build (SDD-011 section 8.6).
func (s *svc) ffmpegChecks(ctx context.Context, privileged bool) []DoctorCheck {
	out, chosen := s.ffmpegCoreChecks(ctx, privileged)

	r := s.media.Runner
	if r == nil || r.In == nil {
		return out
	}

	dir, manifest := "", ffmpeg.OpenH264Manifest{}
	if s.tools != nil {
		dir, manifest = s.tools.Dir, s.tools.openH264Manifest()
	}

	goos, goarch := "", ""
	if s.tools != nil {
		goos, goarch = s.tools.platform()
	}

	other := chosen != "" && chosen != "libopenh264"
	if check, ok := openH264Check(r.In, dir, manifest, goos, goarch, other); ok {
		out = append(out, check)
	}

	return out
}

// licenseCheck is ffmpeg.license: the classification of SDD-011 section 8.2, the build it is
// (vukem, btbn or other) and the flags a build this project distributes must not carry.
func licenseCheck(in *ffmpeg.Installation, privileged bool) DoctorCheck {
	lic := in.Info.License
	fl := in.Flavour()
	problems := fl.Problems()
	licOK := lic.Name != "nonfree" && len(problems) == 0

	licHint := ""

	switch {
	case len(problems) > 0:
		licHint = "this is a build the project distributes and it carries " + strings.Join(problems, ", ") +
			"; use a build without it"
	case lic.Name == "nonfree":
		licHint = "this FFmpeg was built with --enable-nonfree and cannot be redistributed; use an LGPL build"
	case lic.Name == "gpl":
		licHint = "this FFmpeg is GPL (" + gplFlags(in.Info.Flags) + "), not the LGPL build the product is made for: " +
			"fine for your own use, not for anything this project ships or redistributes; " +
			"point STUARTY_FFMPEG at an LGPL build (the pinned one, or the FFmpeg of the container image)"
	}

	parts := []string{lic.Name + versionSuffix(lic.Version3), "flavour " + string(fl.Name)}
	if len(problems) > 0 {
		parts = append(parts, "problems "+strings.Join(problems, ","))
	}

	if privileged {
		parts = append(parts, strings.Join(in.Info.Flags, " "))
	}

	return DoctorCheck{
		Name:   "ffmpeg.license",
		OK:     licOK,
		Hint:   licHint,
		Detail: strings.TrimSpace(strings.Join(parts, "; ")),
	}
}

// gplFlags names the configure flags that make an FFmpeg GPL, or "GPL libraries" when the licence
// came from the encoders (libx264 or libx265) and no flag says it.
func gplFlags(flags []string) string {
	var out []string

	for _, f := range flags {
		if f == "--enable-gpl" || f == "--enable-libx264" || f == "--enable-libx265" {
			out = append(out, f)
		}
	}

	if len(out) == 0 {
		return "GPL libraries"
	}

	return strings.Join(out, " ")
}

// openH264Check is ffmpeg.openh264 and exists for the own build only. It reads the library from disk
// again, so an install made since the start shows. It is ok when the library is installed and
// unchanged, or another H.264 encoder works (otherEncoder).
func openH264Check(
	in *ffmpeg.Installation, toolsDir string, m ffmpeg.OpenH264Manifest, goos, goarch string, otherEncoder bool,
) (DoctorCheck, bool) {
	if in.Flavour().Name != ffmpeg.FlavourOwn {
		return DoctorCheck{}, false
	}

	st := ffmpeg.CheckOpenH264(toolsDir, m, goos, goarch)
	check := DoctorCheck{Name: "ffmpeg.openh264", OK: st.Installed || otherEncoder}

	if st.Installed {
		check.Detail = "installed"

		return check, true
	}

	reason := st.Reason
	if reason == "" {
		reason = ffmpeg.ReasonOpenH264NotInstalled
	}

	check.Detail = reason

	if reason != ffmpeg.ReasonOpenH264Platform {
		check.Detail += fmt.Sprintf(", %.1f MB to download", float64(st.Entry.Bytes)/1e6)
		check.Hint = `Run "` + ffmpeg.OpenH264InstallCommand + `".`
	}

	return check, true
}

func (s *svc) ffmpegCoreChecks(ctx context.Context, privileged bool) (checks []DoctorCheck, chosen string) {
	r := s.media.Runner
	if r == nil || r.In == nil {
		return []DoctorCheck{{
			Name: "ffmpeg.binary", OK: false, Detail: "E_FFMPEG_MISSING: no usable FFmpeg was found",
			Hint: "install FFmpeg 9 or set STUARTY_FFMPEG to its absolute path; import and export need it",
		}}, ""
	}

	in := r.In
	detail := fmt.Sprintf("%s, %s, ffprobe %s", in.Source, in.Info.Version, in.ProbeInfo.Version)

	if privileged {
		detail += ", " + in.FFmpeg.Path()
	}

	out := []DoctorCheck{{
		Name: "ffmpeg.binary", OK: true, Detail: detail, Hint: strings.Join(in.Warnings(), ", "),
	}}

	out = append(out, licenseCheck(in, privileged))

	caps, cerr := s.capabilities(ctx, r)
	if cerr != nil {
		return append(out, DoctorCheck{
			Name: "ffmpeg.capabilities", OK: false, Detail: "the capabilities could not be read",
			Hint: "run doctor again; if it persists the FFmpeg build is damaged",
		}), ""
	}

	out = append(out, DoctorCheck{
		Name: "ffmpeg.capabilities", OK: true,
		Detail: fmt.Sprintf("zscale=%t tonemap=%t bwdif=%t yadif=%t hdr_ok=%t",
			caps.Zscale, caps.Tonemap, caps.Bwdif, caps.Yadif, caps.HDROK),
	})

	det, derr := r.DetectEncoder(ctx, caps, ffmpeg.DetectOptions{})
	if derr != nil || det == nil || det.Chosen == "" {
		return append(out, DoctorCheck{
			Name: "ffmpeg.encoder", OK: false, Detail: "E_NO_ENCODER: no H.264 encoder passed the test encode",
			Hint: "install an FFmpeg with an H.264 encoder (openh264, a hardware encoder) or see doctor --full",
		}), ""
	}

	parts := make([]string, 0, len(det.Candidates))
	for _, cd := range det.Candidates {
		parts = append(parts, fmt.Sprintf("%s %s %d ms", cd.Name, cd.Status, cd.MS))
	}

	return append(out, DoctorCheck{
		Name: "ffmpeg.encoder", OK: true,
		Detail: fmt.Sprintf("%s (forced %t); %s", det.Chosen, det.Forced, strings.Join(parts, ", ")),
	}), det.Chosen
}

// capabilities reads what FFmpeg can do once; the doctor lock serialises the callers.
func (s *svc) capabilities(ctx context.Context, r *ffmpeg.Runner) (*ffmpeg.Capabilities, error) {
	if s.doctorCaps != nil {
		return s.doctorCaps, nil
	}

	caps, err := r.ReadCapabilities(ctx)
	if err != nil {
		return nil, err
	}

	s.doctorCaps = caps

	return caps, nil
}

func versionSuffix(v3 bool) string {
	if v3 {
		return ", version3"
	}

	return ""
}

func (s *svc) transcribeChecks() []DoctorCheck {
	provider := "none"
	if s.transcriber != nil {
		provider = s.transcriber.Name()
	}

	out := []DoctorCheck{{Name: "transcribe.provider", OK: true, Detail: provider}}

	if s.tools == nil {
		return out
	}

	goos, goarch := s.tools.platform()

	for _, kind := range []string{transcribe.KindHelper, transcribe.KindModel} {
		out = append(out, s.toolCheck(kind, goos, goarch))
	}

	return out
}

// toolCheck reports whether the helper or the model is installed and verified.
func (s *svc) toolCheck(kind, goos, goarch string) DoctorCheck {
	check := DoctorCheck{Name: "transcribe." + kind, Optional: true}

	entries, ok := s.tools.Manifest.For("whisper", s.toolsModel(), goos, goarch)
	if !ok {
		check.Detail = "this build pins no " + kind + " for " + goos + "/" + goarch

		return check
	}

	for i := range entries {
		e := &entries[i]
		if e.Kind != kind {
			continue
		}

		if _, found := VerifiedTool(s.tools.Dir, e.Name, e.Kind, e.Model); found {
			check.OK, check.Detail = true, "installed, version "+e.Version

			if kind == transcribe.KindModel {
				check.Detail += ", model " + e.Model + s.modelOrigin(e.Model)
			}

			return check
		}

		check.Detail = fmt.Sprintf("not installed or changed since the install, %d bytes to download", e.Bytes)
		check.Hint = `Run "stuarty tools:install whisper".`

		return check
	}

	check.Detail = "not in the manifest"

	return check
}

// modelOrigin says whether the effective model is the one the install recorded as the default.
func (s *svc) modelOrigin(model string) string {
	if def, ok := DefaultModel(s.tools.Dir, "whisper"); ok && def == model {
		return " (workspace default)"
	}

	return ""
}

func (s *svc) toolsModel() string {
	if s.transcriber != nil {
		if m := s.transcriber.Model(); slices.Contains(transcribe.Models, m) {
			return m
		}
	}

	return "small"
}

func (s *svc) workspaceChecks(ctx context.Context, privileged bool) []DoctorCheck {
	var out []DoctorCheck

	integrity := "not checked"
	healthy := true

	if s.doctor.Integrity != nil {
		msg, err := s.doctor.Integrity(ctx)
		if err != nil || msg != "ok" {
			healthy, integrity = false, "integrity_check failed"
		} else {
			integrity = "database ok"
		}
	}

	free := "free space unknown"
	hint := ""

	if s.workspace != "" && s.media.FreeBytes != nil {
		if n, err := s.media.FreeBytes(s.workspace); err == nil {
			free = fmt.Sprintf("%d MiB free", n>>20)

			if s.limits.MinFreeBytes > 0 && n < uint64(s.limits.MinFreeBytes) {
				healthy = false
				hint = "free space on the disk of the workspace or lower STUARTY_MIN_FREE_BYTES"
			}
		}
	}

	out = append(out, DoctorCheck{Name: "workspace.store", OK: healthy, Detail: integrity + ", " + free, Hint: hint})

	if s.workspace == "" {
		return out
	}

	if _, err := workspace.Dir(s.workspace).LoadToken(); errors.Is(err, workspace.ErrTokenPermissions) {
		out = append(out, DoctorCheck{
			Name: "workspace.token", OK: false, Detail: "the token file is readable by other users",
			Hint: "restrict the token file to its owner (chmod 600) and rotate the token",
		})
	}

	if privileged {
		if name := cloudSyncFolder(s.workspace); name != "" {
			out = append(out, DoctorCheck{
				Name: "workspace.location", OK: true, Detail: "the workspace is inside " + name,
				Hint: "a synchronised folder can corrupt the database and the lock; keep the workspace outside it",
			})
		}
	}

	return out
}

// cloudSyncFolder names the synchronisation service whose folder holds the path, or "".
func cloudSyncFolder(p string) string {
	slashed := strings.ToLower(strings.ReplaceAll(p, `\`, "/"))

	for marker, name := range map[string]string{
		"/onedrive": "OneDrive", "/dropbox": "Dropbox", "/google drive": "Google Drive", "/googledrive": "Google Drive",
		"/mobile documents": "iCloud Drive", "/library/cloudstorage": "a cloud storage folder", "/icloud": "iCloud Drive",
		"/box sync": "Box",
	} {
		if strings.Contains(slashed, marker) {
			return name
		}
	}

	return ""
}

// environmentCheck lists the STUARTY_* variables that are set: the value
// of a plain one, `set` for a secret, and nothing for a path unless the caller
// is privileged (the caller of this function is).
func (s *svc) environmentCheck() DoctorCheck {
	var parts []string

	for _, kv := range s.environ() {
		name, value, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(name, "STUARTY_") || value == "" {
			continue
		}

		if plainVars[name] && !isSecretVar(name) {
			parts = append(parts, name+"="+sanitizeValue(value))
		} else {
			parts = append(parts, name+"=set")
		}
	}

	sort.Strings(parts)

	detail := "no STUARTY_ variable is set"
	if len(parts) > 0 {
		detail = strings.Join(parts, " ")
	}

	return DoctorCheck{Name: "environment", OK: true, Detail: detail}
}

// vaultCheck says where the master key of the vault comes from and whether it opens the vault
// (SDD-014 section 8). The owner removes the key variables from its environment, so this check, not
// environment, is where they show; it names the provider and never a value or a path.
func (s *svc) vaultCheck() DoctorCheck {
	if s.conns == nil {
		// New always builds a registry; only a service assembled by hand in a test has none.
		return DoctorCheck{Name: "vault", OK: true, Detail: "no vault in this process"}
	}

	st := s.conns.VaultStatus()

	file := "no vault file yet"
	if st.Exists {
		file = "vault.json present"
	}

	check := DoctorCheck{Name: "vault", OK: st.Err == nil, Detail: "master key from " + st.Provider + "; " + file}

	if st.Err != nil {
		check.Hint = "the vault cannot be read"
		if _, hint, ok := connections.Locked(st.Err); ok {
			check.Hint = hint
		}
	}

	return check
}

// sanitizeValue keeps a value on one line and short.
func sanitizeValue(v string) string {
	v = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return '_'
		}

		return r
	}, v)

	if len(v) > 120 {
		v = v[:120] + "..."
	}

	return v
}

// verifyChecks recomputes the digests of FFmpeg, ffprobe and the installed
// helpers (SDD-007 section 6.8). A user's own FFmpeg is the user's choice and is
// only fingerprinted; a bundled copy must match the pin of the build.
func (s *svc) verifyChecks() []DoctorCheck {
	var out []DoctorCheck

	if r := s.media.Runner; r != nil && r.In != nil {
		tools := map[string]string{"ffmpeg": r.In.FFmpeg.Path(), "ffprobe": r.In.FFprobe.Path()}

		for _, name := range []string{"ffmpeg", "ffprobe"} {
			out = append(out, s.digestCheck(name, tools[name], r.In.UserBuild))
		}
	}

	if s.tools != nil {
		for _, t := range readInstalled(s.tools.Dir) {
			check := DoctorCheck{Name: "transcribe.digest", Detail: t.Name + " " + t.Kind}
			if _, ok := VerifiedTool(s.tools.Dir, t.Name, t.Kind, t.Model); ok {
				check.OK = true
			} else {
				check.Detail += ": the file differs from what was installed"
				check.Hint = "install it again with stuarty tools:install"
			}

			out = append(out, check)
		}
	}

	return out
}

func (s *svc) digestCheck(name, path string, userBuild bool) DoctorCheck {
	check := DoctorCheck{Name: "ffmpeg.digest", Detail: name}

	got, err := fileSHA256(path)
	if err != nil {
		check.Detail += ": the file cannot be read"

		return check
	}

	pin := strings.ToLower(s.doctor.FFmpegPins[name])

	switch {
	case userBuild:
		check.OK, check.Detail = true, name+": your own build, not compared (sha256 "+got+")"
	case pin == "":
		check.Detail += ": this build embeds no digest to compare with (sha256 " + got + ")"
		check.Hint = "compare it with checksums.txt of the release"
	case len(pin) == len(got) && subtle.ConstantTimeCompare([]byte(pin), []byte(got)) == 1:
		check.OK, check.Detail = true, name+": matches the release"
	default:
		check.Detail += ": MISMATCH with the release (sha256 " + got + ")"
		check.Hint = "the bundled " + name + " was modified or damaged; install the release again"
	}

	return check
}

// childPolicyChecks reports the limits and the sandbox of the child processes as available or
// unavailable (SDD-007 sections 6.5 and 6.7). Neither is a failure where the platform has none:
// the section says what replaces them there.
func (s *svc) childPolicyChecks() []DoctorCheck {
	p := s.doctor.Child

	limits := DoctorCheck{Name: "child.limits", OK: true, Detail: "unavailable: " + p.LimitsWhy}

	switch {
	case p.Limits && runtime.GOOS == "windows":
		limits.Detail = "available: a Job Object bounds the CPU time and the memory of every child; it has no file " +
			"size limit, so the output size watchdog alone bounds the output"
	case p.Limits:
		limits.Detail = "available: prlimit(1) sets RLIMIT_CPU and RLIMIT_FSIZE on every child, and RLIMIT_AS " +
			"on the software paths"
	case p.LimitsWhy == "":
		limits.Detail = "unavailable: no child process runs"
	default:
		limits.Hint = "the timeouts, the output size watchdog and the concurrency limits still apply"
	}

	sandbox := DoctorCheck{
		Name: "child.sandbox", OK: true,
		Detail: "unavailable: not requested (STUARTY_SANDBOX is unset); children run with the user's privileges",
	}

	if p.Sandbox != "" {
		sandbox.Detail = "available: every FFmpeg run is wrapped by bwrap with no network"
	}

	return []DoctorCheck{limits, sandbox}
}

// rendererChecks is export.renderer, present only when the GPU renderer was requested. A missing
// stuarty-render is not a failure: the export draws on the CPU. The path is shown only to a privileged
// caller, like the FFmpeg paths.
func (s *svc) rendererChecks(privileged bool) []DoctorCheck {
	g := s.doctor.GPURenderer
	if !g.Requested {
		return nil
	}

	if g.Host == "" {
		return []DoctorCheck{{
			Name: "export.renderer", OK: true,
			Detail: "gpu requested, stuarty-render not found, using cpu",
			Hint:   "put stuarty-render next to the server executable or in ../libexec/stuarty",
		}}
	}

	detail := "gpu requested, stuarty-render found"
	if privileged {
		detail = "gpu requested, stuarty-render at " + g.Host
	}

	return []DoctorCheck{{Name: "export.renderer", OK: true, Detail: detail}}
}
