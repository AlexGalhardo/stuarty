# SDD-000: Shared definitions

- Amended (2026-11-18) for Stuarty: the product is Stuarty (ADR-0031, ADR-0044). The command is `stuarty`, the variables are `STUARTY_*` and the workspace folder is `~/.stuarty`; section 13 lists the former names, which are still read. A new domain adds its names here first.
- Status: Authoritative (2026-11-14). Every SDD uses these names exactly. A name is changed here first, then everywhere.
- Depends on: ADR-0001, ADR-0006, ADR-0008, ADR-0015 to ADR-0020
- Last change: 2026-11-18, the rename to Stuarty (ADR-0044): every name that carried the product name, and section 13 with the former names. Before that, 2026-11-14: names added from the review of SDD-005, SDD-006 and SDD-007 (operations outside the agent surfaces, tables `document_backups`, `audit` and `api_keys`, `E_RATE_LIMITED`, workspace files, limit variables)
- Purpose: one place for the names that several specifications share, so that documents written in parallel agree.

## 1. Service operations

`service.Service` is the single application boundary (ADR-0008). Each operation maps to exactly one MCP tool, one CLI command and one HTTP endpoint. Details of arguments and results are in SDD-003.

| Service operation | MCP tool | CLI command | HTTP (under `/api/v1`) | Scope |
| --- | --- | --- | --- | --- |
| `ProjectList` | `project_list` | `project:list` | `GET /projects` | read |
| `ProjectCreate` | `project_create` | `project:create` | `POST /projects` | edit |
| `ProjectGet` | `project_get` | `project:show` | `GET /projects/{id}` | read |
| `ProjectTrash`, `ProjectRestore` | `project_trash` | `project:trash`, `project:restore` | `POST /projects/{id}/trash`, `POST /projects/{id}/restore` | edit |
| `Edit` | `edit` | `edit` | `POST /projects/{id}/edit` | edit (or media, per command) |
| `CommandHelp` | `command_help` | `command:help` | `GET /commands`, `GET /commands/{name}` | read |
| `HistoryList`, `Undo`, `Redo` | `history` | `history`, `undo`, `redo` | `GET /projects/{id}/history`, `POST /projects/{id}/undo`, `POST /projects/{id}/redo` | read, edit |
| `HistoryRevert`, `HistoryDiff`, `CheckpointCreate`, `CheckpointList`, `CheckpointRename`, `CheckpointRemove` | `history` (`action` `revert`, `diff`, `checkpoint`, `checkpoints`, `rename_checkpoint`, `remove_checkpoint`) | `revert`, `diff`, `checkpoint:create`, `checkpoint:list`, `checkpoint:rename`, `checkpoint:remove` | `POST /projects/{id}/revert`, `GET /projects/{id}/diff`, `/projects/{id}/checkpoints` (`GET`, `POST`) and `/projects/{id}/checkpoints/{name}` (`PATCH`, `DELETE`) | read, edit |
| `MediaImport` | `media_import` | `media:import` | `POST /projects/{id}/media` | media |
| `MediaInspect` | `media_inspect` | `media:inspect` | `GET /projects/{id}/media/{asset_id}` | read |
| `MediaTrash`, `MediaRestore` | none (CLI and HTTP only) | `media:trash`, `media:restore` | `POST /projects/{id}/media/{asset_id}/trash`, `.../restore` | media |
| `ViewFrame`, `ViewSheet` | `view` | `view` | `GET /projects/{id}/frame`, `GET /projects/{id}/sheet` | read |
| `Lint` | `lint` | `lint` | `GET /projects/{id}/lint` | read |
| `JobStart` | `job_start` | `export`, `transcribe` | `POST /projects/{id}/jobs` | render (export), media (transcribe) |
| `JobGet` | `job_get` | `job:show` | `GET /jobs/{job_id}` | read |
| `JobCancel` | `job_cancel` | `job:cancel` | `POST /jobs/{job_id}/cancel` | render; media is enough for the caller's own `transcribe` or `ingest` job |
| `TemplateList`, `TemplateSave`, `TemplateInstantiate`, `TemplateExport`, `TemplateImport` | `template` | `template:list`, `template:save`, `template:new`, `template:export`, `template:import` | `GET /templates`, `POST /templates`, `POST /templates/{id}/instantiate`, `GET /templates/{id}/file`, `POST /templates/import` | read, edit |
| `Doctor` | none | `doctor` | `GET /system` | read |
| `MotionBrief` | `view` with `mode` `motion` | `motion:brief` | `GET /projects/{id}/motion/brief` (what an external motion render must fit; SDD-011 §15) | read |
| `MotionCheck` | none (the tool list has no room, SDD-011 §15.5) | `motion:check` | `GET /projects/{id}/motion/check` (whether a rendered file fits the brief; SDD-011 §15.3) | read |
| `MotionReplace` | `edit` (a batch of `clip.replace_media`) | `motion:replace` | `POST /projects/{id}/motion/replace` (every clip that reads an earlier render of the same `render_key`, in one undoable batch; SDD-011 §15.4) | edit |
| event stream | none | `events` (prints the stream) | `GET /projects/{id}/events` | read |
| `PlanWindow` | none | none | `GET /projects/{id}/plan` (SDD-004 §10) | read |
| `MediaList` | `media_inspect` called without an asset | `media:list` | `GET /projects/{id}/media` (every asset of the project, including those still ingesting or failed, which are not in the document) | read |
| `JobList` | `job_get` called with a project and no job | `job:list` | `GET /projects/{id}/jobs` | read |
| `SnapPoints` | none | none | `GET /projects/{id}/snap-points` (SDD-012 §7.8) | read |
| `AssetList` | `media_inspect` called with no `project_id` (the library) | `asset:list` | `GET /assets` (the ready files of the workspace by content hash, with the origin, tags and reuse of SDD-011 §14; filters `kind`, `section`, `tag`, `project`, `origin`, `reusable`, `text`; the picker of a brand logo) | read |
| `AssetGet`, `AssetUses` | `media_inspect` called with a `hash` (`AssetGet` only; its entry carries the uses) | `asset:show`, `asset:uses` | `GET /assets/{hash}`, `GET /assets/{hash}/uses` (one library entry with its origin and the projects that use it, SDD-011 §14) | read |
| `AssetSetInfo`, `AssetTag` | `media_import` called with a `hash` and no `paths` | `asset:set`, `asset:tag` | `PUT /assets/{hash}`, `POST /assets/{hash}/tags` (the description, the rights note, the reuse mark and the tags of a library entry, SDD-011 §14) | media |
| `FontList` | none | `fonts` | `GET /fonts` (families and weights available, and the imported font files with their hash) | read |
| `OpenFile` | none | none (the job result gives the path to local callers; with a remote server the CLI downloads the file) | `GET /jobs/{job_id}/file` and the byte-serving routes listed below | read |
| `AuditList` | none | `audit` | `GET /audit` | admin |
| `TrashEmpty` | none | none | `POST /trash/empty` | admin, and only from a browser session (local mode) or an `admin` key (network mode); the local token is refused (SDD-007 §5.3) |
| `KeyCreate`, `KeyList`, `KeyRevoke` | none | none | `POST /keys`, `GET /keys`, `DELETE /keys/{key_id}` | a logged-in browser session; a key cannot call them (network mode only) |
| `UserList`, `UserCreate`, `UserUpdate` | none | none | `GET /users`, `POST /users`, `PATCH /users/{user_id}` | `admin`, and only from a logged-in browser session in network mode; a key cannot call them, whatever its scopes (in local mode the routes answer 404 `E_NOT_FOUND`). `UserUpdate` changes `role` and `disabled` in one statement and ends the user's sessions; the last enabled administrator cannot be demoted or disabled (`E_INVALID_STATE`); an unknown user is `E_NOT_FOUND`; a bad or used email, a bad role or a short password is `E_VALIDATION`. They are not methods of `service.Service`: they live in the auth side of network mode (`web/app/auth`) |
| `ProjectBundleExport`, `ProjectBundleImport` | none | `bundle:export`, `bundle:import` | `POST /projects/{id}/bundle`, `POST /projects/import` | read and render; edit and media |
| `FontImport` | none | `font:import` | `POST /fonts` | media |
| `BrandKitGet`, `BrandKitSet` | none | `brand:show`, `brand:set` | `GET /brand-kit`, `PUT /brand-kit` | read, edit |
| `ProjectOtioExport`, `ProjectOtioImport` | none | `otio:export`, `otio:import` | `GET /projects/{id}/otio`, `POST /projects/{id}/otio` | read; edit (the batch is applied through `Edit`, so each operation needs its own scope too) |
| `ProjectDescribe` | `project_get` called with `detail` `overview` | `project:describe` | `GET /projects/{id}/describe` (the production as text: brief, items, state; SDD-003 §2.3) | read |
| `ProjectDuplicate` | `project_create` called with `from` | `project:duplicate` | `POST /projects/{id}/duplicate` (a copy of the production; its timelines get new ids, SDD-003 §2.3) | edit |
| `ProjectItemAdd`, `ProjectItemMove`, `ProjectItemRename`, `ProjectItemTrash`, `ProjectItemRestore` | `project_item` (argument `action`) | `item:add`, `item:move`, `item:rename`, `item:trash`, `item:restore` | `POST /projects/{id}/items`, `POST /projects/{id}/items/{item_id}/move`, `.../rename`, `.../trash`, `.../restore` | edit |
| `BriefGet`, `BriefSet` | `project_item` (`action` `brief`; given `text`, it sets) | `brief:show`, `brief:set` | `GET /projects/{id}/brief`, `PUT /projects/{id}/brief` | read, edit |
| `BriefingQuestions`, `BriefingAnswer` | `project_item` (`action` `briefing`; given `answers`, it answers) | `briefing:questions`, `briefing:answer` | `GET /briefing/questions`, `PUT /projects/{id}/briefing` | read, edit (SDD-005 §5.12.1) |
| `ConnectionList`, `ConnectionGet` | `connection` (list, or one by id) | `connection:list`, `connection:show` | `GET /connections`, `GET /connections/{id}` | read |
| `ConnectionCreate`, `ConnectionRemove`, `ConnectionTest` | none | `connection:add`, `connection:remove`, `connection:test` | `POST /connections`, `DELETE /connections/{id}`, `POST /connections/{id}/test` | admin (SDD-014) |
| `ImageCreate` | `image` (`action` create) | `image:create` | `POST /images` | edit |
| `ImageList` | `image` (`action` list) | `image:list` | `GET /images` | read |
| `ImageDescribe` | `image` (`action` show) | `image:show` | `GET /images/{id}` | read |
| `ImageEdit` | `image` (`action` edit) | `image:edit` | `POST /images/{id}/edit` | edit |
| `ImageUndo`, `ImageRedo` | `image` (`action` undo, redo) | `image:undo`, `image:redo` | `POST /images/{id}/undo`, `POST /images/{id}/redo` | edit |
| `ImageExport` | `image` (`action` export) | `image:export` | `POST /images/{id}/export` | edit |
| `ImageImport` | `image` (`action` import) | `image:import` | `POST /images/import` (an image file from a path or a library hash becomes a new image document, or one more layer of the document `image_id`; SDD-021 §10) | edit |
| `ImageImportFile` | none | none | `POST /images/import/file` (a file chosen in the browser, as the bytes of the request, becomes a new image document or a layer; SDD-021 §10.2; HTTP only because the bytes are the request) | edit |
| `ExportPresetList` | `command_help` (`domain` export) | `export:presets` | `GET /export-presets` | read |
| `ExportCheck` | `lint` (with `preset`) | `export:check` | `GET /projects/{id}/export-check` | read |
| `AudioMeasure` | `view` (`mode` audio) | `view --mode audio` | `GET /projects/{id}/audio-measure` | read |
| `ExportBatch` | `job_start` (`kind` export_batch) | `export:batch` | `POST /projects/{id}/export-batches` | render |
| `ImageFrame` | none | none | `GET /images/{id}/frame` (SDD-012 §24.3) | read |
| `ImageHistory` | none | none | `GET /images/{id}/history` (SDD-012 §24.3) | read |
| `ImageFile` | none | none | `GET /images/{id}/exports/{name}` (SDD-012 §24.3) | read |
| image document stream | none | none | `GET /images/{id}/events` (SDD-006 §12; the event `image.updated`) | read |
| `ApprovalList`, `ApprovalGet` | none | `approval:list`, `approval:show` | `GET /approvals`, `GET /approvals/{id}` | read (SDD-015) |
| `ApprovalDecide` | none | `approval:decide` | `POST /approvals/{id}/decide` | read, and only from a privileged browser session; the local token and keys get `E_PRIVILEGED_SESSION` (SDD-015 §4) |
| `ApprovalRuleList` | none | `approval:rule:list` | `GET /approval-rules` | read |
| `ApprovalRuleSet`, `ApprovalRuleRemove` | none | `approval:rule:set`, `approval:rule:remove` | `POST /approval-rules`, `DELETE /approval-rules/{id}` | admin, and only from a privileged browser session |
| `CostSummary` | none | `cost:show` | `GET /cost` | read |
| `CostCeilingSet` | none | `cost:ceiling` | `PUT /cost/ceilings` | admin, and only from a privileged browser session |
| approval queue stream | none | none | `GET /approvals/events` (SDD-015 §7) | read |
| `SocialNetworkList` | none | `social:network:list` | `GET /social/networks` | read (SDD-020) |
| `SocialConnectBegin` | none | `social:connect` | `POST /social/connect` | admin |
| `SocialConnectComplete` | none | none | `POST /social/complete` (the page `/social/callback` posts to it) | admin, and only from a browser session |
| `SocialAccountList` | none | `social:account:list` | `GET /social/accounts` | read |
| `SocialAccountSelect` | none | none | `POST /social/select` | admin, and only from a browser session (SDD-020 §17) |
| `PublishValidate` | none | `publish:validate` | `POST /publish/validate` | read |
| `PublishNow` | none yet (later `job_start` with kind `publish.upload`, SDD-020 §13) | `publish:now` | `POST /publish` | render; the job waits for approval |
| `PublishStatus` | none | `publish:status` | `GET /publish/{job_id}` | read |
| `ProviderList` | none | `provider:list` | `GET /providers` | read (SDD-016) |
| `ProviderModels` | none | `provider:models` | `GET /connections/{id}/models` | read |
| `ProviderPriceSet` | none | `provider:price` | `PUT /providers/prices` | admin, and only from a privileged browser session (SDD-016 §7) |
| `TextGenerate` | `job_start` (kind `text.generate`) | `text:generate` | `POST /text/generate` | edit |
| `MemoryList`, `MemoryGet`, `MemoryWrite` | `library` (kind `memory`; actions `list`, `get`, `write`) | `memory:list`, `memory:show`, `memory:write` | `GET /library/memory`, `GET /library/memory/{name}`, `POST /library/memory`, `DELETE /library/memory/{name}` | read, edit (SDD-018) |
| `MemoryRemove` | none (an agent tool removes nothing) | `memory:remove` | `DELETE /library/memory/{name}` | edit |
| `SkillList`, `SkillGet`, `SkillWrite`, `SkillLint` | `library` (kind `skill`; actions `list`, `get`, `write`, `lint`) | `skill:list`, `skill:show`, `skill:write`, `skill:lint` | `GET /library/skills`, `GET /library/skills/{name}`, `PUT /library/skills/{name}`, `GET /library/skills/{name}/lint` | read, edit |
| `SkillImport`, `SkillExport` | none | `skill:import`, `skill:export` | `POST /library/skills/import`, `POST /library/skills/export` | media |
| `SkillMarkReviewed` | none | none | `POST /library/skills/{name}/review` | edit, and only from a browser session (SDD-018 section 7) |
| `PromptList`, `PromptGet`, `PromptWrite`, `PromptRender` | `library` (kind `prompt`; actions `list`, `get`, `write`, `render`) | `prompt:list`, `prompt:show`, `prompt:write`, `prompt:render` | `GET /library/prompts`, `GET /library/prompts/{name}`, `PUT /library/prompts/{name}`, `POST /library/prompts/{name}/render` | read, edit |
| `LibrarySearch`, `LibraryReindex` | `library` (action `search`), none | `library:search`, `library:reindex` | `GET /library/search`, `POST /library/reindex` | read, edit |
| `AgentProfileList` | none (an agent has no use for the command line of another) | `agent:profiles` | `GET /agents/profiles` | read (SDD-017 §3) |
| `AgentProfileSet`, `AgentProfileRemove` | none | none | `PUT /agents/profiles/{name}`, `DELETE /agents/profiles/{name}` | admin, and only from a privileged browser session; every other caller gets `E_PRIVILEGED_SESSION` (SDD-017 §3, §10) |
| `ChatSessionStart`, `ChatSend`, `ChatCancel`, `ChatAnswerPermission`, `ChatSessionDelete` | none | none | `POST /projects/{id}/chat/sessions`, `POST /chat/sessions/{id}/messages`, `POST /chat/sessions/{id}/cancel`, `POST /chat/sessions/{id}/permissions/{request_id}`, `DELETE /chat/sessions/{id}` | edit, and only from a privileged browser session; the hosted agent's own token is refused (SDD-017 §10) |
| `ChatSessionList`, `ChatSessionGet`, `ChatTranscriptGet` | none | `chat:sessions`, `chat:session`, `chat:transcript` | `GET /projects/{id}/chat/sessions`, `GET /chat/sessions/{id}`, `GET /chat/sessions/{id}/transcript` | read (SDD-017 §9) |
| chat session stream | none | none | `GET /chat/sessions/{id}/events` (SDD-017 §8) | read |

