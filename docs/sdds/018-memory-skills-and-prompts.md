# SDD-018: Memory, skills and prompts

- Status: Draft (2026-11-20). Written by epic 15; the code follows in the same epic. Section 17 (the recipes) was added by epic 39.
- Depends on: ADR-0040 (files with an index), ADR-0031, ADR-0020 (untrusted text), ADR-0033 (no credential leaves the vault), SDD-000 (names), SDD-003 (surfaces), SDD-007 (threats T-44 to T-47, scope matrix, untrusted text)
- Implements: PRD-004 RF-385, RF-386, RF-387, RF-388, RF-390, RF-391; section 16 (the built-in skills): RF-301, RF-302, RF-389
- Owner area: `core/library/`, `service/library/`, the library operations in `service/`, the controllers, commands and tool in `web/app/`, the pages Skills, Prompts and Memory; the built-in skills in `web/resources/skills/` (section 16), the recipe templates in `web/resources/templates/` and the briefing question sets per kind in `core/project/` (section 17)

## Contents

1. Overview
2. Layout of the files
3. Front matter
4. Names
5. Memory
6. Prompts
7. Skills: origin, review and the lint
8. Import and export of skills
9. The index and the scan
10. Search
11. Service operations and scopes
12. Errors, limits and files
13. Untrusted text
14. Surfaces
15. Tests
16. The built-in skills
17. Recipes: a product commercial and a long talking video

## 1. Overview

The way a creator works is kept as plain files they own: **memory** (what was learned), **skills** (procedures an agent follows, in the Agent Skills format) and **prompts** (reusable texts with variables). The files are the truth (ADR-0040 item 1); SQLite holds only an index for search and for the lists, and the index can be deleted and rebuilt from the files.

Three rules carry the design:

1. **A skill is text an agent reads.** Stuarty never runs a script that comes with a skill. A script is stored as an inert file and flagged.
2. **An outsider's skill is untrusted until a person has read it.** An imported skill starts unreviewed, is listed with a warning and is not offered to an agent until a person marks it reviewed. A change to its text takes the mark away.
3. **Everything that comes out of the library toward an agent is delimited as untrusted text** (section 13), even what the user wrote: a note can have been written by another agent.

The rules (names, front matter, variables, the lint, limits) are in `core/library/` as pure functions. `service/library/` holds the files, the scan, the import and the export. The index is a repository behind an interface.

## 2. Layout of the files

```text
<workspace>/library/
├── .review-key                     the key that seals the review marks (section 7); made at the first review
├── memory/<name>.md                one fact per file
├── prompts/<name>.md               one prompt per file
└── skills/<name>/                  the Agent Skills layout
    ├── SKILL.md                    required: front matter and instructions
    ├── references/ ...             optional text files the instructions point to
    ├── assets/ ...                 optional files
    └── scripts/ ...                optional; stored inert and flagged, never run
```

The folder is created at the first write. Every path is resolved under `library/` with `os.Root`, so no name, link or file of a skill can lead out of it. A symbolic link inside `library/` is never followed and never indexed.

## 3. Front matter

A file starts with `---` on its own line, the front matter, `---` on its own line, then the Markdown body. The front matter is a **subset of YAML** that the product parses itself (no YAML library, so no anchors, tags, merge keys or custom types):

- top-level `key: value` pairs; a key is `[A-Za-z][A-Za-z0-9_-]*` and may appear once;
- a value is a plain scalar, a `"double quoted"` string (escapes `\"` `\\` `\n` `\t`), a `'single quoted'` string, a flow list `[a, b]`, a block list (`- a` lines), a one-level block map (indented `key: value` lines, scalar values), or a block scalar `|` or `>` (folded to one string);
- a value that starts with `&`, `*`, `!`, `%`, `@` or a backtick, a nested map deeper than one level, a map inside a list, and a front matter over 8 KiB are refused with `E_VALIDATION` and the line;
- unknown keys are kept and written back unchanged.

The product writes the front matter itself, with the keys in the order of the tables below, so a rewrite of an unchanged file is byte for byte the same.

## 4. Names

A name is the file or folder name: lower case letters, digits and single hyphens, `^[a-z0-9]+(-[a-z0-9]+)*$`, at most 64 characters (the Agent Skills rule). The Windows device names (`con`, `prn`, `aux`, `nul`, `com1` to `com9`, `lpt1` to `lpt9`) are refused too, so a library moves between systems. A name is never a path: a request with a separator, a dot, an empty name or a drive letter is `E_VALIDATION` with `details.path` `name`. Within a kind a name is unique; across kinds the entry is `<kind>/<name>` (`memory/`, `skill/`, `prompt/`).

