# SDD-020: Publishing: the connector frame and the YouTube connector

- Status: Draft (2026-11-19). Written by epic 18; the code follows in the same epic. Epics 19, 20, 22, 44 and 45 add their own parts below the connector part.
- Depends on: ADR-0032 (publishing waits for a person), ADR-0033 (the vault), ADR-0035 (the user's own developer apps), SDD-000 (names), SDD-003 (agent interfaces), SDD-007 (threat model), SDD-014 (connections), SDD-015 (approval and cost)
- Implements: PRD-004 RF-400, RF-401 (YouTube), NFR-063, NFR-064
- Owner area: `core/publish/`, `service/social/`, `service/social/youtube/`, `service/connections/oauth/`, the publishing operations in `service/`, the controllers, commands and the callback route in `web/app/`, the setup guide and the publish dialog of the Connections page

## Contents

1. Overview
2. What the official pages said (read on 2026-11-19)
3. The post and its validation (`core/publish`)
4. The state of a publication
5. The connector interface (`service/social`)
6. Signing in with the user's own app (`service/connections/oauth`)
7. The YouTube connector
8. The setup guide
9. Service operations and scopes
10. The job `publish.upload` and its approval
11. Errors
12. Files, tables and variables
13. Surfaces
14. Tests

## 1. Overview

A creator connects their YouTube channel once, with a developer project of their own, and a finished video goes from Stuarty to the channel only after a person approved it. The frame is built for every network that follows: one interface, one package per network, and nothing outside that package knows the network's API (NFR-063).

Four rules carry the design:

1. **The rules of a publication are pure and live in `core/publish`.** What a post is, what a network allows and the state of an upload are decided there. The connector only talks to the network.
2. **Every call goes through a connection.** A connector reaches the network only through `connections.Manager.With`, so it reaches the hosts of its own connection and nothing else (SDD-014 section 6), with a credential that is its own (T-57).
3. **A publication waits for a person.** `publish.upload` is a job kind that needs approval (SDD-015 section 5). Before the approval no request leaves the machine; no operation, surface or rule of an agent skips it (T-48, T-59).
4. **What the network answers is untrusted text.** A title, a description, an error message or a channel name that comes from the network, or from an agent, is delimited and cleaned before a surface shows it (SDD-007 section 8, T-58).

The Instagram and Facebook connectors (epic 19), the calendar and the scheduler (epic 20), X and TikTok (epic 22), Reddit (epic 44) and metrics (epic 45) are not in this document yet.

## 2. What the official pages said (read on 2026-11-19)

Read again before the code, as the research note of 2026-11-18 asked. The guide of section 8 shows this date and links the official pages, because Google changes these rules often (it did on 2026-11-02, 2026-06-01 and 2025-12-04).

| Fact | Value | Official page |
| --- | --- | --- |
| Upload endpoint, resumable | `POST https://www.googleapis.com/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status`; the session address is in the `Location` header of the `200` | `docs/videos/insert`, `guides/using_resumable_upload_protocol` |
| Send the file | `PUT` to the session address with the whole file, or the rest of it with `Content-Range`; `201 Created` with the `video` resource | the resumable protocol |
| Ask how much arrived | an empty `PUT` with `Content-Length: 0` and `Content-Range: bytes */<total>`; `308` with `Range: bytes=0-<last>` (no `Range`: nothing arrived) | the resumable protocol |
| Retry | no answer, or `500`, `502`, `503`, `504`: resume with a backoff. `404`: the session expired, start a new one from the start. Any other `4xx`: permanent | the resumable protocol |
| Maximum file | 256 GB; `video/*` or `application/octet-stream` | `docs/videos/insert` |
| Scope to upload | `https://www.googleapis.com/auth/youtube.upload` (also accepted: `youtube`, `youtube.force-ssl`, `youtubepartner`) | `docs/videos/insert` |
| Quota of an upload | 1 unit in its own bucket of 100 calls a day since 2026-06-01 (before 2025-12-04: about 1,600 units; between: about 100 units of the one bucket of 10,000) | `determine_quota_cost`, `revision_history` |
| Quota of the thumbnail | 50 units per `thumbnails.set` of the 10,000 a day; `videos.list` and `channels.list` 1 unit each; the day ends at midnight Pacific Time | `determine_quota_cost` |
| Unverified projects | since 2026-11-02 an upload from an unverified API project is no longer forced to private (it was, since 2020-07-28). Without a `privacyStatus` the video is public, so Stuarty always sends one | `revision_history`, `docs/videos/insert` |
| Title | at most 100 characters; no `<` or `>` | `docs/videos` |
| Description | at most 5,000 bytes; no `<` or `>` | `docs/videos` |
| Tags | at most 500 characters in all, commas counted; a tag with a space counts as if quoted (2 more) | `docs/videos` |
| Visibility | `private`, `unlisted` or `public` | `docs/videos` |
| Made for kids | `status.selfDeclaredMadeForKids`, a boolean the channel owner declares | `docs/videos` |
| Altered or synthetic content | `status.containsSyntheticMedia`, a boolean (since 2024-10-30) | `revision_history` |
| Thumbnail | `POST https://www.googleapis.com/upload/youtube/v3/thumbnails/set?videoId=<id>`; `image/jpeg` or `image/png`; at most 50 MB since 2026-09-14; for a Short, after processing (since 2026-11-03) | `docs/thumbnails/set`, `revision_history` |
| Upload state | `status.uploadStatus`: `uploaded`, `processed`, `failed`, `rejected`, `deleted`; `failureReason` (`codec`, `conversion`, `emptyFile`, `invalidFile`, `tooSmall`, `uploadAborted`) and `rejectionReason` (`claim`, `copyright`, `duplicate`, `inappropriate`, `legal`, `length`, `termsOfUse`, `trademark`, `uploaderAccountClosed`, `uploaderAccountSuspended`); `processingDetails.processingStatus`: `processing`, `succeeded`, `failed`, `terminated` | `docs/videos` |
| Who is the channel | `GET https://www.googleapis.com/youtube/v3/channels?part=id,snippet&mine=true`; `items[].id` and `items[].snippet.title` | `docs/channels/list` |
| Sign-in | `https://accounts.google.com/o/oauth2/v2/auth` (`response_type=code`, `access_type=offline`, `state`, `scope`, `redirect_uri`); token `POST https://oauth2.googleapis.com/token` (`grant_type` `authorization_code` or `refresh_token`); a `localhost` redirect is exempt from the https rule. Google limits the refresh tokens per client and user: older ones stop working | `guides/auth/server-side-web-apps` |

The pages did not say, in the parts read, that a refresh token of a consent screen in the publishing status "Testing" lasts 7 days; the guide says it as a thing to check, and the connection reports `needs_sign_in` when Google refuses the refresh token (section 7.6). The scope a call to `channels.list` and `videos.list` needs is not named on the pages read either; Stuarty asks for `youtube.readonly` besides `youtube.upload`, so that it can name the channel and read the state of the video, and the guide says so. **Decision of this SDD:** if Google answers a read with `403 insufficientPermissions`, the connection shows `needs_sign_in` with the hint to sign in again, and the upload itself, which needs only `youtube.upload`, is not blocked by a failed read of the state.

## 3. The post and its validation (`core/publish`)

```go
type Post struct {
	Output       File        // the exported video: Bytes and Format ("mp4"); the service fills them from the export job
	Thumbnail    *File       // the exported still, optional ("png" or "jpg")
	Title        string
	Description  string
	Tags         []string
	Visibility   Visibility  // "private" (the default), "unlisted" or "public"
	MadeForKids  *bool       // required: nil is a validation issue, the person must say
	AIDisclosure bool        // the video has realistic altered or synthetic content
}

type Limits struct {
	TitleMax, DescriptionMaxBytes, TagsMaxChars int
	FileMaxBytes, ThumbnailMaxBytes             int64
	Formats, ThumbnailFormats                   []string
	Visibilities                                []Visibility
	NeedsKidsDeclaration                        bool
	Forbidden                                   string // characters a title or description may not hold, "<>" for YouTube
	Source, Read                                string // the official page and the date it was read
}

func Validate(p Post, l Limits) []Issue   // every problem at once, each with a path, a stable code and a message
```

`Validate` is a function of its two arguments and opens nothing. An issue has a `Path` (`title`, `tags`, `output`...), a `Code` (`required`, `too_long`, `forbidden_character`, `unsupported_format`, `too_large`, `not_allowed`, `invalid`) and a `Message`. The title is required and trimmed; the visibility defaults to `private`; a control character, or an invisible or direction-changing one (`publish.Invisible`: the zero-width space, the byte order mark, the bidirectional controls, the tag characters; the zero-width joiner stays for emoji), is invalid in a title, a description and a tag, so that what a person approves is what is sent; the tags are counted as YouTube counts them. A post that fails is `E_VALIDATION` with the first issue's path and every issue in `details.issues`.

## 4. The state of a publication

`prepared`, `waiting_approval`, `uploading`, `processing`, `published`, `failed`. The moves allowed, and only these:

```text
prepared         -> waiting_approval
waiting_approval -> uploading | failed        (approved, or rejected, cancelled or expired)
uploading        -> processing | published | failed
processing       -> published | failed
```

`core/publish.Can(from, to)` says whether a move is allowed. The job is the record: `waiting_approval` is the job state of that name; `queued` and `running` are `uploading`; a `succeeded` job carries the state of the video (`processing` or `published`) in its result; `failed` and `cancelled` are `failed`. `prepared` is what `PublishValidate` answers for a post that is valid and not yet submitted.

## 5. The connector interface (`service/social`)

```go
type Connector interface {
	Describe() Description                                              // network, what it can publish, the limits with source and date, the hosts
	Validate(p publish.Post) []publish.Issue                            // core/publish.Validate with its own limits, plus rules only that network has
	Publish(ctx context.Context, c Call, u Upload) (Published, error)   // remote id, URL, state
	Status(ctx context.Context, c Call, remoteID string) (Remote, error)
	AuthConfig() AuthConfig                                             // the sign-in: URLs, scopes, extra parameters
	Account(ctx context.Context, c Call) (Account, error)               // who the credential is: the channel
	connections.Connector                                                // Test, for the Connections page
}
```

`Call` is the credential and the gated client of one connection (`connections.Credential`) plus a clock; `Upload` is the post, the paths of the files and a `Sessions` store that keeps the address of an upload session so that a restart resumes. A connector returns `*social.Error`, whose `Kind` is `quota`, `not_eligible`, `auth`, `invalid`, `rejected` or `unavailable`; the service turns it into `E_PUBLISH` (section 11).

`service/social.Registry` maps a network name to its connector. Adding a network is its package and one `Register` line in `web/app/owner` (acceptance criterion 5 of epic 18): the tests register a fake connector the same way. The registry registers the connector's kind with `connections.Manager`, so a connection of the network can be created, listed and tested like any other.

## 6. Signing in with the user's own app (`service/connections/oauth`)

A generic authorization-code flow with PKCE (S256), reused by epic 12 and epic 24. The package holds no network code of its own: it builds the authorization address, keeps the pending flows, and exchanges a code or a refresh token through the gated client it is given.

- `Begin` makes a random `state` (32 bytes) and a PKCE verifier, keeps them in memory for 10 minutes with the connection id, the actor and whether it was a browser, and returns the authorization address. At most 20 flows are pending; the oldest expire first.
- `Complete(state, code)` takes the pending flow once: a state that is unknown, used or expired is `E_VALIDATION` on the path `state`. A flow begun by a browser session is completed only by the same actor; one begun by the CLI is completed by a browser session, which is the person who saw the consent screen.
- The code is exchanged at the token address of the connector with the client id, the client secret and the verifier, **through the gated client of the connection**, so the token host must be one of the hosts of the connection. The tokens never leave the closure of `Manager.With`; what is stored in the vault is the secret of the connection (section 7.1).
- The redirect is the local server: `<origin of the app>/social/callback`. The person pastes that address in their Google project; the guide shows it. `Begin` accepts only an `https` address, or an `http` address of a loopback host, whose path is `/social/callback`.

## 7. The YouTube connector

### 7.1 The connection and its secret

A connection of the kind `youtube` has the endpoint `https://www.googleapis.com`, the hosts `www.googleapis.com` and `oauth2.googleapis.com`, and accepts `video`, `image` and `metadata`. Its secret, in the vault, is one JSON document that no surface returns:

```json
{ "client_id": "...", "client_secret": "...", "refresh_token": "...", "account_id": "UC...", "account_title": "..." }
```

`refresh_token`, `account_id` and `account_title` are empty until the sign-in is complete. The accounts list shows `account_id` and `account_title` (the title is text from the network and is delimited); it never shows the other members. An access token is asked for at each operation from the refresh token and is never stored.

### 7.2 The upload

1. Ask for an access token (`grant_type=refresh_token`).
2. Look for a session of this upload (section 7.4); with none, start one: the `POST` of section 2 with the JSON of `snippet` (`title`, `description`, `tags`, `categoryId` 22) and `status` (`privacyStatus`, `selfDeclaredMadeForKids`, `containsSyntheticMedia`), and `X-Upload-Content-Length` and `X-Upload-Content-Type`. The visibility defaults to `private`.
3. `PUT` the file in one request from the open file. After an error or a restart, ask how much arrived (the empty `PUT`) and send the rest from that byte.
4. On `201`, read the video id; set the thumbnail when there is one (a failure of the thumbnail is recorded in the result and does not fail the upload, since the video exists); read the state with `videos.list?part=status,processingDetails` up to 5 times, 2 seconds apart; the result is `published` when `uploadStatus` is `processed` and `processing` otherwise.
5. The URL is `https://www.youtube.com/watch?v=<id>`. It is built by the connector from the id; the network is not asked for it.

### 7.3 Errors

| The network says | `E_PUBLISH` `details.kind` | The message tells the person |
| --- | --- | --- |
| `403 quotaExceeded`, `rateLimitExceeded`, `dailyLimitExceeded`, `429 uploadRateLimitExceeded`, `400 uploadLimitExceeded` | `quota` | the quota or the upload limit of the day was used, that it comes back at midnight Pacific Time, and that publishing again resumes |
| `401`, `invalid_grant` on a refresh, `403 insufficientPermissions` | `auth` | to sign in to the channel again (the connection shows `needs_sign_in`) |
| `403 youtubeSignupRequired`, `forbidden`, `channelClosed`, `channelSuspended`, `forbiddenPrivacySetting` | `not_eligible` | the channel cannot upload, or cannot upload with that setting, and what to check in YouTube Studio |
| other `400` (`invalidTitle`, `invalidDescription`, `invalidTags`, `invalidFilename`, `mediaBodyRequired`...) | `invalid` | which field the network refused |
| `uploadStatus` `rejected` or `failed` | `rejected` | the reason as the network gave it (`duplicate`, `copyright`, `codec`...) |
| `5xx`, no answer, a session that kept failing after 5 resumes | `unavailable` | to try again later |

The message is written by Stuarty from the kind and the reason code; the text of the network's `message` is never copied into it (T-58). `quota`, `unavailable` and `auth` set `details.retryable` true. A job that failed this way is not lost: publishing the same post again resumes the same upload session (section 7.4).

### 7.4 Resuming

The address of the upload session is kept in `<workspace>/publish/uploads/<key>.json` (mode 0600), with the size of the file, where `key` is the SHA-256 of the connection id, the path, the size and the digest of the file (the one its export recorded), and every member of the post the network receives when the session starts (title, description, tags, visibility, `made_for_kids`, `ai_disclosure`; not the thumbnail, which is sent after the upload). It holds no token: the session address is useless without the access token. The file is written when the session is created, removed when the upload ends (`201`) and removed when the network answers `404` (the session expired). A job that runs again after a restart, or a new `PublishNow` of the same post after a failure, finds it and resumes from the byte the network reports; it does not start from zero (acceptance criterion 3). A file that changed has another key.

### 7.5 What the connector never does

It does not call a host that is not in its connection; it does not follow a redirect (the gate returns it); it does not write a credential in a log or an error; it does not publish a post that was not validated; it does not set `privacyStatus` public unless the post says so.

### 7.6 Account state

`SocialAccountList` gives each connection of a social kind with `state`: `ready` (a refresh token is in the vault), `needs_sign_in` (no refresh token yet, or the network refused it at the last use; the last refusal is kept in `last_error` as a stable kind) and the account title. The Connections page shows the button to sign in again.

## 8. The setup guide

A component of the Connections page, in English and Portuguese, that walks through, with the exact values to paste: create a project in the Google Cloud console; enable "YouTube Data API v3"; configure the consent screen (external, add the user's own address as a test user); create an OAuth client of the type "Web application" and paste `<origin>/social/callback` as the authorised redirect address; copy the client id and secret into Stuarty; sign in. It states what section 2 found about unverified projects and the "Testing" status, and shows the date read with the link to the official page. It computes nothing: the redirect address is the origin of the page plus the path the service gave.

## 9. Service operations and scopes

Added to SDD-000 section 1 and to the scope matrix of SDD-007:

| Operation | Scope | Result |
| --- | --- | --- |
| `SocialNetworkList` | read | the networks of the build: name, what each can publish, limits with source and date, the redirect path, the setup steps |
| `SocialConnectBegin` | admin | creates the connection (or takes an existing one by `connection_id`) with the client id and secret in the vault and answers the authorization address and the state; no secret in the result |
| `SocialConnectComplete` | admin, and a browser session | the sign-in redirect (the page `/social/callback` posts to it): exchanges the code, stores the refresh token, reads the channel; answers the account |
| `SocialAccountList` | read | the connections of the social kinds with their account title and state; never a token |
| `PublishValidate` | read | the issues of a post against the limits of its network, and `prepared` when none |
| `PublishNow` | render | starts the job `publish.upload` of one post; answers the job in `waiting_approval` (or `queued` when a rule the person wrote covers it) |
| `PublishStatus` | read | the publication of a job: its state, the remote id and URL; for a video still `processing` it reads the network once |

A project-limited key may use the operations that name its project and is refused the others. `JobStart` with the kind `publish.upload` is the same code as `PublishNow`.

## 10. The job `publish.upload` and its approval

`publish.upload` is registered with `ApprovableKind` (SDD-015 section 5) by the service when a registry of connectors is given. Its arguments: `connection_id`, `export` (the id of a succeeded export job of the project, the video), `thumbnail` (the id of a succeeded still export, optional), `title`, `description`, `tags`, `visibility`, `made_for_kids`, `ai_disclosure`.

`Prepare` resolves the files (the export must belong to the project and be `succeeded`, inside the workspace), validates the post with the connector (`E_VALIDATION`), checks that the connection exists and is of a social kind, and describes the action: the **summary** (one line: network, channel, title, visibility, file name and size; the title is the person's text, so control and invisible characters are refused as SDD-015 section 3 says), the connection, the estimate (no cost). It makes no request.

`Run` runs after the approval, through `Manager.With`: it resolves the files again and refuses with `E_INVALID_STATE` a file whose size or digest differs from what the person was shown, calls `Publish` and returns the remote id, the URL and the state as the result of the job. The cost is none; the quota of the network is not money and is not in the ledger.

## 11. Errors

Added to SDD-000 section 8: `E_PUBLISH` (a network refused or failed a publication; `details.kind` is `quota`, `not_eligible`, `auth`, `invalid`, `rejected` or `unavailable`, `details.network`, `details.retryable` and, for a quota, `details.retry_after` as a time; the hint says what to do). `E_VALIDATION` covers the post (`details.issues`). `E_CONNECTION_NOT_FOUND` is a connection that does not exist. `E_VAULT_LOCKED` is as in SDD-014.

## 12. Files, tables and variables

- No new table: the job, the approval request and the audit rows are the records. A migration is added only if a later epic needs a table (the calendar of epic 20 does).
- The folder `publish/uploads/` of the workspace (section 7.4), added to SDD-000 section 9. The sweeper of SDD-005 section 13.4 removes upload files older than 7 days.
- One variable, for tests only: `STUARTY_TEST_SOCIAL_URL`, the address of a server on loopback that plays a network. The build then registers the connector `fake` (`service/social/fake`) next to YouTube, and the end-to-end spec `youtube-connect.spec.ts` uses it, because a real Google account and a real consent screen are never used by a test. It is refused in network mode and for any address that is not `http` on loopback (`web/app/owner/social.go`).

## 13. Surfaces

- HTTP: `GET /social/networks`, `POST /social/connect`, `POST /social/complete`, `GET /social/accounts`, `POST /publish/validate`, `POST /publish`, `GET /publish/{job_id}`. The redirect address of the sign-in, `<origin>/social/callback`, is a page of the app, not an API route: it reads `state` and `code` from its address and posts them to `POST /social/complete` with the session of the browser, so the callback has the same authentication, origin and request checks as every other call. The page itself is drawn without a session, since the cross-site navigation from the consent screen carries no `SameSite=Strict` cookie (SDD-007 section 3.7 item 2); it holds no data.
- CLI: `social:network:list`, `social:connect`, `social:account:list`, `publish:validate`, `publish:now`, `publish:status`. `social:connect` reads the client secret from standard input or `--secret-file`, never from an argument, and prints the address the person opens in the browser. The callback has no command: it is the browser's. The six commands are left out of the editor skill of `agent:docs`, as the library commands are, because that skill is at its size cap; the skills of publishing (epics 13 and 16) carry them.
- MCP: **no tool runs them in this epic.** The seven operations are in the domain `publish` (SDD-000 §1.1, `service.OperationDomains`), so `command_help` with `domain` `publish` and the agent guide of the domain name them, but no tool routes to them: epic 13 fixed the catalogue at the 20 tools of SDD-003 §4.1 with no new tool (§4.8), and the `kind` of `job_start` is a closed list that does not name `publish.upload`. An agent prepares a publication through the CLI (`publish:now`) or HTTP (`POST /publish`), reads it with `publish:status` and cannot approve it. What is left, for the epic that routes the first `publish` operation through MCP: `publish.upload` in the kind list of `job_start`, as epic 09 did for `text.generate` (SDD-016), since `JobStart` with that kind is already the code of `PublishNow`; the row of `PublishNow` in the parity table with the tool `job_start`; and the bytes it costs paid for inside the budget of NFR-020.
- The UI: the setup guide, the account list and the "publish now" dialog in the Connections page. The dialog sends the post and shows what the service answers (the issues, then the job waiting for approval); it validates nothing itself. It lists the succeeded exports of the project with `GET /projects/{id}/jobs`, so a project with several timelines is asked for its timeline first (the dialog does not yet); the export job of any timeline of the project is accepted by the service, since a job of a timeline belongs to its project.

## 14. Tests

| What | Test |
| --- | --- |
| Validation against the limits, table tests; the state machine | `go test ./core/publish/...` |
| The registry and the fake connector | `go test ./service/social/...` |
| The sign-in against a fake authorization server: happy path, replayed state, expired state, wrong actor | `go test ./service/social/youtube/... -run TestAuth`, `./service/connections/oauth/...` |
| The upload, the thumbnail and the status from recorded request and response pairs, with the quota and eligibility errors, and the resume after an interrupted upload | `go test ./service/social/youtube/...` (fixtures in `testdata/`) |
| No request before approval, then `published`; a rejection sends nothing | `TestPublishNow_*` in `web/app/servicetest` |
| The surfaces agree | `web/contract` parity |
| A post to the wrong account, a token used by another connector, untrusted text, a hijacked callback | `TestSecurity_T56_*` to `TestSecurity_T59_*` |
| The guide and the dialog in both languages | `web/e2e/specs/youtube-connect.spec.ts` |
