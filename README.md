# Vukem Video

A minimalist, open source (MIT) video editor, web first and desktop later, built to be operated by AI agents as first-class users.

An agent edits through MCP, a CLI or an HTTP API. A person edits the same project in a browser UI. Both go through one command core written in Go, so a human and an agent see the same result. Two use cases drive every priority: short vertical videos with captions for social networks, and product commercials.

## Status

**Prototype phase: the product is planned but not built yet.** The repository holds the Go module and package skeleton, the planning documents and a server that boots. The editor, the MCP server, the CLI commands and the HTTP API described below are specified and being built. The first editor release is `v0.2.0`, with installers and a container image of the local server; it does not edit video yet. The documentation site is live at <https://alexgalhardo.github.io/vukem-video/>.

## How it is built

- One command core (`core/`, Go, pure, no I/O) holds every editing rule. The UI, the MCP server, the CLI and the HTTP API are thin adapters over one service.
- The UI never computes the result of an edit: it draws what the core says and sends commands.
- FFmpeg is used only as a separate LGPL process; pixels are composed in Go.
- No outbound network connection by default and no telemetry.

The layout is `core/`, `engine/`, `service/`, `web/`, `site/` and `desktop/` (the contents of `desktop/` come later); the decision is [ADR-0001](docs/adrs/0001-repository-layout-and-go-module.md).

## Quick start

The two paths below use the interface of the Prototype phase ([SDD-003](docs/sdds/003-agent-interfaces.md)); the editor, the MCP server and the CLI commands are still being built, so parts of them are specified but not working yet.

### Install

Each release on the [releases page](https://github.com/AlexGalhardo/vukem-video/releases) attaches archives for six targets, installers of the local server and `checksums.txt`, signed with cosign. The installers put `vukem-video` on your system and add a shortcut that starts the local server on `127.0.0.1` and opens the editor in your browser. Uninstalling removes the program and keeps your workspace.

| System | File |
| --- | --- |
| Windows 11 | `vukem-video_<version>_windows_amd64_setup.exe` |
| Debian and Ubuntu | `vukem-video_<version>_linux_<arch>.deb` |
| Any Linux | `vukem-video_<version>_linux_amd64.AppImage` |
| macOS | `vukem-video_<version>_darwin_<arch>.pkg` |

The container image runs the web version on your machine only; the published port is bound to the loopback of the host:

```bash
docker run --rm -p 127.0.0.1:7878:7878 -v vukem-video:/data \
  -e APP_KEY="$(openssl rand -base64 32)" ghcr.io/alexgalhardo/vukem-video:<version>
```

**Windows SmartScreen and macOS Gatekeeper.** The installers are not signed with a paid code-signing certificate, so both systems warn before the first run. Windows SmartScreen shows "Windows protected your PC" for the `.exe`: choose "More info", then "Run anyway". macOS Gatekeeper says the package cannot be opened because Apple cannot check it: control-click the `.pkg` and choose Open, or allow it in System Settings, Privacy & Security. To check a download before you run it, verify `checksums.txt` with cosign and compare the SHA-256 of the file, as described in the [security policy](.github/SECURITY.md).

### For an agent

1. Add the stdio MCP server to your client: the command is `vukem-video mcp`. It owns the workspace (default `~/.vukem-video`, or `$VUKEM_VIDEO_HOME`) when no server runs.
2. Or use the CLI with `--json`, for example `vukem-video project:create --name "Launch teaser" --json`.
3. Or call the HTTP API of `vukem-video serve` (default `127.0.0.1:7878`, `POST /mcp` for Streamable HTTP MCP).

### For a person

1. Run `vukem-video serve`.
2. Run `vukem-video open`, or open `http://127.0.0.1:7878` in a browser.

### Contributors

To build from source you need Go (see `go.mod`) and Bun for `web/`. The task runner is [Task](https://taskfile.dev): `task lint`, `task test` and `task ci`. Read [CONTRIBUTING](.github/CONTRIBUTING.md) first.

## Documentation

- [Planning documents](docs/README.md): requirements, architecture decisions and specifications.
- The public `AGENTS.md` for contributors is added by a later task; until then [CONTRIBUTING](.github/CONTRIBUTING.md) is the entry point.
- [Security policy](.github/SECURITY.md) and [Code of Conduct](.github/CODE_OF_CONDUCT.md).
- [Changelog](CHANGELOG.md).

The documentation site (SDD-010) is at <https://alexgalhardo.github.io/vukem-video/>, with the documentation under <https://alexgalhardo.github.io/vukem-video/docs/introduction/>.

## License

[MIT](LICENSE.md), Copyright (c) 2026 Alex Galhardo.