Routes that are not service operations: `POST /mcp` (the MCP server over Streamable HTTP, SDD-003 §4.3; `GET` and `DELETE` on the path are not routes and answer 405 `E_METHOD_NOT_ALLOWED`, because the server is stateless; every tool call behind it is a service operation with the scope of its tool), `GET /api/v1/handshake` (the server proves it knows the local token, SDD-007 §3.8), `GET /auth/launch` and `POST /api/v1/system/launch` (browser session from a one-time code, SDD-007 §3.4), `POST /auth/login` and `POST /auth/logout` (network mode: the browser session of a user, SDD-007 §4.1; in local mode they answer 404 `E_NOT_FOUND`), `GET /up` (vukem health probe), `GET /openapi.json` and `GET /api/docs` (the OpenAPI document and its viewer), `POST /api/v1/system/tools/install` and `GET /api/v1/system/tools` (the explicit install of a helper such as whisper.cpp from the editor, and its state with the progress of a running install; the same step as the CLI command `tools:install`; the install is allowed only to a browser session in local mode or an `admin` key in network mode, never to the local token and never on an agent surface, SDD-007 §6.8; the `GET` only reads and takes the `read` scope, so an agent can tell whether a helper is installed before it asks for a transcript), and the byte-serving routes that the editor uses, all `read`, all under the project so that access is checked against it, all with HTTP Range and long-lived caching because their content never changes: `GET /projects/{id}/media/{asset_id}/proxy`, `.../thumbnail`, `.../strip`, `.../peaks`, `.../lut` (the parsed table of a lookup table asset, SDD-004 §3.6), and `GET /projects/{id}/bitmaps/{key}` (rasterized text and shapes by content key). What each returns is in SDD-011 and SDD-004.

