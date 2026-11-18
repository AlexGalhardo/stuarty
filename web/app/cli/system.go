package cli

// The commands that read the state of the installation: doctor, fonts, audit
// and tools:install, and the transcribe command (SDD-003 section 5.4).

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/AlexGalhardo/vukem-video/core/command"
	"github.com/AlexGalhardo/vukem-video/engine/ffmpeg"
	"github.com/AlexGalhardo/vukem-video/engine/transcribe"
	"github.com/AlexGalhardo/vukem-video/service"
	"github.com/AlexGalhardo/vukem-video/web/app/owner"
)

// asOperator marks the caller as the person at the terminal of the owner
// machine. A process that does not own the workspace has no such person: the
// server decides who is allowed (SDD-007 section 6.8).
func (c *call) asOperator(ctx context.Context) context.Context {
	if !c.inProcess {
		return ctx
	}

	caller, _ := service.CallerFrom(ctx)
	caller.Operator = true

	return service.WithCaller(ctx, caller)
}

var doctorCmd = &spec{
	name:    "doctor",
	summary: "check the installation and the workspace",
	usage:   []string{"vukem-video doctor [--full] [--verify] [--bundle] [--gc]"},
	example: "vukem-video doctor --full",
	flags: []flagDef{
		{Name: "full", Kind: kindBool, Default: "false", Usage: "run every check, not only the identity"},
		{Name: "verify", Kind: kindBool, Default: "false", Usage: "recompute the digests of FFmpeg and the helpers"},
		{Name: "bundle", Kind: kindBool, Default: "false", Usage: "write a diagnostic bundle inside the workspace"},
		{Name: "gc", Kind: kindBool, Default: "false", Usage: "remove what no project refers to any more"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.Doctor(c.asOperator(ctx), service.DoctorRequest{
			Full: c.args.boolean("full"), Verify: c.args.boolean("verify"),
			Bundle: c.args.boolean("bundle"), GC: c.args.boolean("gc"),
		})
		if err != nil {
			return nil, err
		}

		out := &output{value: res, text: res.Text}

		for i := range res.Checks {
			ck := &res.Checks[i]
			if ck.OK || ck.Optional {
				continue
			}

			if code, msg, ok := strings.Cut(ck.Detail, ": "); ok && code == "E_FFMPEG_MISSING" {
				return nil, &service.Error{Code: code, Message: msg + ".", Hint: ck.Hint}
			}

			out.exit = ExitFailure
		}

		return out, nil
	},
}

var fontsCmd = &spec{
	name:    "fonts",
	summary: "list the bundled font families",
	usage:   []string{"vukem-video fonts"},
	example: "vukem-video fonts --json",
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.FontList(ctx, service.FontListRequest{})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: fontsText(res)}, nil
	},
}

func fontsText(res *service.FontListResult) string {
	var b strings.Builder

	fmt.Fprintf(&b, "fonts (%d)", len(res.Fonts))

	for _, f := range res.Fonts {
		fmt.Fprintf(&b, "\n%s: %v", f.Family, f.Weights)
	}

	if len(res.Imported) > 0 {
		fmt.Fprintf(&b, "\nimported (%d), by the hash font.add takes:", len(res.Imported))

		for _, f := range res.Imported {
			fmt.Fprintf(&b, "\n%s (%d bytes)", f.Hash, f.Bytes)
		}
	}

	return b.String()
}

var auditCmd = &spec{
	name:    "audit",
	summary: "list the audit table",
	usage:   []string{"vukem-video audit [--project <id>] [--cursor c] [--limit n]"},
	example: "vukem-video audit --limit 20 --json",
	flags: append([]flagDef{
		{Name: "project", Kind: kindString, Usage: "only the entries of this project"},
	}, pageFlags("50")...),
	run: func(ctx context.Context, c *call) (*output, error) {
		res, err := c.svc.AuditList(ctx, service.AuditListRequest{
			ProjectID: c.args.str("project"), Cursor: c.args.str("cursor"), Limit: c.args.integer("limit"),
		})
		if err != nil {
			return nil, err
		}

		return &output{value: res, text: auditText(res)}, nil
	},
}

func auditText(res *service.AuditListResult) string {
	var b strings.Builder

	fmt.Fprintf(&b, "audit (%d entries)", len(res.Entries))

	for i := range res.Entries {
		e := &res.Entries[i]
		fmt.Fprintf(&b, "\n%d %s %s %s", e.ID, e.At, e.Actor, e.Action)

		if e.ProjectID != "" {
			b.WriteString(" " + e.ProjectID)
		}
	}

	if res.NextCursor != nil {
		b.WriteString("\nmore: --cursor " + *res.NextCursor)
	}

	return b.String()
}