## 5. Memory

One fact per file, `library/memory/<name>.md`.

| Key | Meaning |
| --- | --- |
| `name` | the file name, required |
| `description` | one line: the fact, required, at most 200 characters |
| `scope` | `workspace`, or a project id (`prj_...`), default `workspace`; for a key limited to a project the default is that project, the only scope it may write |
| `tags` | a list of at most 10 tags, each a name |
| `actor` | who wrote it (SDD-000 section 3); set by the service from the caller, never from the request |
| `kept` | `true` when a person kept it; a note written by an agent starts `false` |
| `created`, `updated` | UTC times, RFC 3339; set by the service |

The body is the detail, at most 8 KiB. A person at the browser writes a note that is kept at once. A note by any other actor is `kept: false` and is listed as pending until a person keeps it (`MemoryWrite` with `kept` true, from the browser only). An agent cannot rewrite a kept note: it can only add a new one (`E_FORBIDDEN`), so flooding cannot overwrite what a person decided to keep.

Limits: at most 2000 notes in the workspace; at most 50 new notes per actor per UTC day (a person at the browser is not limited); both are `E_LIMIT_EXCEEDED`. The day count is derived from the `actor` and `created` of the files, so it survives an index rebuild. RF-386: the flow reads memory before planning a production (`LibrarySearch` with `scope`) and writes it after.

## 6. Prompts

One prompt per file, `library/prompts/<name>.md`.

| Key | Meaning |
| --- | --- |
| `name` | the file name, required |
| `description` | one line, required, at most 200 characters |
| `tags` | at most 10 |
| `defaults` | a map from variable to its default value; a `PromptWrite` with no `defaults` keeps those of the prompt, and an empty map removes them |
| `created`, `updated`, `actor` | as in section 5 |

The body is the text, at most 32 KiB. A variable is `{{name}}` or `{{ name }}` with a name `[a-z][a-z0-9_]{0,39}`. `PromptRender` takes values and returns the text with each variable replaced once: a value is **never** scanned for variables again, so a value cannot inject another one. A variable with no value and no default is `E_VALIDATION` naming the variable (`details.missing`); a value for a variable the text does not use is also refused (`details.unknown`), so a typo does not pass silently; a value is at most 8 KiB. `{{` that does not form a valid variable is left as it is. The rendered text is the result and goes to the caller; rendering sends it nowhere.

## 7. Skills: origin, review and the lint

`library/skills/<name>/SKILL.md`. The standard fields of the Agent Skills format are `name` (equal to the folder name), `description` (at most 1024 characters), `license`, `compatibility` (at most 500) , `allowed-tools` and `metadata` (a map of strings). The product keeps its own fields in `metadata`, so the file stays valid anywhere else:

| `metadata` key | Meaning |
| --- | --- |
| `stuarty-origin` | `built-in` (shipped with the product), `user` (written here) or `imported` (RF-387) |
| `stuarty-source` | free text: where it came from (a repository, a folder, an author) |
| `stuarty-reviewed` | `hmac-sha256:<hex>`: the content hash a person read, sealed with the review key of the library, or absent |

**Reviewed** is not a flag a file can claim: a skill is reviewed only when `stuarty-reviewed` equals the HMAC-SHA256, under the review key, of the content hash computed now. The content hash is `sha256:` and the SHA-256 of the canonical front matter without `stuarty-reviewed`, the body and every other file of the folder (path, length and bytes, in path order). Any edit, from any tool, takes the review away. The review key is 32 random bytes in `library/.review-key` (hex, file mode 0600), made at the first review of the workspace; a dot file is never scanned, imported or exported. A plain hash would not be enough, because anyone can compute it: a skill folder copied into the workspace by hand, or shipped by a repository with the hash of its own content, would pass as read. With the key, a skill file placed in the workspace by hand is unreviewed until a person reviews it, whatever it says. A key that is missing or not valid is replaced at the next review, which takes every earlier review away: the safe side. Copying the whole `library/` folder, key included, to another machine keeps the reviews.