The MCP catalogue is 20 tools: `project_list`, `project_create`, `project_get`, `project_trash`, `edit`, `command_help`, `history`, `media_import`, `media_inspect`, `view`, `lint`, `job_start`, `job_get`, `job_cancel`, `template`, `font_import`, `connection` (read only: the destinations the user connected, SDD-014), `image` (the image documents of SDD-021, one tool with an `action`), `project_item` (the items and the brief of a production, SDD-003 section 2.3), `library` (memory, skills and prompts: search, list, get, write, render and lint; SDD-018).

**Projects and timelines (ADR-0039).** A project is a production; its timelines are items of it. Every operation above that edits or reads a timeline document (`Edit`, `HistoryList`, `Undo`, `Redo`, `HistoryRevert`, `HistoryDiff`, `Checkpoint*`, `Lint`, `MediaImport`, `MediaInspect`, `MediaList`, `MediaTrash`, `MediaRestore`, `ViewFrame`, `ViewSheet`, `JobStart`, `JobList`, `PlanWindow`, `SnapPoints`, `ProjectOtioExport`, `ProjectOtioImport`, `ProjectBundleExport`, `ExportCheck`, `ExportBatch`, and `ProjectGet` with a detail) accepts an optional `timeline_id` next to `project_id`; the rule is SDD-003 §1.7.

Other CLI commands that are not service operations: `serve` (start the owner process), `mcp` (stdio MCP server), `open` (open the UI of a running server with a fresh launch code), `token:rotate`, `user:create` and `user:passwd` (network mode; they need the workspace lock, so no server may be running), `tools:install` (download a helper such as whisper.cpp after stating its size), `agent:docs` (print or write the generated material for agents, SDD-003 §8), plus vukem's built-in commands.

### 1.1 Domains (ADR-0043, epic 13)

Every operation belongs to one **domain**. The domain names are fixed here; a new domain is added here first.

| Domain | What it holds | Operations today |
| --- | --- | --- |
| `project` | productions: projects, their items, the brief, bundles | `ProjectList`, `ProjectCreate`, `ProjectGet`, `ProjectTrash`, `ProjectRestore`, `ProjectDescribe`, `ProjectDuplicate`, `ProjectItem*`, `Brief*`, `ProjectBundle*`, the event stream |
| `video` | the timeline document: its edit commands, history, lint, frames, jobs, templates, OpenTimelineIO, the hand-off from a motion engine | `Edit` and the commands of the timeline registry, `HistoryList`, `Undo`, `Redo`, `HistoryRevert`, `HistoryDiff`, `Checkpoint*`, `Lint`, `View*`, `Motion*` (brief, check, replace; epic 33), `JobStart`, `JobGet`, `JobList`, `JobCancel`, `ExportFile`, `Template*`, `SnapPoints`, `ProjectOtio*`, `Export*` (presets, the check against one, the batch) |
| `image` | image documents (SDD-021) | `Image*` and the commands of the image registry |
| `writing` | written documents | none yet |
| `asset` | files of the workspace: media, the asset library, fonts | `Media*`, `Asset*` (the library of epic 08: list, get, uses, set info, tag), `FontImport`, `FontList` |
| `publish` | publications and their approval | `Approval*` (the queue that gates a publication and a costly generation, SDD-015), `Social*` (the networks and the accounts a publication goes to), `Publish*` (validate, publish now, status; SDD-020) |
| `provider` | AI providers and generation | `Provider*` (the providers, their models and the price table, SDD-016), `TextGenerate`, `Cost*` (the spend on providers and its ceilings) |
| `memory` | the library: notes, skills, prompts, search | `Memory*`, `Skill*`, `Prompt*`, `Library*` |
| `workspace` | the installation: help, connections, brand kit, health, keys, audit, trash, helpers, the hosted agent chat | `CommandHelp`, `Connection*`, `BrandKit*`, `Doctor`, `AuditList`, `TrashEmpty`, `Tools*`, `Key*`, `AgentProfile*`, `Chat*` (SDD-017) |

Rules: a command (an operation of a registry in `core/command`) takes its domain from the registry that holds it (`command.NewRegistry(domain)`); the registry refuses a command whose own `Domain` differs, and a registry with no domain. The timeline registry is `video`, the image registry `image`. A Service operation has its domain in the table `service.OperationDomains`, and `TestOperationDomains` fails when a Service method has none. The `domain` argument of an MCP tool (SDD-003 section 4.8) takes these names; a missing one means `video`.

## 2. Identifiers

| Prefix | Object | Defined in |
| --- | --- | --- |
| `prj_` | project (a production), and the timeline document it holds: a timeline keeps the `prj_` id of its document (SDD-001, unchanged); the first timeline of a project has the id of the project | SDD-001, SDD-005 §5.12 |
| `pit_` | project item (a brief, a text, a timeline or an output of a project) | SDD-005 §5.12 |
| `ast_` | asset | SDD-001 |
| `trk_` | track | SDD-001 |
| `itm_` | item | SDD-001 |
| `job_` | job | SDD-005 |
| `tpl_` | template | SDD-005 |
| `key_` | API key | SDD-007 |
| `usr_` | user (network mode) | SDD-007 |
| `con_` | connection (SDD-014) | SDD-014 |
| `img_` | image document | SDD-021 |
| `lyr_` | layer of an image document | SDD-021 |
| `apr_` | approval request (SDD-015) | SDD-015 |
| `rul_` | approval rule (SDD-015) | SDD-015 |
| `led_` | cost ledger entry (SDD-015) | SDD-015 |
| `cht_` | hosted agent chat session (SDD-017) | SDD-017 |
| `cpr_` | permission request of a hosted agent (SDD-017 §7) | SDD-017 |

All have the form `<prefix>_<10 lower-case Crockford base32 characters>`.

## 3. Actors

An actor string is one of `user:<id>`, `key:<key id>`, `local:<label>`. `<label>` is supplied by the client in local mode (for example `local:ui`, `local:cli`, `local:claude-code`), limited to `[a-z0-9-]{1,32}`, default `local:cli` for the CLI, `local:mcp` for the MCP server, `local:ui` for the browser and `local:api` for a raw HTTP client that sends no label. A client never obtains `local:ui` by asking for it: that label belongs to the browser session. The stdio MCP server, when it is not given `--actor`, builds its label from the client name and a session id (`local:claude-code-k3m9x2`, SDD-003 section 4.5), so two agents with the same client name are two actors and undo never crosses sessions or reconnections; the label stays within the same `[a-z0-9-]{1,32}` rule. The agent hosted in the chat (SDD-017 section 6.1) is the actor `local:chat-<10 characters of its session id>` in local mode and `key:<session id>` (`cht_` and 10 characters) in network mode, fixed by its token whatever label the client sends.

## 4. Scopes

`read`, `edit`, `media`, `render`, `admin`. In local mode the local token carries all five.

## 5. Events on a project stream

| Event name | When | Payload essentials |
| --- | --- | --- |
| `batch.committed` | a batch was committed | `revision`, `actor`, `diff`, `at` |
| `project.updated` | project metadata outside the document changed (trash, restore) | `project_id`, `state` |
| `asset.updated` | an asset's ingest state or progress changed | `asset_id`, `state`, `progress` |
| `job.updated` | a job's state or progress changed | `job_id`, `kind`, `state`, `progress`, `message` |
| `resync.required` | the client is too far behind to replay | `revision` |

