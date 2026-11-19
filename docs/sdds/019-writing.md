# SDD-019: Writing: scripts, copy and captions

- Status: Draft (2026-11-19). Written by epic 17; the code follows in the same epic.
- Depends on: ADR-0031 (writing is a domain of the same core), ADR-0039 (a project is a production), ADR-0036 (the user brings the provider), ADR-0020 (untrusted text), ADR-0032 (generation waits for approval), SDD-000 (names), SDD-003 (surfaces), SDD-005 section 5.12 (items), SDD-015 (approval and cost), SDD-016 (providers)
- Implements: PRD-004 RF-350, RF-351, RF-352, RF-354 (the script structure that a timeline will follow: RF-352 stops at the text; turning it into a timeline is out of scope)
- Owner area: `core/writing/`, the text operations in `service/`, the controllers, commands and tool arguments in `web/app/`, the page Write in `web/resources/js/pages/Write/`

## Contents

1. Overview
2. The text item
3. Revisions
4. The script
5. The caption and the table of limits
6. Drafting with a model
7. Service operations and scopes
8. Surfaces
9. Errors and limits
10. Untrusted text
11. The page Write
12. Tests

## 1. Overview

A creator writes the script of a video, the copy of an advertisement and the caption of each network inside the project, by hand or with a model. The text is an item of the production (SDD-005 section 5.12), so it is exported with the project, appears in its description and is found next to the timeline it will become and the post it will go out with.

Three rules carry the design:

1. **Every rule is in `core/writing/`**: the structure of a script, the estimate of its duration, the limits of a network, the validation of a caption, the difference between two revisions. The service stores and calls; the UI shows what the core said and computes no count (SDD-012 section 1).
2. **A save never loses what was there.** Every change is a revision with its actor; a person can see what an agent changed and go back. A model never overwrites: its draft is a new revision marked as generated.
3. **What a model or another actor wrote is untrusted text** (section 10), and a model is reached only through the provider layer, under an estimate, a ceiling and an approval (SDD-015, SDD-016).

Out of scope (epic 17): blog posts and their destinations (epic 23), turning a script into a timeline automatically, and the subtitle captions of a video, which stay in the video domain. `caption` here is the text that goes out with a post.

## 2. The text item

A text is a project item of kind `text` (SDD-005 section 5.12) with a **text kind**:

| Text kind | Holds | Structure |
| --- | --- | --- |
| `script` | what is said and shown in a video | scenes, narration and on-screen text (section 4) |
| `copy` | free text: an advertisement, a commercial, a newsletter | none |
| `caption` | the text that goes out with a post | one text with variants per network (section 5) |

The body is Markdown, at most 256 KiB (`project.MaxText`). The text kind is fixed when the text is made: `ProjectItemAdd` with kind `text` and no `text_kind` makes a `copy`, and `TextWrite` without an `item_id` makes the item. Changing the kind of a text is not an operation: the structure a kind adds is read from the body, so a person who wants a caption from a copy copies the body.

Storage: `project_items.text_kind` (`script`, `copy` or `caption`; null for any other kind of item) and the table `text_revisions` (section 3). `project_items.body` always holds the body of the latest revision, so the production state (`HasScript`), the description of a project and the bundle read one place.

`ProjectDescribe` lists every text with its kind, its words and its revision (`text_kind=script words=212 rev=4`), and the production state counts the texts (`texts`). The bundle of a project carries the body and the text kind of each text; the history of revisions does not travel, and an imported text starts at revision 1.

## 3. Revisions

`text_revisions` has one row per save:

| Column | Meaning |
| --- | --- |
| `item_id`, `revision` | the text and the number of the revision, 1, 2, 3 ... without gaps; together the key |
| `body` | the complete Markdown of the revision |
| `actor` | who saved it (SDD-000 section 3) |
| `source` | `write` (a person or an agent saved it), `generated` (a model drafted it, section 6), `revert` (it brought an older revision back), `created` (the text was made with this body) |
| `reverted_to` | for `revert`, the revision whose body it brought back |
| `provider`, `model`, `cost_micros`, `currency`, `job_id` | for `generated`: the provider and the model that wrote it, the real cost and the job (SDD-016 section 6); null for the others |
| `created_ms` | when |