- `SkillWrite` always records the origin `user`; the caller cannot set `built-in` or `imported`. A skill written by a person at the browser is reviewed at once (they wrote it); one written through any other surface is not. The skill that exists is the start of the write: its front matter keys the product does not know are written back, and a request with no `metadata` (or no `files`) keeps the metadata (or the files) the skill has; an empty map (or list) removes them.
- `SkillImport` records `imported` and the licence and source it finds or is given, and never reviews.
- `SkillMarkReviewed` is for a person at the browser: a caller with the `edit` scope and `Caller.Browser`, which is a signed-in session of network mode or a session of the desktop application opened with the launcher secret (SDD-007 section 3.4 item 7). A local web session without the secret is refused, because an agent that holds the token can open one. The request carries the `content_hash` that `SkillGet` returned with the text the person read; when the skill changed since, the mark is refused with `E_REVISION_CONFLICT` and the person reads it again, so a text swapped while the person was reading the old one is never marked. It is refused while the lint reports an error. A built-in skill is installed by the product reviewed, and is `built-in` only while it is the text the product installed (section 16).
- The default agent listing (`SkillList` without `include_unreviewed`, and every skill an agent reads through search) holds only reviewed skills. `SkillGet` of an unreviewed skill is allowed (a person must be able to read it) and its result says `reviewed: false` and carries the warning text.

### 7.1 The skill lint

`SkillLint` returns findings `{code, severity, path, line, message}`; `error` blocks `SkillWrite` and `SkillMarkReviewed`, `warning` informs. The codes are fixed and each has a passing and a failing fixture in `core/library`:

| Code | Severity | Fires when |
| --- | --- | --- |
| `L_SKILL_NO_SKILL_MD` | error | the folder has no `SKILL.md` |
| `L_SKILL_FRONT_MATTER` | error | the front matter is missing, not closed or not valid (section 3) |
| `L_SKILL_NAME` | error | `name` is missing or not a valid name |
| `L_SKILL_NAME_MISMATCH` | error | `name` differs from the folder name |
| `L_SKILL_DESCRIPTION` | error | `description` is missing or empty |
| `L_SKILL_DESCRIPTION_LENGTH` | error | `description` is longer than 1024 characters |
| `L_SKILL_BODY_EMPTY` | error | the body has no text |
| `L_SKILL_REFERENCE_MISSING` | error | a relative link or a `references/`, `assets/` or `scripts/` path in the text names a file the folder does not have |
| `L_SKILL_REFERENCE_ESCAPES` | error | a relative link climbs out of the folder (`..`) or is absolute |
| `L_SKILL_CREDENTIAL` | error | the text tells the agent to ask for, read, print or send a credential: an API key, token, password, secret, private key, `.env`, `vault.json`, the master key, an SSH key |
| `L_SKILL_REMOTE_EXEC` | error | the text tells the agent to pipe a download into a shell (`curl ... \| sh`) |
| `L_SKILL_HIDDEN_TEXT` | error | the text holds characters a person cannot see and a model reads (SDD-007 section 8.2: zero width, bidirectional controls, tag characters) |
| `L_SKILL_SIZE` | error | `SKILL.md` is over 64 KiB, a file is over 512 KiB, the folder is over 2 MiB or has over 100 files |
| `L_SKILL_HTML_COMMENT` | warning | the text has an HTML comment (a place to hide an instruction) |
| `L_SKILL_SCRIPT` | warning | the folder has a script (a file under `scripts/` or with a script extension): it is stored inert and never run |
| `L_SKILL_ALLOWED_TOOLS` | warning | `allowed-tools` is set: in another agent it pre-approves tools |
| `L_SKILL_LICENSE` | warning | an imported skill names no licence |

## 8. Import and export of skills

**Import** (RF-388) reads from a local folder or an archive (`.zip`, `.tar.gz`, `.tgz`) on the machine of the server, through the import path rules of SDD-007 section 7.1 (the same guard as a media file: import roots in network mode, never a file of the workspace). Stuarty opens **no connection** to fetch a skill: the user or their agent downloads it first.

The skill is read in memory and checked before anything is written:

1. The folder or the archive holds exactly one skill: `SKILL.md` at its root, or one top-level directory that holds it. Any entry outside that one directory is refused. The folder the Finder adds (`__MACOSX/`) and `.DS_Store` files are ignored; a path used both as a file and as a folder is refused.
2. The name of an entry, and of a file written to a skill folder, is printable ASCII (so no two names fold together and none hides a character). An entry is refused when its name is absolute, has an empty, `.` or `..` component, a backslash, a colon (a drive letter), a character that is not printable ASCII, a character Windows refuses in a name, a Windows device name, or when two names collide after case folding; when it is a symbolic link, a hard link, a device or encrypted; or when it is nested archive content that needs another read. A refusal names the entry and writes nothing.
3. Limits: at most 100 files, 512 KiB per file, 2 MiB in total, counted on the bytes actually read (a header that lies cannot exceed them), an archive file of at most 8 MiB. A file over the limit is refused by name, from a folder as from an archive, never left out of the skill.
4. The lint runs. A structural error (no `SKILL.md`, a front matter that does not parse, an invalid name) refuses the import; the other findings are stored with the skill and shown. A name that is already taken is `E_ID_EXISTS` unless `replace` is true, and `replace` of a kept user skill needs a person at the browser.
5. The folder is written under a temporary name and renamed into place, so a failed import leaves nothing. The origin is `imported`, the source is the text given or the path's base name, the licence is the one the front matter names; `stuarty-reviewed` is removed whatever the file said.