var transcribeCmd = &spec{
	name:    "transcribe",
	summary: "transcribe the audio of an asset",
	usage: []string{
		"vukem-video transcribe --project <id> --asset <id> [--language auto] [--model <name>] [--wait] [--timeout d]",
	},
	example: "vukem-video transcribe --project prj_7k2m9x4q1b --asset ast_3c5v8n2w6d --language en",
	project: true,
	globals: []string{"wait", "timeout"},
	write:   true,
	flags: []flagDef{
		{Name: "asset", Kind: kindString, Usage: "asset to transcribe (required)"},
		{Name: "language", Kind: kindString, Default: "auto", Usage: "BCP 47 code, or auto"},
		{Name: "model", Kind: kindString, Usage: "installed model name, such as base (default: the workspace default)"},
	},
	run: func(ctx context.Context, c *call) (*output, error) {
		if !c.args.has("asset") {
			return nil, validation("asset", "--asset is required.")
		}

		raw, err := json.Marshal(service.TranscribeArgs{
			AssetID: c.args.str("asset"), Language: c.args.str("language"), Model: c.args.str("model"),
		})
		if err != nil {
			return nil, err
		}

		deadline := time.Now().Add(c.args.timeout())
		wait := c.args.boolean("wait")

		req := service.JobStartRequest{ProjectID: c.projectID, Kind: service.JobTranscribe, Args: raw}

		return c.runJob(ctx, wait, deadline, func(ctx context.Context, waitMS int) (*service.Job, error) {
			req.WaitMS = waitMS

			return c.svc.JobStart(ctx, req)
		})
	},
}

// toolsInstallDoc is the result of tools:install.
type toolsInstallDoc struct {
	State         string                  `json:"state"`
	Files         []service.ToolFile      `json:"files"`
	DownloadBytes int64                   `json:"download_bytes"`
	Installed     []service.InstalledTool `json:"installed"`
	Text          string                  `json:"text"`
}

// toolsInstallCmd is a terminal command only: the download is started by a
// person, never by an agent call (SDD-007 section 6.8), so it has no MCP tool.
var toolsInstallCmd = &spec{
	name:       "tools:install",
	summary:    "download a helper such as the transcription program",
	usage:      []string{"vukem-video tools:install <name> [--model <model>] [--force] [--yes] [--timeout d]"},
	example:    "vukem-video tools:install whisper --model small",
	positional: true,
	write:      true,
	globals:    []string{"timeout"},
	flags: []flagDef{
		{
			Name:  "model",
			Kind:  kindString,
			Usage: "the model to download, which then becomes the default of transcribe (default: small)",
		},
		{Name: "force", Kind: kindBool, Default: "false", Usage: "download again what is installed and verified"},
		{Name: "yes", Kind: kindBool, Default: "false", Usage: "do not ask for confirmation"},
	},
	run: runToolsInstall,
}

func runToolsInstall(ctx context.Context, c *call) (*output, error) {
	if len(c.args.Positional) != 1 {
		return nil, validation("", "tools:install takes one name, got %d.", len(c.args.Positional))
	}

	name, model := c.args.Positional[0], c.args.str("model")
	if model == "" {
		model = "small"
	}

	announce, err := c.installAnnouncement(name, model)
	if err != nil {
		return nil, err
	}

	if !c.args.boolean("yes") {
		if !c.rt.Interactive || c.args.boolean("json") {
			return nil, validation("yes", "tools:install downloads files: %s. Pass --yes to confirm.", announce)
		}

		if !c.confirm(announce) {
			return &output{
				value: toolsInstallDoc{State: "declined", Text: "nothing was downloaded."},
				text:  "nothing was downloaded.",
			}, nil
		}
	}

	ctx = c.asOperator(ctx)

	inst, here, err := c.installer()
	if err != nil {
		return nil, err
	}

	if here {
		caller, _ := service.CallerFrom(ctx)
		caller.Operator = true
		ctx = service.WithCaller(ctx, caller)
	}

	start, err := inst.ToolsInstall(
		ctx, service.ToolsInstallRequest{Name: name, Model: model, Force: c.args.boolean("force")})
	if err != nil {
		return nil, err
	}

	if start.State == "installed" {
		doc := toolsInstallDoc{State: start.State, Files: start.Files, Text: start.Text}

		return &output{value: doc, text: doc.Text}, nil
	}

	deadline := time.Now().Add(c.args.timeout())
	p := c.progress()

	st, err := inst.ToolsStatus(ctx, service.ToolsStatusRequest{})
	for err == nil && st.State == "running" && time.Now().Before(deadline) {
		p.download(st.Done, st.Total)

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}

		st, err = inst.ToolsStatus(ctx, service.ToolsStatusRequest{})
	}

	if err != nil {
		return nil, err
	}

	doc := toolsInstallDoc{
		State: st.State, Files: start.Files, DownloadBytes: start.DownloadBytes, Installed: st.Installed,
		Text: start.Text + "\n" + st.Text,
	}

	switch st.State {
	case "failed":
		if st.Error != nil {
			return nil, st.Error
		}

		return nil, &service.Error{Code: command.CodeInternal, Message: "the install failed."}
	case "running":
		return &output{
			value: doc,
			text:  doc.Text + "\nstill running: run vukem-video doctor --full to see when it ends",
			exit:  ExitTransient,
		}, nil
	}

	return &output{value: doc, text: doc.Text}, nil
}