Rules, in `core/writing`:

- A write whose body equals the body of the latest revision makes no revision and answers with the latest one (a save that changes nothing is not a revision).
- `TextRevert` never rewrites history: it makes a **new** revision, the last one, with the body of the chosen revision and `source` `revert`. Reverting to the latest revision is a no-op.
- `base_revision` on a write is optional: when given and not equal to the latest revision the write is refused with `E_REVISION_CONFLICT` (retryable: read the text again), so two writers do not overwrite each other without knowing. Without it the write wins and becomes the next revision.
- A text keeps at most 500 revisions. A write that makes the 501st drops the oldest one; revision numbers keep growing, so a number is never reused.
- A text in the trash or in a project in the trash cannot be written, as the item rules of SDD-005 section 5.12 say; it can be read.
- Reading a revision that does not exist is `E_NOT_FOUND`.

`Diff(a, b)` compares two bodies at paragraph level (paragraphs are separated by one or more blank lines): the answer is a list of hunks, each `equal`, `removed` or `added`, with the paragraph. It is the longest common subsequence of the paragraphs. A pair of bodies whose paragraph counts multiplied exceed four million (about 2,000 paragraphs each) is answered as one `removed` hunk and one `added` hunk with `coarse` true.

## 4. The script

The structure of a script is read from the Markdown; nothing but the body is stored.

- A heading of level 1 to 3 (`#`, `##`, `###`) starts a **scene**; its text is the title of the scene. Text before the first heading is the scene with no title.
- A paragraph that starts with `VO:` or `NARRATION:` (any case) is **narration**; one that starts with `ON SCREEN:`, `ON-SCREEN:` or `TEXT:` is **on-screen text**. The marker is not part of the text. A paragraph runs until a blank line, a heading or another marker. Any other paragraph is a **note** (a direction for the editor: it is not said and not shown).
- A script with no narration marker at all is read in **plain mode**: every paragraph that is not a heading is narration, so a person who writes the words of a video as prose gets an estimate without learning a marker.

`EstimateDuration(script, wordsPerMinute)` is the number of words of the narration divided by the words per minute, per scene and in total, in milliseconds, rounded to the nearest. A word is a run of characters between white space with at least one letter or digit. On-screen text and notes take no time. The words per minute are 60 to 300, default 150 (`DefaultWordsPerMinute`); the answer also gives the words of the on-screen text, so a person sees how much has to be read on the screen. A script with no narration has 0 ms. Hook, sections and call to action (RF-352) are what the scenes are named; the structure is the person's and the tool does not impose a template.

## 5. The caption and the table of limits

A `caption` is one text with **variants per network**. In the body, a heading of level 2 whose text is the identifier of a network (`## youtube`, `## instagram`, `## facebook`, `## x`, `## tiktok`, any case) starts the variant of that network, and runs to the next such heading. Text before the first one is the **common** variant (no network). A heading that is not a network identifier is plain text of the variant it is in. A caption therefore stays a plain Markdown file that a person can read and write by hand, and the revisions cover the variants as one.

### 5.1 The table

The limits are **data**, one table in `core/writing/limits.go`, each row with the date it was checked and the page it was read from, so that a change of a network is a change of one row (epic 17, risks). The connectors (epics 18, 19, 22) validate against the same table. A figure the network does not publish is `0` (no limit) and the row says so.

