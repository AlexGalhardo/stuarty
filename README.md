<div align="center">

<a href="https://github.com/AlexGalhardo/stuarty" target="_blank" rel="noopener noreferrer">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="site/public/logo-wordmark-dark.png">
    <img src="site/public/logo-wordmark-light.png" alt="Stuarty" height="72">
  </picture>
</a>

<br/>

<a href="https://github.com/AlexGalhardo/stuarty/releases"><img src="https://img.shields.io/github/v/release/AlexGalhardo/stuarty?include_prereleases&label=version" alt="Version"></a>
<a href="go.mod"><img src="https://img.shields.io/badge/go-1.27.4-00ADD8?logo=go&logoColor=white" alt="Go 1.27.4"></a>
<a href="LICENSE.md"><img src="https://img.shields.io/github/license/AlexGalhardo/stuarty" alt="License"></a>

## Create, edit, manage and publish your audiovisual and text content in one place

<br/>

</div>

<br/>

## Introduction

Why use so many tools to create your content, when you can focus on one tool and connect to other tools and services when needed? This is why Stuarty exists.

Stuarty is an open source (MIT), self-hosted studio for independent creators, on the web and on the desktop. It is built around the problems that eat a creator's week:

- **Your work is scattered across five apps and five subscriptions.** The script lives in one app, the cut in another, the thumbnail in a third and the schedule in a fourth. In Stuarty, one project holds everything:
  - [Dashboard and projects](https://video.vukem.dev/docs/guide/dashboard/): every production in one place, from the briefing to the published post.
  - [Video editor](https://video.vukem.dev/docs/guide/timeline/): a multi-track timeline with [text and captions](https://video.vukem.dev/docs/guide/text-and-captions/), [transitions](https://video.vukem.dev/docs/guide/transitions/), [keyframes](https://video.vukem.dev/docs/guide/keyframes/), [effects and colour](https://video.vukem.dev/docs/guide/effects-and-color/), [audio](https://video.vukem.dev/docs/guide/audio/) and a [verified export](https://video.vukem.dev/docs/guide/exporting/).
  - [Image editor](https://video.vukem.dev/docs/guide/image-editor/): layers, adjustments, looks and lookup tables for thumbnails, covers and posters.
  - [Writing](https://video.vukem.dev/docs/guide/writing/) and [blog posts](https://video.vukem.dev/docs/guide/blog-posts/): scripts, descriptions and articles, published to your own blog.
  - [Media library](https://video.vukem.dev/docs/guide/media-library/) and [asset library](https://video.vukem.dev/docs/guide/asset-library/): your footage, images and audio, found again in seconds.
- **Turning a long recording into short vertical videos takes hours of manual work.** Stuarty does the repetitive part for you:
  - [Transcription](https://video.vukem.dev/docs/guide/transcription/) and automatic captions, word by word.
  - [Silence removal](https://video.vukem.dev/docs/guide/silence-removal/) for talking-head recordings.
  - The [vertical cut template](https://video.vukem.dev/docs/guide/template-vertical-cut/), the [safe area](https://video.vukem.dev/docs/guide/safe-area/) of each network, and the recipe for a [long talking video](https://video.vukem.dev/docs/guide/recipe-long-talking-video/).
  - A step-by-step tutorial: [your first vertical short](https://video.vukem.dev/docs/guide/tutorial-vertical-short/).
- **Every video, thumbnail and post looks a little different.** Keep a consistent look without starting from zero:
  - The [brand kit](https://video.vukem.dev/docs/guide/brand-kit/): your colours, fonts and logos in every project.
  - [Templates](https://video.vukem.dev/docs/guide/templates/) such as the [quote card](https://video.vukem.dev/docs/guide/template-quote-card/) and the [product teaser](https://video.vukem.dev/docs/guide/template-product-teaser/), and the recipe for a [product commercial](https://video.vukem.dev/docs/guide/recipe-product-commercial/).
- **Publishing to every network by hand, at the right time, is a job of its own.** Plan once, publish everywhere:
  - The [publishing calendar](https://video.vukem.dev/docs/guide/publishing-calendar/): schedule posts to YouTube, Instagram, Facebook, X and TikTok, through your own developer apps.
  - An [approval queue](https://video.vukem.dev/docs/guide/approvals/): nothing is published, and nothing costly is generated, until a person approves it.
- **AI tools lock you into their cloud and charge you per seat.** Bring your own access instead:
  - [AI providers](https://video.vukem.dev/docs/guide/ai-providers/): your own API key, Sign in with ChatGPT, OpenRouter or the agent you already run locally.
  - The [agent chat](https://video.vukem.dev/docs/guide/agent-chat/) inside the dashboard, with [memory, skills and prompts](https://video.vukem.dev/docs/guide/memory-skills-prompts/) that stay as plain files you own.
- **Your AI agent cannot drive the tools you use.** In Stuarty, agents are first-class users: everything you do in the UI they can do too, with tested parity across three surfaces, and every change they make is attributed to them:
  - [Agents overview](https://video.vukem.dev/docs/agents/overview/) and a tutorial: [edit a video with an agent](https://video.vukem.dev/docs/guide/tutorial-agent-edit/).
  - The [CLI](https://video.vukem.dev/docs/agents/cli/), the [HTTP API](https://video.vukem.dev/docs/api/overview/) and the [MCP server](https://video.vukem.dev/docs/mcp/overview/).
  - [Safety](https://video.vukem.dev/docs/agents/safety/): agents prepare, people approve.
- **Your files, tokens and credentials live in someone else's cloud.** Stuarty runs on your machine or on your own server:
  - [Installation](https://video.vukem.dev/docs/installation/) as a [desktop app](https://video.vukem.dev/docs/guide/desktop/), a [single binary](https://video.vukem.dev/docs/self-hosting/binary/) or with [Docker](https://video.vukem.dev/docs/self-hosting/docker/) on a [VPS](https://video.vukem.dev/docs/self-hosting/vps/).
  - [What leaves your machine](https://video.vukem.dev/docs/what-leaves-your-machine/): nothing, until you connect a service; no telemetry.
  - Credentials in a local encrypted vault ([security](https://video.vukem.dev/docs/self-hosting/security/), [privacy](https://video.vukem.dev/docs/self-hosting/privacy/)), and [backups and upgrades](https://video.vukem.dev/docs/self-hosting/backups-and-upgrades/) you control.
- **One wrong click and an hour of editing is gone.** Every change is a revision:
  - [Undo and history](https://video.vukem.dev/docs/guide/undo-and-history/): step back, revert to any revision, see who changed what, you or an agent.

New here? Start with the [introduction](https://video.vukem.dev/docs/introduction/) and the [quick start](https://video.vukem.dev/docs/quick-start/).

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
