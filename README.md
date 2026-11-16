<div align="center">

<a href="https://video.vukem.dev" target="_blank" rel="noopener noreferrer">
  <img src="site/public/logo.svg" alt="Vukem Video" width="96" height="96">
</a>

<br/>

<a href="https://github.com/AlexGalhardo/vukem-video/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/AlexGalhardo/vukem-video/ci.yml?branch=main&label=tests" alt="Tests"></a>
<a href="https://github.com/AlexGalhardo/vukem-video/actions/workflows/release.yml"><img src="https://img.shields.io/github/actions/workflow/status/AlexGalhardo/vukem-video/release.yml?label=ci%2Fcd" alt="CI/CD"></a>
<a href="https://github.com/AlexGalhardo/vukem-video/releases"><img src="https://img.shields.io/github/v/release/AlexGalhardo/vukem-video?include_prereleases&label=version" alt="Version"></a>
<a href="go.mod"><img src="https://img.shields.io/badge/go-1.27.4-00ADD8?logo=go&logoColor=white" alt="Go 1.27.4"></a>
<a href="LICENSE.md"><img src="https://img.shields.io/github/license/AlexGalhardo/vukem-video" alt="License"></a>
<a href="https://scorecard.dev/viewer/?uri=github.com/AlexGalhardo/vukem-video"><img src="https://api.scorecard.dev/projects/github.com/AlexGalhardo/vukem-video/badge" alt="OpenSSF Scorecard"></a>

### Vukem Video is a minimalist, open source video editor, web first and desktop, built to be operated by AI agents and by people through one command core

<br/>

</div>

<br/>

| Area | What you get |
| --- | --- |
| **Documentation site** | <a href="https://video.vukem.dev">https://video.vukem.dev</a> |
| **Two use cases** | Short vertical videos with captions for social networks<br>Product commercials |
| **Interfaces** | Editor in the browser<br>MCP server (`vukem-video mcp`)<br>CLI with `--json`<br>HTTP API (`vukem-video serve`)<br>Desktop application |
| **Editing** | Captions from a transcript, with editable words and styles<br>Cuts, trims, splits, ripple, roll, slide and slip, and silence removal<br>Chroma key, blend modes, filters, color and blur per clip<br>Framing, speed, transitions and keyframes<br>Re-target a project to 16:9, 9:16, 1:1 or 4:5 in one command<br>Text, shapes and markers<br>Audio ducking and beat marks<br>Templates and brand kit |
| **Export** | MP4, GIF, WAV and AAC<br>A full-resolution PNG of one frame<br>A range of the timeline<br>OpenTimelineIO export and import |
| **For agents** | Agent docs generated from the code: `SKILL.md`, an `AGENTS.md` snippet and the MCP instructions<br>`command_help` lists every operation with arguments and an example<br>`lint` finds problems before export<br>Batches of operations are applied as one revision |
| **Local and private** | No outbound network connection by default and no telemetry<br>The server listens on `127.0.0.1`<br>FFmpeg only as a separate LGPL process |

## How it is built

- One command core (`core/`, Go, pure, no I/O) holds every editing rule.
- The UI, the MCP server, the CLI and the HTTP API are thin adapters over one service, so a person and an agent see the same result.
- The UI never computes the result of an edit: it draws what the core says and sends commands.
- The layout is `core/`, `engine/`, `service/`, `web/`, `site/` and `desktop/` ([ADR-0001](docs/adrs/0001-repository-layout-and-go-module.md)).

## Quick start

Run the container image of the latest release. The port is published on your computer only:

```bash
docker run --rm -p 127.0.0.1:7878:7878 -v vukem-video:/data \
  -e APP_KEY="$(openssl rand -base64 32)" \
  -e VUKEM_VIDEO_ADMIN_EMAIL=admin@example.com \
  -e VUKEM_VIDEO_ADMIN_PASSWORD=choose-a-long-password \
  ghcr.io/alexgalhardo/vukem-video:latest
```

Then open `http://127.0.0.1:7878` in a browser and sign in with that email and password. The two variables create the first administrator on the first start only; later starts ignore them.

Installers for Windows, Linux and macOS and the desktop application are on the [releases page](https://github.com/AlexGalhardo/vukem-video/releases). Read [Installation](https://video.vukem.dev/docs/getting-started/installation/) for every option and how to verify a download.

## Agentic development

Vukem Video is built for AI agents as first-class users. Add the stdio MCP server to your client:

```sh
vukem-video mcp
```

Or use the CLI with `--json`:

```sh
vukem-video project:create --name "Launch teaser" --json
```

Or call the HTTP API of `vukem-video serve` (default `127.0.0.1:7878`, `POST /mcp` for Streamable HTTP MCP). The workspace is `~/.vukem-video`, or `$VUKEM_VIDEO_HOME`.

## Stability

Versions follow SemVer. Before 1.0 a minor release may break. From `v1.0.0` the MCP tools, the CLI, the HTTP API, the timeline document schema and the workspace layout only break in a major release. See the [changelog](CHANGELOG.md).

## Contributing

To build from source you need Go (see `go.mod`) and Bun for `web/`. The task runner is [Task](https://taskfile.dev): `task lint`, `task test` and `task ci`. Read the [contribution guide](.github/CONTRIBUTING.md) to get started. The planning documents (requirements, decisions and specifications) are in [docs/](docs/README.md).

## Code of conduct

To keep the community welcoming to everyone, please review and follow the [Code of Conduct](.github/CODE_OF_CONDUCT.md).

## Security vulnerabilities

If you discover a security vulnerability, please follow the [security policy](.github/SECURITY.md) instead of opening a public issue.

## License

Vukem Video is open-source software licensed under the [MIT license](LICENSE.md).