| Network | Length | Hashtags | Mentions | Links | Forbidden | Checked | Source |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `youtube` (the description of a video) | 5,000 bytes of UTF-8 | 60 | no limit published | clickable | `<` and `>` | 2026-11-19 | `developers.google.com/youtube/v3/docs/videos` (`snippet.description`); `support.google.com/youtube/answer/6390658` (hashtags) |
| `instagram` | 2,200 characters (code points) | 30 | 20 | not clickable | none | 2026-11-19 | `developers.facebook.com/documentation/instagram-platform/instagram-graph-api/reference/ig-user/media` (`caption`: "Maximum 2200 characters, 30 hashtags, and 20 @ tags") |
| `facebook` (the message of a Page post) | 63,206 characters (code points) | no limit published | no limit published | clickable | none | 2026-11-19 | `developers.facebook.com/docs/graph-api/reference/page/feed/` documents no limit for `message`; 63,206 is the long-published figure of the product, kept until Meta publishes one |
| `x` | 280 weighted characters | no limit published | no limit published | clickable, each link weighs 23 | none | 2026-11-19 | `docs.x.com/fundamentals/counting-characters` (text is counted after Unicode NFC; emoji, CJK and other non-Latin characters weigh 2, a URL 23) |
| `tiktok` (the title of a direct post) | 2,200 UTF-16 code units | no limit published | no limit published | not clickable | none | 2026-11-19 | `developers.tiktok.com/doc/content-posting-api-reference-direct-post` ("The maximum length is 2200 in UTF-16 runes") |

The unit of the length is part of the row (`bytes`, `characters`, `utf16_units`, `weighted`), so the same text has a different count for each network and the report says which.

### 5.2 Counting and validation

`ValidateCaption(text, network)` reads the variant text (trimmed of white space at both ends) and returns the **counts** (length in the unit of the network, hashtags, mentions, links) and the **problems**, each with a `code`, the `limit`, the `actual` value, the `unit` and a sentence that names both:

| Code | When |
| --- | --- |
| `empty` | the variant has no visible character |
| `too_long` | the length is over the limit of the network |
| `too_many_hashtags` | there are more hashtags than the network allows (when it publishes a limit) |
| `too_many_mentions` | there are more mentions than the network allows (when it publishes a limit) |
| `forbidden_character` | the text holds a character the network refuses (`limit` 0; the sentence names it) |
| `unknown_network` | the network is not in the table |

The report of a variant also carries `name`, the name of the network in the table, so that a surface draws it without a table of its own. A **hashtag** is `#` at the start of the text or after a character that is not a letter, a digit, `_` or `&`, followed by letters, digits and `_` with at least one letter. A **mention** is `@` in the same position followed by 1 to 30 letters, digits, `.` and `_`. A **link** is `http://` or `https://` followed by non-space characters. A link that is clickable adds nothing to the problems; for a network where it is not, the report carries the count so a person can see that the link will be dead text (`links` and `links_clickable`). The counting never changes the text.

`CaptionValidate` reads either a text and a network, or a stored caption (`project_id`, `item_id`) and optionally one network: for a stored caption it validates every variant of the body against its network (the common variant against none: it is reported with the counts and no problem) and the answer lists the variants in the order of the body.

## 6. Drafting with a model

`TextDraft` asks the provider layer for a draft of a text from the brief of the project and an instruction, and stores the answer as a **new revision** marked `generated`; it never overwrites and never changes an earlier revision.

