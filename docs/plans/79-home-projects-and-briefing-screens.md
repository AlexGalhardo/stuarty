# 79: Home, projects, briefing and the workflow screens

## Goal

A beginner opens Stuarty, says what they want to create, a text or a video, and always sees the next step until the publication.

## Context and related decisions

- Author decisions D-134 and D-137. Frames 01, 02a to 02c, 19 (brand kit), 22 and 33a to 33c of the final redesign file held by the author, with their state frames.
- Code: `web/resources/js/pages/Dashboard/Home.tsx`, `web/resources/js/pages/Projects/`, `web/resources/js/pages/Settings/BrandKit.tsx`.

## User stories

- As a creator, I want to choose text, video, image or later as my first decision so that no format is forced on me.
- As a creator, I want a text project to look complete without a thumbnail.
- As a creator, I want to see each step with who does it and what it costs.

## Scope

- Modules: `web/resources/js/pages/Dashboard/`, `web/resources/js/pages/Projects/`, `web/resources/js/pages/Settings/BrandKit.tsx`, new workflow components in `web/resources/js/workflow/`, their language keys.
- Must not touch: `core/`, `service/`, the shell, other pages.

## Out of scope

New project fields beyond epic 78, and the pages of Write and Publish.

## Dependencies

74, 75 and 78.

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | Home, Projects, New project, Briefing, Workflow, Brand kit |
| HTTP, CLI, MCP | None |

## Core commands and permissions

None new. It uses `ProjectCreate`, `ProjectGet`, `ProjectList` and `ProjectStepSkip`.

## Technical design

- **New project** asks what to create in four equal cards. Format appears only for video. A text asks for its type: post, copy, script or caption.
- **The briefing** collects the real fields of `ProjectCreate`: a description of 64 to 1024 characters, the five parts of the goal and the due date. The dialog says "Continue to the briefing", because the project is created at its end.
- **`TrailStep` and `TrailDetail`** draw the server's workflow and compute nothing.
- **Home** shows the project in progress with its trail, the sections and what waits for the person.
- **`ProjectCardText`** shows excerpt, type, words and reading time. A text project offers no export.
- **Empty states** teach in three steps and never assume video.

## Tasks

- [x] 1. Workflow components over the server's steps. Verify: `cd web && bun run test -- workflow` passes.
- [x] 2. Home and its states. Verify: new spec `home-trail` passes.
- [x] 3. Projects list with both card types. Verify: `cd web && bun run test -- Projects` passes.
- [x] 4. New project with the four kinds. Verify: new spec `new-project-kinds` passes, and a text project creates no timeline.
- [x] 5. Briefing for each kind, with errors under each field. Verify: `cd web && bun run test -- Briefing` passes.
- [x] 6. The project detail with the workflow, skip and "Add media". Verify: new spec `project-workflow` passes.
- [x] 7. Brand kit on the new components. Verify: `cd web && bun run test -- BrandKit` passes.
- [ ] 8. Check 1440, 1280 and 390 px in both themes with axe. Verify: `cd web && bun run e2e -- a11y` passes.

## Test strategy

Component tests from fixed workflow payloads, one browser spec of a text project and one of a video project.

## Acceptance criteria

1. **Given** a person choosing Text, **when** they finish the briefing, **then** the project exists with no format and no timeline asked for.
2. **Given** a text project, **when** its card shows, **then** it has excerpt, type, words and reading time and no empty thumbnail.
3. **Given** a workflow, **when** rendered, **then** every step shows its actor, tool, state and cost, and an unknown value reads "Not reported".
4. **Given** a text project, **when** "Add media" adds a video, **then** the new steps appear with no reload of the finished ones.
5. **Given** an empty workspace, **when** Home opens, **then** it teaches the first step and does not mention only video.
6. `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n`, `bun run lint` and `task test:web` pass.

## Risks and mitigation

- Logic could leak into the UI: the step state comes only from the server.
- **STOP** if a frame needs a field that epic 78 does not return.

## Size

L: the first screens a person sees, with one new flow. About 8 commits.

## Priority

Must. It is the didactic core the author chose.

## Hand-off for review

The commits, each criterion with its command and result, screenshots of both flows in both themes.