Files are copied as bytes; none is executed, opened by another program or given the executable bit.

**Export** (`SkillExport`) writes one skill, or every reviewed skill, as folders in the Agent Skills layout under `<workspace>/exports/library/<name>/` (SDD-007 section 7.5: no destination argument), without the `stuarty-reviewed` mark, with `stuarty-origin` and `stuarty-source` kept so a later import still shows where the skill came from. The result gives the path to a local caller. The exported folder passes `SkillLint` when imported again.

## 9. The index and the scan

The index is two tables (migration `20261119130000_create_library_index`): `library_entries` and the full-text table `library_fts`. It is rebuilt from the files: `LibraryReindex` empties it and reads every file; a **scan** compares the files with the rows (`sig`) and re-reads what differs, adds what is new and drops what is gone. The scan runs on the first read after the process starts and at the start of every list, get and search (a few hundred files cost a few directory reads), so a file edited outside the product is found at the next call (RF-391). A file that does not parse is indexed as `invalid` with its error, never skipped silently, and shown in the list with the reason. A request that reads a file reads the file, never the index.

| Table | Columns |
| --- | --- |
| `library_entries` | `key` (primary key, `<kind>/<name>`), `kind`, `name`, `description`, `scope`, `origin`, `reviewed` (0 or 1, computed at the scan from the content hash), `kept` (0 or 1), `actor`, `tags` (JSON array), `state` (`ok` or `invalid`), `error`, `path` (relative to `library/`), `sig` (file count, bytes and latest modification time of the file or folder, what the scan compares), `lint_errors`, `lint_warnings`, `has_scripts` (skills), `created_ms`, `updated_ms` |
| `library_fts` | FTS5 over `key` (not indexed), `name`, `description`, `tags`, `body`; the tokenizer is `unicode61` with `remove_diacritics 2` |

## 10. Search