- It is a job of the kind `text.draft` (SDD-000 section 6), registered as an approvable kind like `text.generate` (SDD-015 section 5, SDD-016 section 6). The arguments are `item_id`, `connection_id`, `model`, `instruction` (required, at most 4,000 characters), `network` (for a caption: draft the variant of one network), `max_tokens` and `temperature`. The job has the estimate, the summary a person reads, the ceilings, the approval (or the rule the user wrote), the ledger and the real cost of `text.generate`: it is `text.generate` with a prompt that the core built and a place to put the answer. No outbound call is made before the approval.
- The prompt is built by `core/writing` (`DraftPrompt`): the kind and its format (the markers of section 4 for a script, the variants of section 5 for a caption, with the limits of the network), the brief, the current body of the text and the instruction. The brief and the current body are **delimited as material** inside the prompt (`<material name="brief">`, `<material name="current">`, with every `<` of the text escaped so that it cannot close its own block; at most 8,000 characters of the brief and 20,000 of the current text go in), and the system text says that what is between the tags is information and never an order. The instruction is the task: it is the last part of the prompt, as written by the person or the agent that asked. The brand kit and the memory of the library join the prompt when a later epic wires them; this epic sends the brief only.
- When the job succeeds, in the same transaction that records the cost, the answer is written as the next revision of the item with `source` `generated`, the provider, the model, the real cost and the job id. If the text was changed by someone else between the start and the end of the job, the draft is still a new revision (the other one stays the revision before it): nothing is lost, and the person chooses. If the item was trashed or its project was trashed meanwhile, the job fails with `E_INVALID_STATE` after the cost is recorded, and the text of the answer is kept in the job result.
- A generated answer is limited to what the item may hold (256 KiB, valid UTF-8, no control characters but the line break and the tab); an answer that is not is cut at the last paragraph that fits and the revision says nothing else: the provider layer already caps the tokens.
- The result of the job is the `text.generate` result with `item_id` and `revision`. A job belongs to a timeline (SDD-003 section 1.7): `TextDraft` gives the job the first active timeline of the project, so the caller of a project with several is not asked which; a `job_start` of kind `text.draft` takes `timeline_id` as any job does.
- For a caption with a `network`, the answer is the text of that variant only and `core/writing` puts it into the body (`SetVariant`): the variant of the network is replaced, or added at the end, and the other variants are kept. The base is the body at the moment the job ends, so a variant written meanwhile for another network is not lost.

The prompt library (epic 15) may be used for the instruction: `PromptRender` produces a text a person or an agent passes as the instruction. The service does not read the library itself.

## 7. Service operations and scopes

All in the domain `writing` (SDD-000 section 1.1).

| Operation | Does | Scope |
| --- | --- | --- |
| `TextList` | the texts of a project, or of every project the caller may see, with kind, words and revision, newest first; filters `text_kind`, `state` | read |
| `TextGet` | one text with its body (untrusted), or one of its revisions | read |
| `TextWrite` | makes a text (`title`, `text_kind`, no `item_id`) or saves a new revision of one (`item_id`, `text`, optional `base_revision`, `title`) | edit |
| `TextRevisions` | the revisions of a text, newest first, with actor, source and cost, and optionally the diff between two (`from`, `to`) | read |
| `TextRevert` | a new revision with the body of an older one | edit |
| `TextDraft` | the job of section 6 | edit (the job waits for approval) |
| `CaptionValidate` | the counts and the problems of section 5.2 | read |
| `ScriptEstimate` | the estimate of section 4 for a stored script or for a text | read |

A key limited to one project reaches the texts of that project only, named by the project or by any of its timelines (SDD-003 section 1.7 rule 4); a timeline of another project, that project and an id of nothing get the same refusal. In network mode a user sees the texts of their own projects (`TextList` without a project lists those only).

## 8. Surfaces

| Operation | MCP | CLI | HTTP |
| --- | --- | --- | --- |
| `TextList` | `project_item` (`action` `text`, without `item_id`) | `text:list` | `GET /texts` |
| `TextGet` | `project_item` (`action` `text`, with `item_id`) | `text:show` | `GET /projects/{id}/texts/{item}` |
| `TextWrite` | `project_item` (`action` `text`, with `text` or `text_kind`) | `text:write` | `POST /projects/{id}/texts`, `PUT /projects/{id}/texts/{item}` |
| `TextRevisions` | `project_item` (`action` `revisions`) | `text:revisions` | `GET /projects/{id}/texts/{item}/revisions` |
| `TextRevert` | `project_item` (`action` `revert`) | `text:revert` | `POST /projects/{id}/texts/{item}/revert` |
| `TextDraft` | `job_start` (`kind` `text.draft`) | `text:draft` | `POST /projects/{id}/texts/{item}/draft` |
| `CaptionValidate` | `project_item` (`action` `check`, with `network`) | `caption:validate` | `POST /captions/validate` |
| `ScriptEstimate` | `project_item` (`action` `check`, with `wpm`) | `script:estimate` | `POST /scripts/estimate` |