Two events are not on a project stream but on the approval queue stream `GET /approvals/events` (SDD-015 §7): `approval.requested` (a request was made, or is pending when the stream opens) and `approval.decided` (a request was approved, rejected or expired); both carry the request.

The event `id` of `batch.committed` is the revision. Details in SDD-006.

An image document has a stream of its own, `GET /images/{id}/events` (SDD-006 §12), with one event name besides `resync.required`: `image.updated` (a batch of the image was committed), with the revision as its `id` and `image_id`, `revision`, `base_revision`, `kind`, `actor`, `diff` and `at` in its payload. It follows the rules of `batch.committed`, on the topic `image:<id>`.

## 6. Jobs

Kinds: `ingest`, `transcribe`, `export`, `export_batch`, `frame`, `text.generate` (SDD-016 §6), and the kinds a feature epic registers as needing approval (SDD-015 §5). States: `queued`, `running`, `succeeded`, `failed`, `cancelled`, and `waiting_approval` (a job of a kind that needs approval, until a person or a rule approves it; it is not running and counts as active; at most 50 per submitter and 1000 in the workspace, `E_LIMIT_EXCEEDED` past either, SDD-015 §5). Asset ingest states: `pending`, `ingesting`, `ready`, `failed`.

## 7. Database tables

| Table | Holds | Defined in |
| --- | --- | --- |
| `projects` | one row per project (a production): name, trash state, owner | SDD-005 §5.12 |
| `project_items` | the items of a project: brief, text, timeline, output; their order and trash state | SDD-005 §5.12 |
| `checkpoints` | the names and notes a person or an agent put on a revision of a timeline, and the automatic ones before an agent's first edit of the day | SDD-005 §5.14 |
| `timelines` | one row per timeline document: current document, revision, schema version, trash state (the former `projects` table, renamed) | SDD-005 §5.2 |
| `operations` | the append-only log, one row per committed batch of a timeline (its `project_id` column holds the timeline id) | SDD-005 |
| `idempotency_keys` | stored batch results by key | SDD-005 |
| `assets` | one row per file in the content store, by hash | SDD-005 |
| `artifacts` | derived files of an asset (probe, normalized, proxy, chunks, thumbnails, peaks, silence, loudness, speech, beats, frames, transcript), keyed by asset hash, kind, profile version, variant and extension | SDD-005, SDD-011 §6 |
| `project_assets` | which project uses which asset, and trash state of that use | SDD-005 |
| `jobs` | job state (ADR-0018) | SDD-005 |
| `templates` | template documents | SDD-005 |
| `document_backups` | the document as it was before a schema migration or an import | SDD-005 |
| `audit` | append-only record of actions outside a document batch (trash, restore, emptying, imports, jobs, keys), with their actor | SDD-005 |
| `api_keys` | network mode: maps a `key_` identifier to its vukem token, owner and optional project | SDD-005, SDD-007 |
| `queue_jobs`, `queue_failed_jobs` | vukem's queue | vukem |
| `users` | network mode: email, argon2id password hash, role (`admin` or `member`), session version | SDD-007 |
| `brand_kits` | the brand kit: logo, named colours and fonts, one row per owner  | SDD-005 §5.11 |
| `font_owners` | which user imported or brought each stored font file, network mode  | SDD-005 §5.11, SDD-007 §7.7 |
| `connections` | the destinations the user connected: kind, label, hosts, what each may receive, last use and the id of its secret in the vault; no secret | SDD-014 |
| `image_documents`, `image_operations` | image documents, and their append-only log of batches | SDD-005 §5.12, SDD-021 |
| `approval_requests`, `approval_rules`, `cost_ceilings`, `cost_ledger` | the approval queue, the rules that approve without asking, the cost ceilings and the record of every estimate and real cost; no secret | SDD-015 |
| `image_documents`, `image_operations` | image documents, and their append-only log of batches | SDD-005 §5.13, SDD-021 |
| `library_entries`, `library_fts` | the index of memory, skills and prompts for search and lists; rebuilt from the files, holds no content that the files lack | SDD-018 §9 |
| `asset_info`, `asset_tags`, `asset_search` | the library side of an asset by hash: origin, rights note, description, the reuse mark, the tags and the full-text index over them | SDD-005 §5.5.1, SDD-011 §14 |
| `agent_profiles`, `chat_sessions`, `chat_events` | the agents the user may host (a name, an executable and its arguments), and the transcripts of the chat sessions per project; no credential, no environment | SDD-017 §3, §9 |
| vukem's own tables for sessions (`sessions`) and tokens (`access_tokens`) | network mode | vukem |

A document with a `template` member is a template draft, or an instance when `template.instance` is true (SDD-001 section 3.10). `TemplateInstantiate` writes the flag; lint reads it (`L_SLOT_UNFILLED`, SDD-003).

## 8. Error codes outside the core

The core's codes are in SDD-002 §A.8. The service adds:

| Code | Meaning | Retryable |
| --- | --- | --- |
| `E_UNAUTHORIZED` | missing or invalid token, key or session | no |
| `E_FORBIDDEN` | the key lacks the scope or is limited to another project | no |
| `E_WORKSPACE_LOCKED` | another process owns the workspace and cannot be reached | yes |
| `E_PATH_NOT_ALLOWED` | a path is outside the allowed import roots or is not a regular file | no |
| `E_FILE_NOT_FOUND` | the path does not exist or cannot be read | no |
| `E_UNSUPPORTED_MEDIA` | the file is not media FFmpeg can decode, or has no usable stream | no |
| `E_INGEST_FAILED` | ingest failed; `details.reason` | no |
| `E_ASSET_NOT_READY` | the asset is still ingesting | yes |
| `E_FFMPEG_MISSING` | no usable FFmpeg was found | no |
| `E_NO_ENCODER` | no encoder works on this machine for the codec the export needs (H.264, or the `details.codec` that was asked for: `hevc` or `av1`, SDD-004 section 8.3); `details.reason` `codec_unavailable` and `details.candidates` | no |
| `E_EXPORT_FAILED` | the export process failed; `details.reason` | no |
| `E_VERIFY_FAILED` | the exported file did not pass verification; `details.checks` | no |
| `E_TRANSCRIBE_UNAVAILABLE` | no transcription provider is installed or configured; `details` names what is missing, its download size, the install command and the `tool` and `model` it installs | no |
| `E_TRANSCRIBE_FAILED` | a transcription ran and failed (no output, provider error, no word timings); `details.reason` | no |
| `E_QUEUE_FULL` | a concurrency limit is reached; `details.limit` names it (`exports`, `exports_per_key`, `ingests`, `transcribes`, `streams`, `frames`) with `value` and `ceiling`, and `details.running` lists the jobs of a job queue | yes |
| `E_RATE_LIMITED` | the caller sent too many requests; the answer carries `Retry-After`, and `details` has `retry_after`, `limit`, `value` and `ceiling` | yes |
| `E_LIMIT_EXCEEDED` | a configured ceiling (file size, duration, resolution) is exceeded | no |
| `E_CANCELLED` | the job was cancelled | no |
| `E_CLIENT_CLOSED` | the caller closed the request (or its deadline passed) before the answer; status 499, logged below error level, never a server fault | no |
| `E_NOTHING_TO_UNDO`, `E_NOTHING_TO_REDO` | the actor has no step to undo or redo | no |
| `E_LINT_BLOCKING` | an export was refused because the lint has errors; `details.findings` | no |
| `E_PAYLOAD_TOO_LARGE` | a request or batch is too large | no |
| `E_NOT_FOUND` | also used by the service for unknown projects, jobs, templates | no |
| `E_METHOD_NOT_ALLOWED` | HTTP only: the route does not answer that method; `OPTIONS` is always refused (no CORS, SDD-007 §3.6) | no |
| `E_CONNECTION_NOT_FOUND` | the connection does not exist or was removed (SDD-014 §7) | no |
| `E_CONNECTION_DESTINATION` | a request through a connection is for a host or a scheme it does not allow (SDD-014 §6) | no |
| `E_VAULT_LOCKED` | the vault cannot be opened: master key missing, wrong or not storable; `details.reason` (SDD-014 §4) | no |
| `E_PRIVILEGED_SESSION` | the operation is for a person in a privileged browser session; the caller is the local token, a key or an agent surface (SDD-015 §4) | no |
| `E_APPROVAL_NOT_FOUND` | the approval request or the approval rule does not exist (SDD-015 §8) | no |
| `E_COST_CEILING` | an estimate is over a cost ceiling; `details` hold `scope`, `reason`, `limit_micros`, `spent_micros`, `estimate_micros`, `currency`; nothing was spent (SDD-015 §6) | no |
| `E_APPROVAL_EXPIRED` | in the error of a job whose approval request expired before a person answered (SDD-015 §3) | no |
| `E_PUBLISH` | a network refused or failed a publication; `details.kind` is `quota`, `not_eligible`, `auth`, `invalid`, `rejected` or `unavailable`, with `network` and, for a quota, `retry_after`; the `retryable` of the error is true for the kinds `quota`, `auth` and `unavailable` (SDD-020 §7.3) | no |
| `E_PROVIDER` | an AI provider refused or failed a request; `details.kind` is `auth`, `rate_limited`, `bad_request`, `unavailable` or `unreadable`, with `status`, `code` and `retry_after_seconds` when known; the body of the provider is never in it (SDD-016 §8) | no |
| `E_PRICE_UNKNOWN` | no price is known for the model of a generation, so no estimate can be made; `details` hold `provider` and `model` (SDD-016 §4.2) | no |
| `E_AGENT_NOT_FOUND` | the agent profile does not exist (SDD-017 §11) | no |
| `E_CHAT_NOT_FOUND` | the chat session does not exist, or the caller may not see its project (SDD-017 §11) | no |
| `E_AGENT_UNAVAILABLE` | the hosted agent could not be started or opened; `details.reason` `not_found`, `start_failed`, `exited`, `timeout`, `protocol`, `protocol_version`, `no_http_mcp`, `sign_in_required`, `limit` (SDD-017 §11) | no |