`LibrarySearch` runs one full-text query over the three kinds. The query is split into terms (lower case letters and digits, at most 8 terms of at most 64 characters); every term must match as a prefix in the name, description, tags or body. FTS syntax in the query is never passed through: a term is quoted. Filters: `kinds`, `scope` (memory), `tags`, `origin`, and `include_unreviewed` (default false: unreviewed skills and pending notes are left out of an agent's results; the browser sees them). Results are ordered by relevance, then name.

The answer fits a budget: `budget` characters (default 4000, at most 16000, about 1250 tokens at most); each hit carries a snippet of at most 300 characters around the first match, hits are added while the budget holds, and `truncated` says that more existed. Every text of a hit is untrusted (section 13).

## 11. Service operations and scopes

| Operation | Scope | Further rule |
| --- | --- | --- |
| `MemoryList`, `MemoryGet`, `SkillList`, `SkillGet`, `PromptList`, `PromptGet`, `LibrarySearch`, `SkillLint`, `PromptRender` | read | lists and searches leave out unreviewed skills and pending notes unless `include_unreviewed` is true (HTTP and CLI take it; the MCP tool has no such argument, and its calls are made with `agent_view`, which also refuses `SkillGet` of an unreviewed skill with `E_FORBIDDEN` and the hint to ask the person to review it) |
| `MemoryWrite`, `MemoryRemove`, `SkillWrite`, `PromptWrite`, `LibraryReindex` | edit | the rules of sections 5 to 7 |
| `SkillImport`, `SkillExport` | media | reads or writes files by path rules; no MCP tool |
| `SkillMarkReviewed` | edit | a person at the browser only (the session cookie), with the `content_hash` of the text read; no MCP tool and no CLI command |

In network mode the library belongs to the workspace and is shared by its users. A key limited to one project may read and write only memory whose scope is that project or `workspace` (read only). `MemoryRemove` deletes the file; a caller that is not a person at the browser cannot remove a kept note (`E_FORBIDDEN`). Every write, import, export, review and removal appends an audit row (`library.memory.write`, `library.skill.import`, and so on) with the actor.

## 12. Errors, limits and files

No new error code: `E_VALIDATION` (a name, a field, a variable, an archive entry; `details.findings` for the lint), `E_NOT_FOUND`, `E_ID_EXISTS` (a taken name), `E_FORBIDDEN`, `E_REVISION_CONFLICT` (a review of a skill that changed since it was read), `E_LIMIT_EXCEEDED`, `E_PAYLOAD_TOO_LARGE`, `E_PATH_NOT_ALLOWED` and `E_FILE_NOT_FOUND` (import paths).

| Limit | Value |
| --- | --- |
| notes in the workspace; new notes per actor per day | 2000; 50 |
| memory body; prompt body; a variable value | 8 KiB; 32 KiB; 8 KiB |
| skills; prompts | 500; 500 |
| a skill: `SKILL.md`; a file; the folder; the files | 64 KiB; 512 KiB; 2 MiB; 100 |
| an archive file; the front matter | 8 MiB; 8 KiB |
| search: terms; hits; budget | 8; 50 (default 20); 16000 characters (default 4000) |

Files: `library/` in the workspace (SDD-000 section 9), `exports/library/` for exported skills. Tables: `library_entries`, `library_fts` (section 9). Identifiers: none; an entry is `<kind>/<name>`.

## 13. Untrusted text

The pages edit the text of an entry, so the service returns it to a session of the UI (a person at the browser, or a session of local mode opened without the launcher secret, actor `local:ui`) exactly as the file has it (the page escapes it when it draws it): a page that got the cleaned text would write it back over the file on its next save. Every other caller gets the text cleaned. Every text of the library that reaches an agent is untrusted: a description, a body, a snippet, a tag, a source and a rendered prompt. The fields are tagged `untrusted:"true"` and listed in the provenance table of SDD-007 section 8.1 with the source `library_text`. In the text part of a result the library text is a block `<untrusted_media_text source="library_text" ...>` with its `kind` and `name`; in structured content it is a plain string cleaned and with `<` escaped (`untrusted.Value`). The names, kinds, flags and counts are the product's own and are not. The paths of the files of a skill are not the product's (an archive names them): in the text of `SkillGet` and `SkillLint` the file list and the findings are blocks of their own (`field` `files` and `findings`). A skill's `SKILL.md` is shown whole to a person who asks for it: the warning for an unreviewed skill is a sentence of the product outside the block.

## 14. Surfaces

| Operation | MCP | CLI | HTTP (under `/api/v1`) |
| --- | --- | --- | --- |
| `MemoryList`, `MemoryGet`, `MemoryWrite` | `library` (kind `memory`) | `memory:list`, `memory:show`, `memory:write` | `GET /library/memory`, `GET /library/memory/{name}`, `POST /library/memory`, `DELETE /library/memory/{name}` |
| `MemoryRemove` | none: no MCP tool removes anything | `memory:remove` | `DELETE /library/memory/{name}` |
| `SkillList`, `SkillGet`, `SkillWrite`, `SkillLint` | `library` (kind `skill`) | `skill:list`, `skill:show`, `skill:write`, `skill:lint` | `GET /library/skills`, `GET /library/skills/{name}`, `PUT /library/skills/{name}`, `GET /library/skills/{name}/lint` |
| `SkillImport`, `SkillExport`, `SkillMarkReviewed` | none | `skill:import`, `skill:export`, none | `POST /library/skills/import`, `POST /library/skills/export`, `POST /library/skills/{name}/review` |
| `PromptList`, `PromptGet`, `PromptWrite`, `PromptRender` | `library` (kind `prompt`) | `prompt:list`, `prompt:show`, `prompt:write`, `prompt:render` | `GET /library/prompts`, `GET /library/prompts/{name}`, `PUT /library/prompts/{name}`, `POST /library/prompts/{name}/render` |
| `LibrarySearch`, `LibraryReindex` | `library` (action `search`) and none | `library:search`, `library:reindex` | `GET /library/search`, `POST /library/reindex` |

`MemoryWrite` is a `POST` that creates the note or, when `name` names one, replaces it; the name may be left out and is then made from the description. Skills and prompts are written with `PUT` on their name. The lists and the search take their filters as query parameters (`tags` and `kinds` as comma separated words, `include_unreviewed` and `agent_view` as booleans). `agent_view` is set by every call of the MCP tool and can only narrow what a call sees: it leaves out what is not reviewed or kept whatever `include_unreviewed` says, and `SkillGet` and `MemoryGet` refuse an entry that is not.

The MCP tool `library` takes `action` (`search`, `list`, `get`, `write`, `render`, `lint`) and `kind`; its definition is in SDD-003 section 4.2. The pages are `/memory`, `/skills` and `/prompts` of the dashboard (SDD-012 section 23): a list with a search box and the filters, an editor with a preview, and for a skill the origin, licence, source, the lint findings and the review state with the button for a person to mark it read. The pages compute nothing: the lint, the rendering and the review are the answers of the service.

## 15. Tests

- `core/library`: the front matter parser and writer (a table of valid and hostile inputs; write then parse is the identity), names, variables and rendering, the content hash, and every lint code on a failing and a passing fixture.
- `service/library`: files, scan, a file edited outside, deleting the index and rebuilding it, memory limits, prompts, search and its budget, skills, import from a folder and from archives, export and import again.
- Security (SDD-007 T-44 to T-47): `TestSecurity_T44_*` to `TestSecurity_T47_*`.
- Contract: parity of every operation on HTTP, CLI and MCP, and the untrusted marking.
- End to end: `web/e2e/specs/library.spec.ts`.

## 16. The built-in skills

The product ships a small set of skills written for it and tested with it (epic 16, RF-389), so an agent and a creator are led through the first complete flow: a short vertical video with captions and a thumbnail, from the briefing to the publication (ADR-0031). They are Agent Skills folders in `web/resources/skills/<name>/` (`SKILL.md`, optional `references/`), embedded in the binary with `go:embed` (package `web/resources/skills`) and handed to the service as `service.Options.BuiltInSkills`; `service/library` has no knowledge of where they come from.

**The set.** `start-here` (a router: which skill for which request), `brief-a-production`, `short-vertical-video`, `captions-for-shorts`, `thumbnail`, `recall-learnings`, `production-retro`, `fetch-clip`, and the writing skills the first flow needs, adapted from the two sources the author approved (D-071): `write-a-hook`, `short-script` and `caption-per-network`, and `captions-for-shorts` carries a model of the second source. A step that depends on an operation that does not exist yet is marked in the text (`Not available yet:`) and is finished by the epic that adds the operation.

**`motion-with-an-external-engine` (epic 33, RF-084).** A skill for an animation the editor cannot make (SDD-011 section 15): get the brief (`view` with `mode` `motion`, or `motion:brief`), render on the agent's own side with an engine the user installed (HyperFrames is the one it names, adapted from its `hyperframes-cli` skill: lint, check and look at the snapshots before rendering, confirm the duration of the file after), import the file with origin `rendered_externally`, a `render_key` and the engine and its version, check it against the brief (`motion:check` in the CLI, or `media_inspect` compared with the brief over MCP), then place it with `clip.add` or, when the key already exists, replace the clips with `clip.replace_media` in one `edit` call. The skill says that Stuarty never runs, installs or downloads the engine, and names the version of the engine its steps were written against; an engine whose command line differs is the agent's to read (`--help`). Its source and licence are named in its front matter and its notice is in `licenses/vendored/NOTICE-skills.md`, like the other adapted skills. It is within the 12,400 characters of an agent `SKILL.md`.

**Install and update.** When the service starts it reads the embedded folders and, for each one, in this order:

1. The folder `library/skills/<name>/` does not exist: it is written as the embedded text with `stuarty-origin: built-in`, sealed as reviewed (the product wrote and read it), and its content hash and the product version are recorded in `library/.built-in.json`.
2. The folder exists and its content hash is the one recorded (the folder is as the product left it): when the embedded text now differs, the folder is rewritten the same way and the record updated; when it does not, nothing is written, so an unchanged skill keeps its file times.
3. The folder exists and its hash is not the recorded one (a person edited it, or it was never installed by the product): it is **never overwritten**. It reads as `user`.

`library/.built-in.json` is a dot file: no scan, import or export reads it, and it maps a name to `{version, hash}`. The embedded text stays in the binary, so a person who wants a built-in skill back deletes their folder and starts the product again (the record of a folder that is gone is dropped and step 1 runs). A skill that a later binary no longer ships is left as it is.

**The origin is not a claim of the file.** `stuarty-origin: built-in` in a `SKILL.md` counts only while the content hash of the folder is the hash recorded in `library/.built-in.json`; otherwise the skill is listed as `user`. A folder copied in by hand, an imported one and an edited built-in therefore never read as `built-in`, whatever their front matter says; a record that is missing or damaged takes the origin away (the safe side) and the next start repairs it for the folders that are untouched.

**Every skill names real operations.** A test (`go test ./web/resources/skills/...`) reads every built-in skill, extracts the names of operations, MCP tools and CLI commands the text mentions in code spans and checks them against the registry of SDD-000 (the Service operations, the MCP tools of SDD-003 section 4.1, the CLI commands and the HTTP routes), so a renamed operation breaks the build instead of the agent. Names of operations of later epics are written in a line that starts `Not available yet:` and are not checked until that epic removes the line.

**Static rules for every built-in skill**, in the same test: it passes `SkillLint` with no finding of any severity; it names no credential to ask for or read; it never tells the agent to approve a publication or a cost, to publish without approval, or to bypass the approval queue (checked as imperative phrases: `approve it yourself`, `publish without`, `skip the approval`, `bypass`); and where it says to publish or to generate at a cost it names the approval queue as the way.

**Adapted material (D-071).** Two sources are approved, with attribution and adaptation: `coreyhaines31/marketingskills` (MIT) and `heygen-com/hyperframes` (Apache-2.0). A skill adapted from one names it in its front matter: `license` (the licence of the source), `metadata.stuarty-source` (the repository, the path of the original and the commit read, as `owner/repo path @ <40 hex characters>`) and `metadata.stuarty-adapted-from` (the source id below). A skill that is not adapted names none of the three. The licence text of each source is in `licenses/vendored/<source id>-<SPDX id>.txt` (the notices tool has fixed groups, and adapted text is source kept in the repository, so it is the `vendored` group), listed in `.config/third-party.json` and in `THIRD_PARTY_NOTICES.md`, with `licenses/vendored/NOTICE-skills.md` listing each adapted skill, its source path and what was changed; `task notices:check` fails when the inventory and the folder differ. A test checks that every adapted skill names a source whose licence text is in `licenses/vendored/`. Nothing is copied from a source whose licence was not read at that commit.

| Source id | Repository | Licence |
| --- | --- | --- |
| `marketingskills` | `coreyhaines31/marketingskills` | MIT |
| `hyperframes` | `heygen-com/hyperframes` | Apache-2.0 |

**Agent-side only.** `fetch-clip` tells the agent to download with its own tools on its own side and then to import the file with a source and a rights note (`MediaImport`); the product gains no downloader and opens no connection (ADR-0032). **Recall and retro.** `recall-learnings` reads memory (`LibrarySearch` with `scope`) at the start of a production and `production-retro` writes what was learned at the end with `MemoryWrite`, which an agent's note starts unkept (section 5).

## 17. Recipes: a product commercial and a long talking video

Epic 39 (ADR-0040, ADR-0031, ADR-0042, RF-389) adds two more kinds of production after the short vertical video. **A recipe is a skill plus templates plus briefing questions, and nothing else**: it names operations that exist, a gap becomes a product issue and the skill says what the person does by hand, and it adds no command, no table and no tool.

**The production kind.** `core/project` names three kinds of production, `short`, `commercial` and `long` (`ProductionKind`), and the questions of the briefing are data per kind: `BriefingQuestionsFor(kind)`, `CheckAnswersFor`, `BriefingMarkdownFor` and `ParseBriefingFor`. `short` is the set of section 5.12.1 of SDD-005 (`BriefingQuestions` and the other functions without a kind are its names, unchanged). A brief that one kind wrote is not read as the brief of another: its headings are not the headings of the other set, so `ParseBriefingFor` says it was not written by that briefing, and a brief written by hand is never replaced by accident (SDD-005 section 5.12.1). The kind is not stored: the questions that a brief answers say which kind it is. The service operations `BriefingQuestions` and `BriefingAnswer` serve the set of `short` only; for another kind the skill asks the questions of its set and writes the brief with `brief:set` (or `project_item` with `action` `brief`), in the schema of section 5.12.1 (`# Brief`, then `## <heading>` and the answer for each answered question, in the order of the set). Serving the other sets through the briefing operations is a product issue, not part of this epic.

| Question id (heading) | `commercial` | `long` |
| --- | --- | --- |
| `product` (Product), `problem` (Problem), `offer` (Offer), `call_to_action` (Call to action) | required text | no |
| `topic` (Topic), `speakers` (Speakers), `goal` (Goal) | no | required text |
| `audience`, `platform`, `format` | required (the platforms and the formats of the kind) | required |
| `duration` (Duration (seconds)) | required, 5 to 90 | required, 120 to 14,400 |
| `cut_style` (Cut style: `tight`, `natural`, `relaxed`) | no | required choice |
| `other_formats`, `product_shots`, `proof` | optional text | no |
| `chapters`, `keep`, `short_from_it` (`yes` or `no`) | no | optional |
| `tone`, `references`, `must_include`, `must_avoid`, `deadline` | optional | optional |

The test is `go test ./core/project/... -run TestBriefingKinds`: every set is well formed (unique ids and headings, bounds, choices with options), a brief written by a kind reads back as it was written and only for that kind, the required answers and the bounds of the duration are enforced per kind, and an unknown kind is `E_VALIDATION` with `path` `kind`.

**The skills** (`web/resources/skills/`, the rules of section 16 apply to each):

- `product-commercial`: the whole path of a commercial of 15 or 30 seconds, in the order hook, problem, product shots, offer, call to action; one brief, one timeline made from a template, and a version per aspect ratio with `settings.reframe` and an export of each (9:16, 1:1, 16:9). Motion that the editor cannot make goes through `motion-with-an-external-engine`.
- `commercial-copy`: the words of a commercial, adapted from the marketing set (source `marketingskills`, the hook and short-form video references of its ad creative skill): the hook has three parts that do different work (what is seen, what is said, what is read), the seconds after it must continue its promise, and every claim traces to the brief, so a number, a review or a guarantee that the brief does not hold is never written.
- `long-talking-video`: the edit of a long video of a person speaking: the dead air and the false starts out (`clip.remove_silences`, `item.split` and `item.delete` on what the person marks as a retake), the chapters as markers (`marker.add`) and as the chapter list that goes in the description, the captions from the transcript, cut-away images over the talking head, and a short extracted from the long one in a duplicate of the project (`project:duplicate`) reframed to 9:16.
- `chapter-titles`: the titles of the chapters and the text of the chapter list, written from the transcript; a title says what the viewer gets, not the order of the talk.

`start-here` routes the two kinds of request to them, and a request for a short stays with `short-vertical-video`.

**The templates** are documents without media, built from commands by the generator test of their folder (never written by hand), embedded in the binary by the package `web/resources/templates` and installed into the workspace as built-in, like the starter templates of ADR-0022 item 8 (`service.SeedBuiltInTemplates`): each has a fixed identifier, a workspace that holds it is left as it is, and one that a migration or a person changed is never overwritten. The timeline templates are in the library of templates (`template:list`), the image templates in the library of images with `template` true (`image:list --templates only`). In network mode only the timeline templates are installed: an image belongs to one user (SDD-021), so a shared image template is a product issue.

| Template | Kind | What it holds |
| --- | --- | --- |
| `commercial-15s` | timeline, 9:16 | the five beats in 15 seconds: hook, problem, product, offer, call to action; the words are text slots, the shots and the music media slots |
| `commercial-30s` | timeline, 9:16 | the same five beats in 30 seconds, with two product shots and a proof card |
| `long-video-opening` | timeline, 16:9 | a cold open, the title card, a lower third with the name of the speaker, music under the opening; the talking head follows it |
| `commercial-cover-vertical`, `commercial-cover-landscape`, `long-video-thumbnail` | image | the matching covers: a picture slot, a title text slot and the colours of the brand |

The colours of a template are named (`brand`, `ink`, `accent`), so that `template:new --brand-color <name>` takes the colour of the brand kit (SDD-005 section 5.11). **Golden exports.** Every template has a golden file in its folder: for a timeline template, the SHA-256 of the pixels of some frames of its instance filled with fixture images (the way of the golden frames of `engine/export`), and for an image template the PNG its instance renders. `go test ./web/resources/templates/...` fails when a change in the core, the renderer or the template changes a pixel; `-update` rewrites the files and every rewrite is a reviewed change.

**Each recipe is proven like the pilot** (SDD-008): `scripts/recipe-commercial.sh` and `scripts/recipe-long-video.sh`, with no network and the fake AI provider on loopback of the first flow, each in its own workspace and port, end in files that are checked with `ffprobe`: the commercial in three aspect ratios made from one brief, the long video with its chapters, its captions and the short extracted from it. Both are part of `verify.sh --full`.

**Adapted material** follows section 16: `commercial-copy` names `marketingskills` in `license`, `metadata.stuarty-source` and `metadata.stuarty-adapted-from`, and `licenses/vendored/NOTICE-skills.md` lists it with what was changed. `product-commercial`, `long-talking-video` and `chapter-titles` are written for Stuarty and name no source.

**Known gaps, kept as product issues.** A marker is a place on the timeline: the exported file carries no chapter list, so the chapters travel as the text of the description. The briefing operations serve the `short` set only. The image templates are not shared in network mode. The transcript of a long video is the work of the transcription helper; the scripted run uses the stub of the first flow.