// installer is where the install runs. Against a server that owns a local workspace the server
// refuses every caller that holds only the local token, so the person at the terminal installs from
// this process into the same tools directory; the server reads it on its next use (VV-077). Against a
// remote server the server decides, and in process the Service is the owner.
func (c *call) installer() (inst service.ToolsInstaller, here bool, err error) {
	if c.inProcess || c.workspace == "" {
		return c.svc, false, nil
	}

	if c.rt.Installer != nil {
		inst, err = c.rt.Installer(c.workspace)

		return inst, true, err
	}

	t, err := owner.InstallTools(c.workspace)
	if err != nil {
		return nil, false, err
	}

	return service.NewToolsInstaller(t), true, nil
}

// installAnnouncement states what a confirmed install will download, from the
// manifest of the build, before anything is requested.
func (c *call) installAnnouncement(name, model string) (string, error) {
	if name == ffmpeg.OpenH264Tool {
		return openH264Announcement()
	}

	load := c.rt.Manifest
	if load == nil {
		load = transcribe.DefaultManifest
	}

	manifest, err := load()
	if err != nil {
		return "", err
	}

	entries, ok := manifest.For(name, model, runtime.GOOS, runtime.GOARCH)
	if !ok {
		return "", &service.Error{
			Code: command.CodeValidation,
			Message: fmt.Sprintf(
				"there is no %q helper with model %q for %s/%s.", name, model, runtime.GOOS, runtime.GOARCH,
			),
			Path: "name",
			Hint: "the models this build can install: " + strings.Join(manifest.Models(name), ", "),
		}
	}

	var (
		total int64
		host  string
	)

	for i := range entries {
		total += entries[i].Bytes

		if u, perr := url.Parse(entries[i].URL); perr == nil && host == "" {
			host = u.Hostname()
		}
	}

	return fmt.Sprintf("%d files, %.1f MB from %s", len(entries), float64(total)/1e6, host), nil
}

func (c *call) confirm(announce string) bool {
	fmt.Fprintf(c.rt.Stderr, "Download %s? [y/N] ", announce)

	line, _ := bufio.NewReader(c.rt.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))

	return answer == "y" || answer == "yes"
}

// openH264Announcement is the announcement of the Cisco library of the own FFmpeg build, whose
// manifest is the one of engine/ffmpeg (SDD-007 section 6.8), not the transcription manifest.
func openH264Announcement() (string, error) {
	manifest, err := ffmpeg.DefaultOpenH264Manifest()
	if err != nil {
		return "", err
	}

	entry, ok := manifest.For(runtime.GOOS, runtime.GOARCH)
	if !ok || !entry.Published {
		return "", &service.Error{
			Code:    ffmpeg.CodeNoEncoder,
			Message: fmt.Sprintf("there is no OpenH264 library to install for %s/%s.", runtime.GOOS, runtime.GOARCH),
			Hint:    "use the hardware or operating system encoder of this machine",
		}
	}

	host := ""
	if u, perr := url.Parse(entry.URL); perr == nil {
		host = u.Hostname()
	}

	return fmt.Sprintf("1 file, %.1f MB from %s", float64(entry.Bytes)/1e6, host), nil
}