HTTP status mapping: `E_VALIDATION`, `E_INVALID_TIME`, `E_UNKNOWN_COMMAND` 400; `E_UNAUTHORIZED` 401; `E_FORBIDDEN`, `E_PATH_NOT_ALLOWED`, `E_CONNECTION_DESTINATION`, `E_PRIVILEGED_SESSION` 403; `E_NOT_FOUND`, `E_FILE_NOT_FOUND`, `E_CONNECTION_NOT_FOUND`, `E_APPROVAL_NOT_FOUND`, `E_AGENT_NOT_FOUND`, `E_CHAT_NOT_FOUND` 404; `E_REVISION_CONFLICT`, `E_IDEMPOTENCY_MISMATCH`, `E_OVERLAP`, `E_INVALID_STATE`, `E_IN_USE`, `E_NOT_EMPTY`, `E_ID_EXISTS`, `E_ASSET_NOT_READY` 409; `E_PAYLOAD_TOO_LARGE` 413; `E_METHOD_NOT_ALLOWED` 405; `E_LIMIT`, `E_LIMIT_EXCEEDED`, `E_UNSUPPORTED_MEDIA`, `E_TYPE_MISMATCH`, `E_OUT_OF_SOURCE`, `E_FONT_UNKNOWN`, `E_SLOT`, `E_LINT_BLOCKING`, `E_COST_CEILING`, `E_PRICE_UNKNOWN` 422; `E_QUEUE_FULL`, `E_RATE_LIMITED` 429; `E_NOTHING_TO_UNDO`, `E_NOTHING_TO_REDO`, `E_APPROVAL_EXPIRED` 409; `E_CLIENT_CLOSED` 499; `E_WORKSPACE_LOCKED`, `E_TRANSCRIBE_UNAVAILABLE`, `E_FFMPEG_MISSING`, `E_NO_ENCODER`, `E_VAULT_LOCKED`, `E_AGENT_UNAVAILABLE` 503; `E_PUBLISH`, `E_PROVIDER` 502; `E_INTERNAL` and everything else 500. The job codes `E_INGEST_FAILED`, `E_EXPORT_FAILED`, `E_VERIFY_FAILED`, `E_TRANSCRIBE_FAILED` and `E_CANCELLED` normally travel inside a job object in a 200 answer; they are 500 only when a call itself raises them.

CLI exit codes: `0` success; `2` for every code marked retryable; `1` for the rest.

## 9. Workspace layout

```text
<workspace>/                 default ~/.stuarty, or $STUARTY_HOME
├── db.sqlite                database
├── adopted.json             present only in a workspace adopted from `~/.vukem-video`: where it came from, how and the backup (SDD-005 §6.6)
├── lock                     empty file that carries the operating-system lock of the owner process (SDD-005 §6.2)
├── runtime.json             owner process: schema, pid, addr, mode, started_at, version (present only while a server runs; no secret)
├── token                    local token, file mode 0600 (local mode only)
├── vault.json               encrypted credentials of the connections, file mode 0600; created with the first credential (SDD-014 §3)
├── publish/                 uploads/<key>.json: the address of an upload session of a publication in progress, file mode 0600, no token (SDD-020 §7.4)
├── providers/               prices.json: the price table the person edits for the models whose provider publishes no price, file mode 0600 (SDD-016 §4.3)
├── library/                 memory/, prompts/ and skills/<name>/SKILL.md: the files of the library, the truth of the index; .review-key seals the review marks (SDD-018 §2, §7)
├── store/                   content-addressed originals: store/<first 2 hex>/<sha256>; store/.incoming/ holds copies in progress
├── artifacts/               derived files: artifacts/<asset sha256>/<kind>-<profile version>[-<variant>].<ext> (SDD-011 §6.1)
├── exports/                 exported files: exports/<project id>/<job id>.mp4 (`.gif`, `.wav` or `.m4a` for the other formats of SDD-004 §8.1)
│   └── images/              exported images: exports/images/<image id>/<revision>-<digest>.<ext> (SDD-021 §6)
├── frames/                  cache of rendered frames: frames/<project id>/<revision>/<frame>-<width>.jpg
├── bitmaps/                 cache of rasterized text and shapes, by content hash
├── fonts/                   user fonts, by hash 
├── tools/                   installed helpers (whisper.cpp, models); tools/.download/ holds downloads in progress
├── chat/                    the working directory of a hosted agent, one folder per project: chat/<project id>/, mode 0700 (SDD-017 §4)
├── tmp/                     one directory per running child process: tmp/<job id>/ (SDD-007 §6.4)
├── backups/                 copies of db.sqlite made before a migration (SDD-005 §7)
├── logs/                    rotating log files
└── trash/                   nothing lives here: trash is a database state, files are kept in place
```

## 10. Configuration

Environment variables, all optional. Names are final. Ceilings that have no variable here are fixed constants of the code, listed in the SDD that owns them (for example the longest side of a source and the length of the ingest queue: SDD-011 §1.5); a constant becomes a variable only by adding it to this table first.

