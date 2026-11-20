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
  - [Dashboard and projects](site/content/docs/en/guide/dashboard.mdx): every production in one place, from the briefing to the published post.
  - [Video editor](site/content/docs/en/guide/timeline.mdx): a multi-track timeline with [text and captions](site/content/docs/en/guide/text-and-captions.mdx), [transitions](site/content/docs/en/guide/transitions.mdx), [keyframes](site/content/docs/en/guide/keyframes.mdx), [effects and colour](site/content/docs/en/guide/effects-and-color.mdx), [audio](site/content/docs/en/guide/audio.mdx) and a [verified export](site/content/docs/en/guide/exporting.mdx).
  - [Image editor](site/content/docs/en/guide/image-editor.mdx): layers, adjustments, looks and lookup tables for thumbnails, covers and posters.
  - [Writing](site/content/docs/en/guide/writing.mdx) and [blog posts](site/content/docs/en/guide/blog-posts.mdx): scripts, descriptions and articles, published to your own blog.
  - [Media library](site/content/docs/en/guide/media-library.mdx) and [asset library](site/content/docs/en/guide/asset-library.mdx): your footage, images and audio, found again in seconds.
- **Turning a long recording into short vertical videos takes hours of manual work.** Stuarty does the repetitive part for you:
  - [Transcription](site/content/docs/en/guide/transcription.mdx) and automatic captions, word by word.
  - [Silence removal](site/content/docs/en/guide/silence-removal.mdx) for talking-head recordings.
  - The [vertical cut template](site/content/docs/en/guide/template-vertical-cut.mdx), the [safe area](site/content/docs/en/guide/safe-area.mdx) of each network, and the recipe for a [long talking video](site/content/docs/en/guide/recipe-long-talking-video.mdx).
  - A step-by-step tutorial: [your first vertical short](site/content/docs/en/guide/tutorial-vertical-short.mdx).
- **Every video, thumbnail and post looks a little different.** Keep a consistent look without starting from zero:
  - The [brand kit](site/content/docs/en/guide/brand-kit.mdx): your colours, fonts and logos in every project.
  - [Templates](site/content/docs/en/guide/templates.mdx) such as the [quote card](site/content/docs/en/guide/template-quote-card.mdx) and the [product teaser](site/content/docs/en/guide/template-product-teaser.mdx), and the recipe for a [product commercial](site/content/docs/en/guide/recipe-product-commercial.mdx).
- **Publishing to every network by hand, at the right time, is a job of its own.** Plan once, publish everywhere:
  - The [publishing calendar](site/content/docs/en/guide/publishing-calendar.mdx): schedule posts to YouTube, Instagram, Facebook, X and TikTok, through your own developer apps.
  - An [approval queue](site/content/docs/en/guide/approvals.mdx): nothing is published, and nothing costly is generated, until a person approves it.
- **AI tools lock you into their cloud and charge you per seat.** Bring your own access instead:
  - [AI providers](site/content/docs/en/guide/ai-providers.mdx): your own API key, Sign in with ChatGPT, OpenRouter or the agent you already run locally.
  - The [agent chat](site/content/docs/en/guide/agent-chat.mdx) inside the dashboard, with [memory, skills and prompts](site/content/docs/en/guide/memory-skills-prompts.mdx) that stay as plain files you own.
- **Your AI agent cannot drive the tools you use.** In Stuarty, agents are first-class users: everything you do in the UI they can do too, with tested parity across three surfaces, and every change they make is attributed to them:
  - [Agents overview](site/content/docs/en/agents/overview.mdx) and a tutorial: [edit a video with an agent](site/content/docs/en/guide/tutorial-agent-edit.mdx).
  - The [CLI](site/content/docs/en/agents/cli.mdx), the [HTTP API](site/content/docs/en/api/overview.mdx) and the [MCP server](site/content/docs/en/mcp/overview.mdx).
  - [Safety](site/content/docs/en/agents/safety.mdx): agents prepare, people approve.
- **Your files, tokens and credentials live in someone else's cloud.** Stuarty runs on your machine or on your own server:
  - [Installation](site/content/docs/en/getting-started/installation.mdx) as a [desktop app](site/content/docs/en/guide/desktop.mdx), a [single binary](site/content/docs/en/self-hosting/binary.mdx) or with [Docker](site/content/docs/en/self-hosting/docker.mdx) on a [VPS](site/content/docs/en/self-hosting/vps.mdx).
  - [What leaves your machine](site/content/docs/en/getting-started/what-leaves-your-machine.mdx): nothing, until you connect a service; no telemetry.
  - Credentials in a local encrypted vault ([security](site/content/docs/en/self-hosting/security.mdx), [privacy](site/content/docs/en/self-hosting/privacy.mdx)), and [backups and upgrades](site/content/docs/en/self-hosting/backups-and-upgrades.mdx) you control.
- **One wrong click and an hour of editing is gone.** Every change is a revision:
  - [Undo and history](site/content/docs/en/guide/undo-and-history.mdx): step back, revert to any revision, see who changed what, you or an agent.

New here? Start with the [introduction](site/content/docs/en/getting-started/introduction.mdx) and the [quick start](site/content/docs/en/getting-started/quick-start.mdx).

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
