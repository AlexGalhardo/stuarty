<div align="center">

<a href="https://github.com/AlexGalhardo/stuarty" target="_blank" rel="noopener noreferrer">
  <img src="site/public/logo.svg" alt="Stuarty" width="96" height="96">
</a>

<br/>

<a href="https://github.com/AlexGalhardo/stuarty/releases"><img src="https://img.shields.io/github/v/release/AlexGalhardo/stuarty?include_prereleases&label=version" alt="Version"></a>
<a href="go.mod"><img src="https://img.shields.io/badge/go-1.27.4-00ADD8?logo=go&logoColor=white" alt="Go 1.27.4"></a>
<a href="LICENSE.md"><img src="https://img.shields.io/github/license/AlexGalhardo/stuarty" alt="License"></a>

### Stuarty: create, edit, manage and publish your content in one place

<br/>

</div>

<br/>

Stuarty is an open source (MIT), self-hosted studio for people who make content for the Internet, built so that AI agents are first-class users: everything a person does in the UI can be done through the HTTP API, the CLI and MCP. Files, tokens and credentials stay on your machine or on your own server.

It grew out of a video editor called Vukem Video. The editor is what works today; the rest is being built, epic by epic, from [`docs/plans/`](docs/plans/00-roadmap.md). The code still uses the former name (`vukem-video`) until the rename described in [ADR-0044](docs/adrs/0044-rename-to-stuarty.md).

## What works today: the video editor

| Area | What you get |
| --- | --- |
| **Interfaces** | Editor in the browser<br>MCP server (`vukem-video mcp`)<br>CLI with `--json`<br>HTTP API (`vukem-video serve`)<br>Desktop application (a native window over the same web UI) |
| **Editing** | Captions from a transcript, with editable words and styles<br>Cuts, trims, splits, ripple, roll, slide and slip, and silence removal<br>Chroma key, blend modes, masks, filters, effects, colour and blur per clip<br>Framing, speed curves, transitions and keyframes<br>Re-target a project to 16:9, 9:16, 1:1 or 4:5 in one command<br>Text, shapes and markers<br>Audio ducking and beat marks<br>Templates and brand kit |
| **Export** | MP4, GIF, WAV and AAC<br>A full-resolution PNG of one frame<br>A range of the timeline<br>OpenTimelineIO export and import |
| **For agents** | Agent docs generated from the code: `SKILL.md`, an `AGENTS.md` snippet and the MCP instructions<br>`command_help` lists every operation with arguments and an example<br>`lint` finds problems before export<br>Batches of operations are applied as one revision |
| **Local and private** | No telemetry<br>The server listens on `127.0.0.1`<br>FFmpeg only as a separate LGPL process |

## What is being built

| Area | Plan |
| --- | --- |
| **Projects** | A project holds a whole production: briefing, scripts, assets, timelines, images, exports and publications |
| **Dashboard** | One home with a sidebar: Projects, Edit Video, Edit Image, Write, Visual Assets, Video Assets, Publish, Skills, Prompts, Memory, Connections |
| **Image editor** | Layers, text, shapes, adjustments, masks and templates on the same core and the same renderer as the video editor; thumbnails first |
| **Writing** | Video scripts, commercial copy and blog posts, with your briefing, your brand and your memory as context |
| **AI** | Bring your own: an API key, Sign in with ChatGPT, OpenRouter, or the agent you already have installed. Image, voice and video generation with the cost shown first |
| **Agent chat** | Your own agent (Claude Code, Codex, OpenCode) hosted in a chat panel, working on the open project |
| **Publishing** | Schedule and publish to YouTube, Instagram and Facebook first, then X and TikTok drafts; blog posts to files, WordPress, a webhook or the API. Nothing is published before you approve it |
| **Memory, skills, prompts** | Markdown files in your workspace that you and any agent can read |
| **Self-hosting** | A desktop application and a Docker installation for a VPS, with every credential in an encrypted vault |

The requirements are in [`docs/prds/`](docs/prds), the decisions in [`docs/adrs/`](docs/adrs) and the order of the work in [`docs/plans/00-roadmap.md`](docs/plans/00-roadmap.md).

## How it is built

- One command core (`core/`, Go, pure, no I/O) holds every rule of every domain.
- The UI, the MCP server, the CLI and the HTTP API are thin adapters over one service, so a person and an agent see the same result.
- The UI never computes the result of an edit: it draws what the core says and sends commands.
- One renderer (`renderer/`, Rust compiled to WebAssembly) draws the pixels in the browser and in the exported file.
- Data leaves the machine only towards a destination you connected, and publishing waits for your approval ([ADR-0032](docs/adrs/0032-controlled-outbound-and-human-approval.md)).
- The layout is `core/`, `engine/`, `renderer/`, `service/`, `web/`, `site/` and `desktop/` ([ADR-0001](docs/adrs/0001-repository-layout-and-go-module.md)).

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

Installers for Windows, Linux and macOS and the desktop application are on the [releases page](https://github.com/AlexGalhardo/stuarty/releases). The documentation site is <https://video.vukem.dev>; it moves with the rename.

## Working with an agent

Add the stdio MCP server to your client:

```sh
vukem-video mcp
```

Or use the CLI with `--json`:

```sh
vukem-video project:create --name "Launch teaser" --json
```

Or call the HTTP API of `vukem-video serve` (default `127.0.0.1:7878`, `POST /mcp` for Streamable HTTP MCP). The workspace is `~/.vukem-video`, or `$VUKEM_VIDEO_HOME`.

## Stability

Versions follow SemVer. Stuarty is in `0.x`: a minor release may break. A project or a workspace saved by a released version opens in every later one. See the [changelog](CHANGELOG.md).

## Contributing

To build from source you need Go (see `go.mod`) and Bun for `web/`. The task runner is [Task](https://taskfile.dev): `task lint` and `task test`. Read the [contribution guide](.github/CONTRIBUTING.md) to get started and [`AGENTS.md`](AGENTS.md) for how the team of agents works. The planning documents (requirements, decisions, specifications and epics) are in [docs/](docs/README.md).

## Code of conduct

To keep the community welcoming to everyone, please review and follow the [Code of Conduct](.github/CODE_OF_CONDUCT.md).

## Security vulnerabilities

If you discover a security vulnerability, please follow the [security policy](.github/SECURITY.md) instead of opening a public issue.

## License

Stuarty is open-source software licensed under the [MIT license](LICENSE.md).