| Variable | Default | Meaning |
| --- | --- | --- |
| `STUARTY_HOME` | `~/.stuarty` | workspace directory |
| `STUARTY_ADDR` | `127.0.0.1:7878` | bind address; a non-loopback address requires network mode |
| `STUARTY_MODE` | `local` | `local` or `network` |
| `STUARTY_FFMPEG`, `STUARTY_FFPROBE` | discovered | explicit binaries |
| `STUARTY_ENCODER` | automatic | force an H.264 encoder by FFmpeg name |
| `STUARTY_ALLOW_USER_X264` | `false` | allow `libx264` when the user's own FFmpeg has it |
| `STUARTY_EXPORT_RENDERER` | `go` | what draws the exported frames: `go` (the compositor in Go), `rust` (the renderer of `renderer/`, a WebAssembly module under `wazero`, which draws the same bytes: SDD-013 §3.10) or `gpu` (the native GPU host `stuarty-render` when the machine has a graphics adapter, `rust` otherwise: SDD-013 §4.9). Any other value stops the server at start. The default changes only when the stage C gate passes on every system |
| `STUARTY_IMPORT_ROOTS` | none | in network mode, directories from which import by path is allowed |
| `STUARTY_MAX_EXPORTS` | `2` | concurrent exports per workspace |
| `STUARTY_MAX_IMPORT_BYTES` | `8589934592` (8 GiB) | largest file accepted by import |
| `STUARTY_TRANSCRIBE_URL`, `STUARTY_TRANSCRIBE_KEY`, `STUARTY_TRANSCRIBE_MODEL` | none | OpenAI-compatible transcription API; when unset, the local provider is used |
| `STUARTY_WHISPER_MODEL` | `small` | local model name: `tiny`, `base`, `small` or `large-v3-turbo` |
| `STUARTY_TEST_TRANSCRIBER` | none | test only: `stub` selects a provider that answers every audio with four fixed words (`hello` 100-400 ms, `brave` 400-700, `new` 1100-1400, `world` 1400-1900, language `en`). Honoured only with `APP_ENV=test` in local mode; with any other `APP_ENV`, in network mode, or with any other value the server refuses to start. Never set in production |
| `STUARTY_TEST_SOCIAL_URL` | none | test only: the address (`http://127.0.0.1:<port>`) of a server on loopback that plays a social network; the build then also offers the network `fake`, which speaks only to that address (SDD-020 §12). Honoured in local mode only; any other address, scheme or mode and the server refuses to start. Never set in production |
| `STUARTY_MAX_EXPORTS_PER_KEY` | `1` | concurrent exports running per actor (two more may wait) |
| `STUARTY_MAX_INGESTS` | `2` | concurrent ingests per workspace |
| `STUARTY_MAX_TRANSCRIBES` | `1` | concurrent transcriptions per workspace |
| `STUARTY_MAX_TRANSCRIBES_PER_KEY` | `1` | transcriptions of one actor (key or user) that are queued or running; the same rule as `MAX_EXPORTS_PER_KEY` |
| `STUARTY_MAX_IMPORT_SECONDS` | `14400` | longest source accepted by import |
| `STUARTY_MAX_IMPORT_PIXELS` | `33177600` | largest source frame (7680 x 4320); no side above 8192 |
| `STUARTY_MAX_TRANSCRIBE_SECONDS` | `7200` | longest audio per transcription job |
| `STUARTY_MAX_EXPORT_BYTES` | `4294967296` (4 GiB) | largest exported file |
| `STUARTY_MIN_FREE_BYTES` | `2147483648` (2 GiB) | writes are refused below this free space on the workspace volume |
| `STUARTY_CACHE_MAX_BYTES` | `2147483648` (2 GiB) | size of the frame and bitmap caches before eviction |
| `STUARTY_RATE_LIMIT_PER_MINUTE` | `300` | requests per minute per key or local token |
| `STUARTY_RATE_LIMIT_SESSION_PER_MINUTE` | `1800` | requests per minute per browser session |
| `STUARTY_RATE_LIMIT_HEAVY_PER_MINUTE` | `10` | calls of `JobStart`, `MediaImport`, `TemplateImport`, `ProjectBundleExport`, `ProjectBundleImport`, `FontImport`, `ProjectOtioImport` and `AudioMeasure` per minute per credential (SDD-007 section 4.4); one list, the same on the documentation pages and in the OpenAPI document |
| `STUARTY_SANDBOX` | `none` | `bwrap` runs FFmpeg and whisper.cpp in a sandbox on Linux |
| `STUARTY_INSECURE_ALLOW_ROOT` | `false` | allow network mode as root |
| `STUARTY_LAUNCHER_SECRET` | none | set only by the desktop shell in the environment of the server it starts (SDD-007 §5.3); the server reads it once and removes it; never set it by hand or write it to a file |
| `STUARTY_MASTER_KEY`, `STUARTY_MASTER_KEY_FILE` | none | the master key of the vault as base64 or 64 hex digits, or the path of a file that holds it (a Docker secret); without either, a desktop uses the operating system keychain (SDD-014 §4) |
| `STUARTY_ADMIN_EMAIL`, `STUARTY_ADMIN_PASSWORD` | none | network mode: create the first administrator at start-up when no user exists |
| `STUARTY_URL`, `STUARTY_KEY` | none | client side: make the CLI and the stdio MCP server talk to a remote server with an API key |
| `STUARTY_PROJECT` | none | client side: the default project of CLI commands that take one |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | none | turns on OpenTelemetry (ADR-0026) |

The limits and their errors are specified in SDD-007 §9. Variables read by vukem itself and used by the product (`APP_URL`, `APP_KEY`, `TLS_CERT_FILE`, `TLS_KEY_FILE`, `TRUSTED_PROXIES`, `TRUST_FORWARDED_PROTO`, `SHUTDOWN_TIMEOUT`, `DB_MAX_OPEN`) keep vukem's names.

## 11. Headers and cookies

| Name | Kind | Meaning | Defined in |
| --- | --- | --- | --- |
| `Authorization: Bearer <token>` | request header | the local token (`vvl_...`) or an API key (`vvk_...`) | SDD-007 §3.7, §4.2 |
| `Stuarty-Client` | request header | the label of a bearer client in local mode (`[a-z0-9-]{1,32}`), which becomes the actor `local:<label>` | SDD-007 §3.7 |
| `Idempotency-Key` | request header | the idempotency key of an edit over HTTP (8 to 64 characters of `[A-Za-z0-9._~-]`) | SDD-003 §6, SDD-005 §10 |
| `X-Vukem-Revision` | response header | the revision a read or a byte response belongs to | SDD-003 §6 |
| `X-Request-Id` | request and response header | the request identifier, repeated in the error envelope as `request_id` | SDD-003 §1.4 |
| `Last-Event-ID` | request header | the last revision a stream client received (the `since` query parameter does the same) | SDD-006 §4 |
| `stuarty_session_<port>` | cookie | the browser session in local mode, `<port>` being the port of the Host the browser used, so two local servers on one host do not share a cookie (VV-170); in network mode over plain HTTP on localhost the name is `stuarty_session` | SDD-007 §3.5 |
| `__Host-stuarty_session` | cookie | the browser session in network mode over TLS | SDD-007 §4.1 |

## 12. Renderer names (SDD-013)

| Name | Kind | Meaning | Defined in |
| --- | --- | --- | --- |
| `RenderInputVersion` | constant | the version number of the render input the renderer reads, 1 | SDD-013 §2.1 |
| `vr_api_version`, `vr_alloc`, `vr_free`, `vr_init`, `vr_put_texture`, `vr_put_bitmap`, `vr_put_lut`, `vr_release`, `vr_render`, `vr_error`, `vr_text_font_put`, `vr_text_run`, `vr_text_validate_font`, `vr_text_result_ptr`, `vr_text_result_len`, `vr_text_result_clear` | WebAssembly exports | the only functions the core renderer module exports, besides `memory`; the module has no import | SDD-013 §4.2 |
| `API_VERSION` | constant | the version of that boundary (`renderer/stuarty-renderer/src/lib.rs`), 3 (3 added `vr_put_lut`); a host refuses a module with another number | SDD-013 §4.2 |
| `/assets/renderer-<sha256 first 12 hex>.wasm` | route | the core renderer module served to the browser, named by its digest | SDD-013 §4.1 |
| `engine/renderhost` | Go package | the `wazero` host of the core module; the only place that knows the exports | SDD-013 §4.6 |
| `invalid_plan`, `unsupported_version`, `unsupported_layer`, `missing_texture`, `limit_exceeded`, `out_of_memory`, `invalid_argument`, `render_trap`, `device_lost`, `unsupported_format` | renderer reasons | the closed set of reasons of a renderer failure; in an export they appear as `details.renderer_reason` of `E_EXPORT_FAILED` (`details.reason = "render_failed"`), and `limit_exceeded` as `E_LIMIT_EXCEEDED` | SDD-013 §7.1, §7.3 |
| `render_frame_side`, `render_layers`, `render_input_bytes`, `render_texture_side`, `render_texture_bytes`, `render_bitmap_bytes` | `details.limit` values | the renderer's own limits (4096, 64, 4 MiB, 8192, 512 MiB, 128 MiB) | SDD-013 §7.2 |

## 13. Former names (ADR-0044)

The product was called Vukem Video until release 0.29.0. The code, the documents and the generated pages use the names above. Someone who used Vukem Video keeps their projects: the former names below are still read, and the former workspace folder is adopted at the first start.

### 13.1 Variables the program reads

Each variable is read under its new name first. When the new one is unset and the former one is set, the former value is used and one warning per variable is written to the standard error at each start (`stuarty: VUKEM_VIDEO_HOME is deprecated and is read only until release 1.0.0: set STUARTY_HOME instead`). When both names are set the new one wins and one line says so (`stuarty: VUKEM_VIDEO_MODE is ignored because STUARTY_MODE is set; set the value you want under STUARTY_MODE and remove VUKEM_VIDEO_MODE`); the container image sets `STUARTY_MODE`, `STUARTY_HOME`, `STUARTY_FFMPEG` and `STUARTY_FFPROBE` itself, so those four given under the former name to the image are ignored this way. The former names are read until release 1.0.0. One table in `service/config` holds the pairs, and a test walks it.