No new MCP tool: the catalogue stays at 20 and its budget (SDD-003 section 4.8, 22,389 of 22,400 bytes after this epic) is paid by short names (`wpm`, `until`; `to` is refused by the outbound check of SDD-003 section 9) and a shorter description of `job_get`. In `project_item`, `action` `text` with no `item_id` lists the texts of the project, or with `text` makes one (`title`, `text_kind`); with an `item_id` it reads the text (`revision` for an older one), or with `text` saves a new revision (`revision` is the one the writer read, the `base_revision` of the Service); `revisions` compares `revision` with `until` when `revision` is given; `check` with `network` measures a caption, otherwise estimates a script at `wpm`, and with an `item_id` alone the kind of the text decides. The cross-project list of `TextList` has no MCP form: an agent lists the texts of one project. The draft is `job_start` with `kind` `text.draft` and `args` `item_id`, `connection_id`, `model`, `instruction`. The writing commands are listed by `stuarty writing` and by `command_help` with `domain` `writing`.

## 9. Errors and limits

`E_VALIDATION` (a text kind that does not exist, a title that fails the rule of an item title, a text that is not UTF-8, an instruction that is empty or too long, words per minute out of range, a network that is not in the table for `CaptionValidate` with a stored caption), `E_LIMIT_EXCEEDED` (256 KiB, 256 items per project), `E_NOT_FOUND` (project, item or revision), `E_INVALID_STATE` (the text or the project is in the trash; the item is not a text), `E_REVISION_CONFLICT` (`base_revision`), and the errors of the provider layer for a draft. No new code.

Limits: 256 KiB per text, 500 revisions per text, 4,000 characters of instruction, 50 texts in one `TextList` page (the page of the list operations of SDD-003 section 3.6).

## 10. Untrusted text

Every body, title and project name that leaves a writing operation toward an agent is marked `untrusted:"true"` in the result type, so MCP delimits it and carries `untrusted_notice` (SDD-003 section 3.10, SDD-007 section 10). The body of a **generated** revision is the text of a model: it is delimited the same way, never rendered as HTML (section 11), never used as an instruction by the product, and the summary that a person reads before approving a draft is made of text the product wrote and a cleaned excerpt of the instruction (`textSummary` of SDD-016 section 6). The Markdown is stored and returned as it is: nothing is "sanitized" by editing it, because a person's text is theirs; the defence is that no surface interprets it.

## 11. The page Write

The section **Write** of the navigation (SDD-012) lists the texts across the projects, filtered by project and kind, and opens one in the editor:

- a plain Markdown editor (a text area) with a preview; the preview is drawn from text nodes only (headings, paragraphs, lists, emphasis, code), never from HTML, and a link is shown as text. No rich-text library;
- save makes a revision and shows its number; a conflict (`E_REVISION_CONFLICT`) offers to reload;
- the list of revisions with actor, source (a generated one shows provider, model and cost), the text of one revision, the difference with the one before it (from `TextRevisions`), and a revert button;
- for a script: the estimate (from `ScriptEstimate`) with the words per minute the person sets; for a caption: the counters of each variant against its network (from `CaptionValidate`), each problem with its sentence. The counters are what the core returned; the UI computes nothing;
- the action **Draft with a model**: a connection, a model, an instruction and the estimate before the job starts; the job waits in the approval queue as any costly generation and, when it succeeds, the draft appears as a new revision.

Strings are in `en` and `pt-BR`. The page follows `docs/design-system.md` and the accessibility rules of SDD-012.

## 12. Tests

| Test | Holds |
| --- | --- |
| `core/writing` table tests | every limit of the table at its boundary (one below, at, one above) for each network; the hashtag, mention and link counts; the weighted count of X; the script structure and the estimate with several words per minute; the diff |
| `service` tests | the write, revert and conflict rules against a real store; a draft is a new revision with provider, model and cost and leaves earlier revisions intact; a draft that is refused by a ceiling writes nothing |
| `web/contract` | one row per operation of section 8 on the three surfaces, and the scenario `writing.json` |
| `web/e2e/specs/writing.spec.ts` | write a script, draft a caption, see the counters, revert |
| security | an agent cannot reach a text of another user or of another project of its key; a body is delimited; the preview does not render HTML |
