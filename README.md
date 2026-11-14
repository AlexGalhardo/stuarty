# Vukem Video

A minimalist, open source (MIT) video editor, web first and desktop later, built to be operated by AI agents as first-class users.

An agent edits through MCP, a CLI or an HTTP API. A person edits the same project in a browser UI. Both go through one command core written in Go, so a human and an agent see the same result. Two use cases drive every priority: short vertical videos with captions for social networks, and product commercials.

## Status

**Prototype phase: the product is planned but not built yet.** The repository holds the Go module and package skeleton, the planning documents and a server that boots. The editor, the MCP server, the CLI commands and the HTTP API described below are specified, not released. There is no release and no installer yet, and the documentation site is not live.

## How it is built

- One command core (`core/`, Go, pure, no I/O) holds every editing rule. The UI, the MCP server, the CLI and the HTTP API are thin adapters over one service.
- The UI never computes the result of an edit: it draws what the core says and sends commands.
- FFmpeg is used only as a separate LGPL process; pixels are composed in Go.
- No outbound network connection by default and no telemetry.

The layout is `core/`, `engine/`, `service/`, `web/`, `site/` and `desktop/` (`site/` and the contents of `desktop/` come later); the decision is [ADR-0001](docs/adrs/0001-repository-layout-and-go-module.md).

## Quick start

Both paths below describe the planned interface of the Prototype phase ([SDD-003](docs/sdds/003-agent-interfaces.md)). They do not work until the first release.

### For an agent

1. Add the stdio MCP server to your client: the command is `vukem-video mcp`. It owns the workspace (default `~/.vukem-video`, or `$VUKEM_VIDEO_HOME`) when no server runs.
2. Or use the CLI with `--json`, for example `vukem-video project:create --name "Launch teaser" --json`.
3. Or call the HTTP API of `vukem-video serve` (default `127.0.0.1:7878`, `POST /mcp` for Streamable HTTP MCP).

### For a person

1. Run `vukem-video serve`.
2. Run `vukem-video open`, or open `http://127.0.0.1:7878` in a browser.

### Contributors

To build from source you need Go (see `go.mod`) and Bun for `web/`. The task runner (`task lint`, `task test`, `task ci`) arrives with a later task; until then use `go build ./...` and `go test ./...`. Read [CONTRIBUTING](.github/CONTRIBUTING.md) first.

## Documentation

- [Planning documents](docs/README.md): requirements, architecture decisions and specifications.
- The public `AGENTS.md` for contributors is added by a later task; until then [CONTRIBUTING](.github/CONTRIBUTING.md) is the entry point.
- [Security policy](.github/SECURITY.md) and [Code of Conduct](.github/CODE_OF_CONDUCT.md).
- [Changelog](CHANGELOG.md).

The documentation site is planned (SDD-010) and has no address yet.

## License

[MIT](LICENSE.md), Copyright (c) 2026 Alex Galhardo.