| Former name | Name |
| --- | --- |
| `VUKEM_VIDEO_HOME` | `STUARTY_HOME` |
| `VUKEM_VIDEO_ADDR` | `STUARTY_ADDR` |
| `VUKEM_VIDEO_MODE` | `STUARTY_MODE` |
| `VUKEM_VIDEO_FFMPEG` | `STUARTY_FFMPEG` |
| `VUKEM_VIDEO_FFPROBE` | `STUARTY_FFPROBE` |
| `VUKEM_VIDEO_ENCODER` | `STUARTY_ENCODER` |
| `VUKEM_VIDEO_ALLOW_USER_X264` | `STUARTY_ALLOW_USER_X264` |
| `VUKEM_VIDEO_EXPORT_RENDERER` | `STUARTY_EXPORT_RENDERER` |
| `VUKEM_VIDEO_IMPORT_ROOTS` | `STUARTY_IMPORT_ROOTS` |
| `VUKEM_VIDEO_MAX_EXPORTS` | `STUARTY_MAX_EXPORTS` |
| `VUKEM_VIDEO_MAX_IMPORT_BYTES` | `STUARTY_MAX_IMPORT_BYTES` |
| `VUKEM_VIDEO_TRANSCRIBE_URL` | `STUARTY_TRANSCRIBE_URL` |
| `VUKEM_VIDEO_TRANSCRIBE_KEY` | `STUARTY_TRANSCRIBE_KEY` |
| `VUKEM_VIDEO_TRANSCRIBE_MODEL` | `STUARTY_TRANSCRIBE_MODEL` |
| `VUKEM_VIDEO_WHISPER_MODEL` | `STUARTY_WHISPER_MODEL` |
| `VUKEM_VIDEO_TEST_TRANSCRIBER` | `STUARTY_TEST_TRANSCRIBER` |
| `VUKEM_VIDEO_MAX_EXPORTS_PER_KEY` | `STUARTY_MAX_EXPORTS_PER_KEY` |
| `VUKEM_VIDEO_MAX_INGESTS` | `STUARTY_MAX_INGESTS` |
| `VUKEM_VIDEO_MAX_TRANSCRIBES` | `STUARTY_MAX_TRANSCRIBES` |
| `VUKEM_VIDEO_MAX_TRANSCRIBES_PER_KEY` | `STUARTY_MAX_TRANSCRIBES_PER_KEY` |
| `VUKEM_VIDEO_MAX_IMPORT_SECONDS` | `STUARTY_MAX_IMPORT_SECONDS` |
| `VUKEM_VIDEO_MAX_IMPORT_PIXELS` | `STUARTY_MAX_IMPORT_PIXELS` |
| `VUKEM_VIDEO_MAX_TRANSCRIBE_SECONDS` | `STUARTY_MAX_TRANSCRIBE_SECONDS` |
| `VUKEM_VIDEO_MAX_EXPORT_BYTES` | `STUARTY_MAX_EXPORT_BYTES` |
| `VUKEM_VIDEO_MIN_FREE_BYTES` | `STUARTY_MIN_FREE_BYTES` |
| `VUKEM_VIDEO_CACHE_MAX_BYTES` | `STUARTY_CACHE_MAX_BYTES` |
| `VUKEM_VIDEO_RATE_LIMIT_PER_MINUTE` | `STUARTY_RATE_LIMIT_PER_MINUTE` |
| `VUKEM_VIDEO_RATE_LIMIT_SESSION_PER_MINUTE` | `STUARTY_RATE_LIMIT_SESSION_PER_MINUTE` |
| `VUKEM_VIDEO_RATE_LIMIT_HEAVY_PER_MINUTE` | `STUARTY_RATE_LIMIT_HEAVY_PER_MINUTE` |
| `VUKEM_VIDEO_SANDBOX` | `STUARTY_SANDBOX` |
| `VUKEM_VIDEO_INSECURE_ALLOW_ROOT` | `STUARTY_INSECURE_ALLOW_ROOT` |
| `VUKEM_VIDEO_LAUNCHER_SECRET` | `STUARTY_LAUNCHER_SECRET` |
| `VUKEM_VIDEO_ADMIN_EMAIL` | `STUARTY_ADMIN_EMAIL` |
| `VUKEM_VIDEO_ADMIN_PASSWORD` | `STUARTY_ADMIN_PASSWORD` |
| `VUKEM_VIDEO_URL` | `STUARTY_URL` |
| `VUKEM_VIDEO_KEY` | `STUARTY_KEY` |
| `VUKEM_VIDEO_PROJECT` | `STUARTY_PROJECT` |
| `VUKEM_VIDEO_NO_BROWSER` | `STUARTY_NO_BROWSER` |

### 13.2 Variables of tests and tools

These are read only by tests, scripts and developer tools. They are renamed with no fallback; the new name replaces the prefix `VUKEM_VIDEO_` with `STUARTY_`.

| Former name | Name |
| --- | --- |
| `VUKEM_VIDEO_BENCH_MIN_RT` | `STUARTY_BENCH_MIN_RT` |
| `VUKEM_VIDEO_BIN` | `STUARTY_BIN` |
| `VUKEM_VIDEO_BUDGET_RELEASE` | `STUARTY_BUDGET_RELEASE` |
| `VUKEM_VIDEO_CONTRACT_BIND_HELPER` | `STUARTY_CONTRACT_BIND_HELPER` |
| `VUKEM_VIDEO_CRASH_ACTION` | `STUARTY_CRASH_ACTION` |
| `VUKEM_VIDEO_CRASH_AT` | `STUARTY_CRASH_AT` |
| `VUKEM_VIDEO_CRASH_BUNDLE` | `STUARTY_CRASH_BUNDLE` |
| `VUKEM_VIDEO_CRASH_EXPORT_JOB` | `STUARTY_CRASH_EXPORT_JOB` |
| `VUKEM_VIDEO_CRASH_PROJECT` | `STUARTY_CRASH_PROJECT` |
| `VUKEM_VIDEO_CRASH_ROLE` | `STUARTY_CRASH_ROLE` |
| `VUKEM_VIDEO_CRASH_TRACK` | `STUARTY_CRASH_TRACK` |
| `VUKEM_VIDEO_DETECT_ONLY` | `STUARTY_DETECT_ONLY` |
| `VUKEM_VIDEO_E2E_HOME` | `STUARTY_E2E_HOME` |
| `VUKEM_VIDEO_E2E_PORT` | `STUARTY_E2E_PORT` |
| `VUKEM_VIDEO_FIREFOX_HEADED` | `STUARTY_FIREFOX_HEADED` |
| `VUKEM_VIDEO_FUTURE_SECRET` | `STUARTY_FUTURE_SECRET` |
| `VUKEM_VIDEO_HARNESS` | `STUARTY_HARNESS` |
| `VUKEM_VIDEO_KILL_ROLE` | `STUARTY_KILL_ROLE` |
| `VUKEM_VIDEO_KILL_TRACKS` | `STUARTY_KILL_TRACKS` |
| `VUKEM_VIDEO_LARGE_TESTS` | `STUARTY_LARGE_TESTS` |
| `VUKEM_VIDEO_NFR` | `STUARTY_NFR` |
| `VUKEM_VIDEO_PERF` | `STUARTY_PERF` |
| `VUKEM_VIDEO_SOAK` | `STUARTY_SOAK` |
| `VUKEM_VIDEO_SOAK_DURATION` | `STUARTY_SOAK_DURATION` |
| `VUKEM_VIDEO_SOAK_HEAPDIR` | `STUARTY_SOAK_HEAPDIR` |
| `VUKEM_VIDEO_SOAK_MINUTES` | `STUARTY_SOAK_MINUTES` |
| `VUKEM_VIDEO_TEST_ALPHA_HASHES` | `STUARTY_TEST_ALPHA_HASHES` |
| `VUKEM_VIDEO_TEST_ARTIFACTS_DIR` | `STUARTY_TEST_ARTIFACTS_DIR` |
| `VUKEM_VIDEO_TEST_COMPOSE_HASHES` | `STUARTY_TEST_COMPOSE_HASHES` |
| `VUKEM_VIDEO_TEST_FFMPEG_DIR` | `STUARTY_TEST_FFMPEG_DIR` |
| `VUKEM_VIDEO_TEST_GOLDEN_BEFORE_DIR` | `STUARTY_TEST_GOLDEN_BEFORE_DIR` |
| `VUKEM_VIDEO_TEST_GPU_ADAPTER` | `STUARTY_TEST_GPU_ADAPTER` |
| `VUKEM_VIDEO_TEST_GPU_HOST` | `STUARTY_TEST_GPU_HOST` |
| `VUKEM_VIDEO_TEST_OTHER` | `STUARTY_TEST_OTHER` |
| `VUKEM_VIDEO_TEST_OTHER_FFMPEG_DIR` | `STUARTY_TEST_OTHER_FFMPEG_DIR` |
| `VUKEM_VIDEO_TEST_STARTUP_ENV_HELPER` | `STUARTY_TEST_STARTUP_ENV_HELPER` |
| `VUKEM_VIDEO_TEST_TOOLS_DIR` | `STUARTY_TEST_TOOLS_DIR` |
| `VUKEM_VIDEO_TEST_VR_PARITY` | `STUARTY_TEST_VR_PARITY` |
| `VUKEM_VIDEO_TOKEN` | `STUARTY_TOKEN` |
| `VUKEM_VIDEO_TRANSCRIBE_TOKEN` | `STUARTY_TRANSCRIBE_TOKEN` |
| `VUKEM_VIDEO_WORKSPACE_TEST_HELPER` | `STUARTY_WORKSPACE_TEST_HELPER` |

### 13.3 Other names

