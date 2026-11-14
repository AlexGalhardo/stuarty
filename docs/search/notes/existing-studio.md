# Raw notes: existing Studio app in `web/`

> **Note (2026-11-14).** Paths in this file written as `C:\dev\vukem-video\web` now mean `C:\dev\vukem-video\video\web`, after the workspace was reorganized. The decision on this code is D-002: the editor replaces the Studio and reuses the pieces listed below.

Read by the architect on 2026-11-14, straight from the code in `C:\dev\vukem-video\web` (local copy of the `main` of `AlexGalhardo/vukem-video`, commit `4461c61`). Everything here is FACT verified in the cited file, unless marked otherwise.

## What it is

- A DIFFERENT product from the editor we are planning: a generator "brief → prompt → scene plan (LLM) → FFmpeg → MP4" with automatic quality checking. It has no timeline, no manual editing, and no MCP. Source: `web/README.md`, `web/prompt.md`, `web/ROADMAP.md`.
- vukem app `v0.43.0`, Go `1.27.4`, SQLite via `modernc.org/sqlite` (pure Go, indirect from vukem). Source: `web/go.mod`.
- Frontend: React `19.3.1`, `@inertiajs/react` `3.9.1`, Vite `8.3.4`, Tailwind `4.3.3`, shadcn `4.23.0`, radix-ui `1.8.1`, TypeScript `7.0.2`, Biome `2.6.6`, Playwright `1.64.1`, Bun `1.4.4`. Source: `web/package.json`.
- Render code: `web/app/studio/` with 2,848 lines of Go (including tests). Largest: `audio.go` (374), `run.go` (364), `render.go` (350), `qa.go` (295).

## What is reusable for the editor (INFERENCE of the architect)

| Piece | Where | Reuse |
| --- | --- | --- |
| FFmpeg call via `os/exec` with context and timeout | `app/studio/exec.go`, `render.go` | Direct pattern for export; confirms the author already uses FFmpeg as a separate process, without cgo |
| ffprobe inspection + `faststart` check + duration | `app/studio/qa.go` (`Inspect`, `FastStart`, `MediaDuration`, `CheckFile`) | Basis of the "export lint" and of verifiable feedback for agents |
| Loudness (`loudnorm=print_format=json`), target -14 LUFS / -1.5 dBTP | `app/studio/render.go:255`, `docs/web-research.md` | Audio project rule already decided in Studio |
| Burned-in captions with libass (`subtitles=...:force_style=`) | `app/studio/render.go:69`, `captions.go` | Caption path already proven; font `DejaVu Sans` installed in the image |
| vukem job queue in the database (`QUEUE_CONNECTION=database`, `queue:work`) | `Dockerfile.app`, `app/jobs/` | Indicates that vukem ALREADY ships a database queue: river/asynq from the stack draft may be unnecessary (confirm in `notes/vukem.md`) |
| Go migrations of vukem | `database/migrations/*.go` | goose from the draft may be unnecessary (confirm) |
| Job limits and budget, render slots | `app/jobs/limits.go`, `budget.go`, migrations `add_job_limits`, `add_render_slots` | Per-account resource limits model already exists |
| CI, hooks, `cliff.toml`, Astro docs site, Dockerfiles, Railway | `.github/workflows/`, `.githooks/`, `site/` | vukem conventions applied; they move to the root in the monorepo |

## Conflicts with the editor brief

1. **GPL encoder already in use.** `app/studio/render.go:81` encodes with `-c:v libx264`, and `Dockerfile.app` installs `ffmpeg` from Alpine (`apk add ffmpeg`). The editor brief anticipates an "LGPL build, avoiding x264/x265". INFERENCE: the Docker image is published (GHCR + Docker Hub, by the author's convention), and publishing an image with a GPL binary is distribution, not just server-side use. The codec policy must also apply to the image. Check in `notes/licensing.md`.
2. **Remotion.** `web/prompt.md` lists Remotion as the Studio's primary tool; `docs/web-research.md` records that Remotion's license requires a company license for companies with more than three people (October 2026 values cited there: about US$ 25 per seat per month, or US$ 0.01 per render with a minimum of US$ 100 per month) and points to HyperFrames (HeyGen, Apache-2.0) as a permissive alternative. For an MIT product, Remotion as a core dependency is a license risk already recognized by the author.
3. **Language.** The Studio keeps the repository in English ("Keep the repository in English", `web/README.md`); the editor brief asks for `docs/` in pt-BR. Confirm in the interview.
4. **`vukem-feedback.md`.** In the Studio it is local and git-ignored (`web/prompt.md`); the editor brief wants it at the root, continuously fed. Confirm whether it is versioned.

## Possible vukem friction (to confirm in the vukem code before it becomes a VF entry)

- `Dockerfile.app` comments: "The app owns a `jobs` table (render jobs), so the vukem database queue uses queue_jobs/queue_failed_jobs". This suggests that vukem's default queue table is named `jobs` and collides with a very common domain table name.