| Former name | Name | Note |
| --- | --- | --- |
| `vukem-video` (the executable) | `stuarty` | one executable: server, CLI, MCP server and UI assets |
| `vukem-video-desktop` | `stuarty-desktop` | the desktop shell |
| `vukem-render` | `stuarty-render` | the native GPU host (SDD-013 §4.9) |
| `vukem-renderer`, `vukem-renderer-text`, `vukem-renderer-gpu` | `stuarty-renderer`, `stuarty-renderer-text`, `stuarty-renderer-gpu` | the Rust crates of `renderer/` |
| `github.com/AlexGalhardo/vukem-video` | `github.com/AlexGalhardo/stuarty` | the Go module path |
| `~/.vukem-video` | `~/.stuarty` | the workspace folder; adopted at the first start (SDD-005 §6.6) |
| `vukem-video.log` | `stuarty.log` | the log file in `logs/` |
| `Vukem-Video-Client`, `Vukem-Video-Launcher`, `Vukem-Video-Lang` | `Stuarty-Client`, `Stuarty-Launcher`, `Stuarty-Lang` | request headers (section 11) |
| `urn:vukem-video:error:<code>` | `urn:stuarty:error:<code>` | the `type` of an HTTP problem body (SDD-003 §1.4); a client matches on `code`, which does not change |
| `vukem_video_session` | `stuarty_session` | the session cookie (section 11); a browser signs in again once |
| `vukem-video-bundle`, `vukem-video-template` | `stuarty-bundle`, `stuarty-template` | the `format` of a bundle and of a template file; a file with the former value is still imported |
| `<template id>.vukem-template.zip` | `<template id>.stuarty-template.zip` | the file name of a template download |
| `ghcr.io/alexgalhardo/vukem-video`, `alexgalhardo/vukem-video` | `ghcr.io/alexgalhardo/stuarty`, `alexgalhardo/stuarty` | the container image |

### 13.4 Names that stay

These identifiers outlive the rename on purpose, because changing them would break something that already exists on a user's machine or in a published artifact.

| Name | Why it stays |
| --- | --- |
| `vvl_`, `vvk_` (token prefixes), `.vvbundle` (bundle extension) | existing tokens, API keys and bundle files; they carry the initials, not the name |
| `vukem-video-update-feed-v1`, `vukem-video-update-artifact-v1` (the signing labels of the update feed, SDD-009 §10.5) | an installed Vukem Video verifies the feed with these labels; a feed signed under other labels would be refused, so Stuarty could not be offered to it as its update |
| `VUKEM_VIDEO_OPENH264_DIR` | read by the pinned own FFmpeg build `ffmpeg-9.0.2-2` (its patch, `.config/ffmpeg-build/patches/`); it changes with the next rebuild of that build, and the server sets it for the FFmpeg child process only |
| `https://vukem-video.dev/schema/timeline.v1.schema.json` (the `$id` of the timeline schema) | an identifier of the first release; the address of the documentation site is a setting of the author, so the identifier does not move with the name |
| `dev.vukem.vukem-video`, `dev.vukem.vukem-video.launcher` and `dev.vukem.vukem-video.desktop` (the macOS package and bundle identifiers) | the identity macOS upgrades by: a package of Stuarty with them replaces an installed Vukem Video; the Windows `AppId` GUIDs stay for the same reason |
| `--extra-version=vukem<N>` (the configure line of the own FFmpeg build) | read by `engine/ffmpeg/flavour.go` to recognise the own build; it changes with the next rebuild of that build |
| `vukem` as the OTIO metadata namespace, `X-Vukem-*` response headers, `window.vukemDesktop` | the short name of the framework era, not the product name; a wire and file format identifier, changed only by a decision of its own |

## 14. Image documents (SDD-021)

The operations of the registry of domain `image` (`core/command` registry built by `core/image`, schemas in `core/schema/image/`), applied by `ImageEdit`. The names are not in the video registry; `text.set` and `shape.set` keep their video meaning.

| Name | Kind | Meaning |
| --- | --- | --- |
| `image.create`, `canvas.resize` | operation | set the canvas of a document; resize it |
| `layer.add_image`, `layer.add_text`, `layer.add_shape`, `layer.add_fill` | operation | add a layer of that kind |
| `layer.group`, `layer.ungroup`, `layer.move` | operation | structure: group, ungroup, change order and parent |
| `layer.transform`, `layer.set`, `layer.crop`, `layer.duplicate`, `layer.remove` | operation | geometry, common properties, crop of an image layer, copy, removal |
| `layer.set_text`, `layer.set_shape` | operation | change the content or style of a text layer, the look of a shape |
| `layer.align`, `layer.fit` | operation | align to the canvas or to a layer, fit (cover, contain) |
| `layer.adjust`, `layer.blend` | operation | the adjustments of a layer (brightness, contrast, saturation, temperature, tint, hue, exposure, levels, tone curve) and its blend mode (SDD-021 section 3, epic 29) |
| `layer.filter_add`, `layer.filter_set`, `layer.filter_remove`, `layer.filter_move` | operation | the ordered filters of a layer |
| `layer.mask_set`, `layer.mask_clear` | operation | the mask of a layer: a shape, or the alpha of another layer |
| `grayscale`, `sepia`, `invert`, `warm`, `cool`, `blur`, `sharpen`, `vignette`, `grain`, `shadow`, `glow`, `stroke` | image filter kind | the `kind` of a filter of a layer, each at most once per layer (SDD-021 section 3.1) |
| `rect`, `rounded_rect`, `ellipse`, `layer` | image mask kind | the `kind` of the mask of a layer |
| `normal`, `multiply`, `screen`, `overlay`, `soft_light`, `hard_light`, `darken`, `lighten`, `color_dodge`, `color_burn`, `difference` | image blend mode | the `mode` of `layer.blend`; the renderer also reads `add` (SDD-013 section 3.11) |
| `video_thumbnail`, `square_post`, `vertical_cover` | preset | 1280 x 720, 1080 x 1080, 1080 x 1920 |
| `L_IMAGE_EMPTY_SLOT`, `L_IMAGE_TEXT_OUTSIDE`, `L_IMAGE_TEXT_SMALL`, `L_IMAGE_LOW_CONTRAST`, `L_IMAGE_UPSCALED` | lint code | the rules of SDD-021 section 8; the first blocks an export |
| `png`, `jpeg`, `webp` | image export format | the formats of `ImageExport` |
| `E_SLOT` `details.reason` `unknown_slot`, `wrong_kind`, `invalid_value`, `missing` | error detail | a template filled badly (SDD-021 section 7) |
| `E_EXPORT_FAILED` `details.reason` `webp_unavailable` | error detail | no WebP encoder is wired (SDD-021 §6) |

## 15. Export presets, codecs and batches (SDD-004 sections 8.2 to 8.4)

| Name | Kind | Meaning |
| --- | --- | --- |
| `youtube-shorts`, `instagram-reels`, `facebook-reels`, `tiktok`, `x-video` | export preset id | the five presets of RF-087, in this order in `ExportPresetList` |
| `h264`, `hevc`, `av1` | export codec | the codecs of an export; `h264` is the default |
| `X_DURATION_TOO_LONG`, `X_DURATION_TOO_SHORT`, `X_FPS_TOO_HIGH`, `X_FPS_TOO_LOW`, `X_RESOLUTION_TOO_SMALL`, `X_RESOLUTION_TOO_LARGE`, `X_ASPECT_RATIO`, `X_SIZE_TOO_LARGE` | export check finding | what `ExportCheck` reports (SDD-002 A.13) |
| `export_batch` | job kind | the parent job of `ExportBatch`; its children are `export` jobs (SDD-004 section 8.4) |
| `E_VALIDATION` `details.reason` `preset_mismatch` | error detail | an export with a preset was refused because the project does not fit it; `details.findings` |
| `E_NO_ENCODER` `details.reason` `codec_unavailable` | error detail | the codec asked for has no allowed encoder that passes the test encode on this machine |
| `preset_<finding>` | verification check | a check of the finished file against its preset, named after the finding (`preset_duration_too_long`) |
| `video_codec`, `codec_fallback`, `preset`, `notes` | export result member | the codec written (`video_codec`, as before), the codec wanted when the export fell back to `h264`, the preset id, and the notes that say so |

## 16. Asset library (SDD-011 §14)

| Name | Kind | Meaning |
| --- | --- | --- |
| `imported`, `generated`, `fetched_by_agent`, `rendered_externally` | origin kind | where an asset came from: a file the user brought, a file a provider generated, a file an agent downloaded on its own side, a file rendered by another tool |
| `visual`, `video` | library section | the two sections over one library: Visual Assets (kinds `image`, `lut`) and Video Assets (kinds `video`, `audio`) |
| `E_VALIDATION` `details.reason` `origin_kind_unknown`, `source_required`, `rights_note_required`, `provider_required`, `model_required`, `too_long`, `cost_invalid`, `tags_invalid`, `kind_unknown`, `section_unknown`, `origin_member_not_allowed`, `origin_member_invalid` | error detail | an origin, a tag set or a list filter the library refuses (`path` is `origin.<field>`, `tags`, `kind`, `section`, `origin` or `tag`); the last two refuse `engine`, `engine_version` or `render_key` on another kind than `rendered_externally`, or in a form that is not a token (SDD-011 §14.2, epic 33); stable for agents |
| `asset_hash`, `hash` | field | the content hash `sha256:<64 hex>` of an asset, the key of the library (a bare 64 hex string is accepted and normalised) |
