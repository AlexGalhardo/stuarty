# SDD-020: Publishing: the connector frame, YouTube, Instagram and Facebook

- Status: Draft (2026-11-19). Written by epic 18 (sections 1 to 14: the connector frame and YouTube); the code follows in the same epic. Part 2 (sections 15 to 22, Instagram and Facebook) was written by epic 19 on 2026-11-19. Part 3 (sections 23 to 31: the publication, the calendar, the approval of a scheduled post and the scheduler) was written by epic 20. Epics 22, 44 and 45 add their own parts below.
- Depends on: ADR-0032 (publishing waits for a person), ADR-0033 (the vault), ADR-0035 (the user's own developer apps), ADR-0039 (a publication belongs to a project), ADR-0046 (the publishing scheduler), SDD-000 (names), SDD-003 (agent interfaces), SDD-005 (tables), SDD-007 (threat model), SDD-014 (connections), SDD-015 (approval and cost)
- Implements: PRD-004 RF-400, RF-401 (YouTube, Instagram, Facebook), RF-402 to RF-406, RF-407 (AI disclosure), RF-408, NFR-063, NFR-064
- Owner area: `core/publish/`, `service/publish/`, `service/social/`, `service/social/youtube/`, `service/social/meta/`, `service/social/instagram/`, `service/social/facebook/`, `service/connections/oauth/`, the publishing operations in `service/`, the controllers, commands and the callback route in `web/app/`, the setup guide and the publish dialog of the Connections page, the Publish pages in `web/resources/js/pages/Publish/`

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

Part 2, Instagram and Facebook (epic 19):

15. What the official pages said (read on 2026-11-19)
16. The cases offered and the cases not offered
17. Signing in with a Meta app and choosing the account
18. The limits, as data, and the media of the post
19. Publishing: the steps at Meta and the errors
20. AI disclosure
21. The setup guide and the picker
22. Operations, variables and tests of part 2

Part 3, the calendar and the scheduler (epic 20):

23. The publication and its targets
24. The state of a target
25. Time: the zone, daylight saving and a missed time
26. Approval and cost of a scheduled post
27. The scheduler
28. Service operations of the publication
29. Tables, events and surfaces of the publication
30. The calendar and the editor
31. Tests of the publication

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
- One variable, for tests only: `STUARTY_TEST_SOCIAL_URL`, the address of a server on loopback that plays a network. The build then registers the connector `fake` (`service/social/fake`) next to YouTube, and the end-to-end spec `youtube-connect.spec.ts` uses it, because a real Google account and a real consent screen are never used by a test. It is honoured only with `APP_ENV=test` (SDD-007 T-67), and refused in network mode and for any address that is not `http` on loopback (`web/app/owner/social.go`).

## 13. Surfaces

- HTTP: `GET /social/networks`, `POST /social/connect`, `POST /social/complete`, `GET /social/accounts`, `POST /publish/validate`, `POST /publish`, `GET /publish/{job_id}`. The redirect address of the sign-in, `<origin>/social/callback`, is a page of the app, not an API route: it reads `state` and `code` from its address, removes them from the address and from the history entry before the exchange starts (VV-274, as `/auth/callback` does for T-76), and posts them to `POST /social/complete` with the session of the browser, so the callback has the same authentication, origin and request checks as every other call. The page itself is drawn without a session, since the cross-site navigation from the consent screen carries no `SameSite=Strict` cookie (SDD-007 section 3.7 item 2); it holds no data.
- CLI: `social:network:list`, `social:connect`, `social:account:list`, `publish:validate`, `publish:now`, `publish:status`. `social:connect` reads the client secret from standard input or `--secret-file`, never from an argument, and prints the address the person opens in the browser. The callback has no command: it is the browser's. The six commands are left out of the editor skill of `agent:docs`, as the library commands are, because that skill is at its size cap; the skills of publishing (epics 13 and 16) carry them.
- MCP (author decision D-093, which brought it back inside the budget of NFR-020 and the cap of 20 tools): `PublishNow` is `job_start` with `kind` `publish.upload` and the post as `args` (`connection_id`, `export` the id of a succeeded export job, `title`, `made_for_kids`, and optionally `thumbnail`, `description`, `tags`, `visibility`, `ai_disclosure`); it answers a job in `waiting_approval` with its `approval_id`, and nothing leaves the machine before a person approves, as on HTTP and in the CLI. `PublishStatus` is `job_get` with the `job_id`: the `state` of the job and, once it succeeded, `result.publish` (network, connection, `url`, `state` `processing` or `published`); the job error carries the failure. The request that waits for a person is read with `job_get` and `approval_id` (SDD-003 §4.1, E-6). `PublishValidate` and the other operations of the domain have no tool, and no tool approves, rejects or sets a rule. `job_get` shows the job's own state, so the `uploading` and `processing` refinement and the one status question to the network of `PublishStatus` are for the CLI and HTTP.
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

## Part 2: Instagram and Facebook (epic 19)

A creator publishes the same vertical video as a Reel on an Instagram professional account and on a Facebook Page, through their own Meta developer app, after approving each post. The frame of sections 5 to 13 is unchanged: three packages (`service/social/meta` for what both share, `service/social/instagram`, `service/social/facebook`) and two registration lines in `web/app/owner/social.go`. The one addition to the frame is the optional interface `social.Chooser` of section 17.

## 15. What the official pages said (read on 2026-11-19)

Read again before the code, as the research note of 2026-11-18 asked. Meta moves its pages and its API version often; the guide of section 21 shows this date and links the pages. The pages are rendered by script and were read through a text extraction: a value below that the extraction did not state is marked "not stated" and is never guessed.

| Fact | Value | Official page |
| --- | --- | --- |
| Who needs App Review | "My app is only for a business I own or manage", with Facebook Login, at Standard Access: App Review "Not required". The page says nothing of app roles, of development against live mode or of Business Verification; Business Verification is "required for all apps making requests for Advanced Access" (the permissions page), which Stuarty never asks for | `instagram-platform/app-review`, `permissions` |
| Instagram account | a professional account (Business or Creator), linked to a Facebook Page. A personal account cannot be published to | `instagram-platform/content-publishing` |
| Login path used | Instagram API with Facebook Login: host `graph.facebook.com`, a Facebook Page access token, permissions `instagram_basic`, `instagram_content_publish`, `pages_read_engagement` (and `pages_show_list`, which they depend on) | `instagram-platform/content-publishing`, `permissions` |
| Login path not used | Instagram API with Instagram Login (`graph.instagram.com`, `instagram_business_*`): it has no Facebook Page, but the page does not offer the resumable upload for it, and a public address is never invented (section 16) | `instagram-platform/content-publishing` |
| Instagram, create the container | `POST /<IG_ID>/media` with `media_type=REELS`, `upload_type=resumable`, `caption`, `share_to_feed`, `is_ai_generated`; the answer has the container `id` and the upload `uri`. `upload_type=resumable` is for "Facebook Login for Business apps only" | `ig-user/media`, `content-publishing` |
| Instagram, send the file | `POST https://rupload.facebook.com/ig-api-upload/<version>/<container id>` with `Authorization: OAuth <token>`, `offset: 0`, `file_size: <bytes>` and the file as the body; `{"success": true}`. A `file_url` header takes a hosted file instead | `ig-user/media` |
| Instagram, processing | `GET /<container id>?fields=status_code`: `IN_PROGRESS`, `FINISHED`, `ERROR`, `EXPIRED`, `PUBLISHED`. "Query a container's status once per minute, for no more than 5 minutes." A failed upload is a `ProcessingFailedError` with `retriable: false` | `content-publishing` |
| Instagram, publish | `POST /<IG_ID>/media_publish` with `creation_id`; the answer is `{"id": "<media id>"}`; `config_issue` (`CAPTION_NOT_ATTACHED`, `USER_TAGGING_FAILURE`) means published with a part dropped. Publish only at `FINISHED` | `ig-user/media_publish` |
| Instagram, the limit of a day | 100 API-published posts in a moving 24 hours; `GET /<IG_ID>/content_publishing_limit` reads it. The carousel section of the same page still says 50 | `content-publishing` |
| Instagram Reel | MOV or MP4, no edit lists, moov first; HEVC or H.264, progressive, closed GOP, 4:2:0; AAC up to 48 kHz, mono or stereo; 23 to 60 fps; width up to 1,920; aspect ratio 0.01:1 to 10:1 (9:16 recommended); video at most 25 Mbit/s VBR; 3 seconds to 15 minutes; at most 300 MB. Cover: JPEG, 8 MB, but `cover_url` is a public address | `ig-user/media` |
| Instagram caption | 2,200 characters, 30 hashtags, 20 @ mentions | `ig-user/media` |
| Instagram, AI | `is_ai_generated`, "optional self-disclosure of AI usage" | `ig-user/media`, `content-publishing` |
| Facebook Reel, steps | `POST /<PAGE_ID>/video_reels` with `upload_phase=start` answers `video_id` and `upload_url`; `POST` to the upload address on `rupload.facebook.com` with `Authorization: OAuth <Page token>`, `offset`, `file_size`; `POST /<PAGE_ID>/video_reels` with `video_id`, `upload_phase=finish`, `video_state=PUBLISHED`, `description`, `title`, `is_ai_generated` | `video-api/guides/reels-publishing`, `page/video_reels` |
| Facebook Reel, permissions | a Page access token of a person who can perform `CREATE_CONTENT` on the Page; `pages_show_list`, `pages_read_engagement`, `pages_manage_posts` | `video-api/guides/reels-publishing`, `permissions` |
| Facebook Reel, limits | 3 to 90 seconds; aspect ratio 9:16; at least 540 x 960 (1,080 x 1,920 recommended); 24 to 60 fps; `.mp4`, H.264 or H.265; 30 API-published posts in a moving 24 hours. **A maximum file size is not stated.** The description's maximum is not stated either | `video-api/guides/reels-publishing` |
| Facebook Reel, status | `GET /<video id>?fields=status`: `uploading_phase`, `processing_phase`, `publishing_phase`, `video_status` (`processing`, `ready`, `upload_failed`, `upload_complete`, `expired`, `error`); an interrupted upload resumes with `offset` set to `bytes_transfered` of the status | `video-api/guides/reels-publishing` |
| Facebook Reel, errors | `1363040` aspect ratio, `1363127` resolution, `1363128` duration, `1363129` frame rate, `100` missing parameter, `190` token, `200` permission, `368` blocked, `613` rate limit, `6000` the file | `video-api/guides/reels-publishing`, `page/video_reels` |
| Long-lived tokens | `GET /oauth/access_token?grant_type=fb_exchange_token&client_id&client_secret&fb_exchange_token` trades a short-lived user token for one of about 60 days; `GET /<user id>/accounts` with it lists the Pages, each with its own `access_token`, `name`, `id`; a Page token got from a long-lived user token "has no expiration date" and is invalidated only by the conditions of the error page | `facebook-login/guides/access-tokens/get-long-lived` |
| Graph errors | `{"error": {"message", "type", "code", "error_subcode", "error_user_msg", "fbtrace_id"}}`. Codes `1`, `2` transient; `4`, `17`, `341` rate limits; `10` and `200` to `299` permission; `190` token (subcodes `458` app not installed, `460` password changed, `463` expired, `467` invalid); `368` blocked | `graph-api/guides/error-handling` |
| Instagram publishing errors | code / subcode: `-1/2207001` server; `-2/2207003` media download timeout; `-2/2207020` media expired; `-1/2207032` creation failed; `-1/2207053` unknown upload error; `4/2207051` flagged as spam; `9/2207042` daily publishing limit; `24/2207006` media not found; `24/2207008` container missing or expired; `25/2207050` account restricted or inactive; `100/2207023` unknown media type; `352/2207026` unsupported video format; `9004/2207052` media address not fetchable; `9007/2207027` media not ready; `36004/2207010` caption too long | `instagram-graph-api/reference/error-codes` |

Not stated on any page read, and so recorded as such: whether a Meta app in development mode lets its own admins publish without App Review for these permissions (the pages describe Standard Access and "own business" only); whether `rupload.facebook.com` takes a file larger than the 300 MB the Reel limit allows in one request; the maximum size of a Facebook Reel; whether the Facebook token endpoint ignores a PKCE `code_verifier` and answers a `POST`. Section 22 lists them as what only the author's own app can verify.

**STOP condition of the epic.** Own-app use still needs no review at Standard Access, and the resumable upload exists for video with Facebook Login. Neither STOP condition is true.

## 16. The cases offered and the cases not offered

| Case | Offered | Why |
| --- | --- | --- |
| A vertical video as an Instagram Reel | yes | the resumable upload of section 15 |
| A vertical video as a Facebook Page Reel | yes | `video_reels`, `upload_url` on `rupload.facebook.com` |
| A Facebook Page video that is not a Reel (longer than 90 seconds, or not 9:16) | no, for now | it needs the chunked `/<PAGE_ID>/videos` upload with its own contract; the flow of the product is the short vertical video (PRD-004 section 3). A post outside the Reel limits is refused by `PublishValidate` with the exact limit, so nothing is sent. Recorded as a follow-up |
| A thumbnail or cover for either network | no | the cover of an Instagram Reel is `cover_url`, a public address, and the Facebook Reel has no cover field in the steps read. The connectors publish only `video`; a post with a thumbnail is refused with `not_allowed` on `thumbnail`. `thumb_offset` (a frame of the video) is not exposed yet |
| An image post, a carousel, a Story, a comment, a direct message | no | out of scope of epic 19. Image posts wait for epic 27 |
| A personal Facebook profile | no | the API does not publish to one |
| Instagram Login (no Facebook Page) | no | see section 15 |
| A public address of the file | never | no central Stuarty service, and the product never serves the user's media to a third party. `video_url` and `file_url` are never sent (acceptance criterion 3), and a test asserts it |

## 17. Signing in with a Meta app and choosing the account

One Meta app, the user's own, serves both networks. The person makes **one connection per network** (kind `instagram`, kind `facebook`), each with the app id and secret in the vault, and signs in to each; a second consent screen of the same app is a click.

1. **Authorization address.** `https://www.facebook.com/<version>/dialog/oauth` with `response_type=code`, `client_id`, `redirect_uri`, `state`, and the scopes: Facebook `pages_show_list pages_read_engagement pages_manage_posts`; Instagram `pages_show_list pages_read_engagement instagram_basic instagram_content_publish`. The `code_challenge` of the generic flow is sent; whether Meta uses it is one of the points of section 15 that is not stated.
2. **The code** is traded at `https://graph.facebook.com/<version>/oauth/access_token` through the gated client, then that token is traded for a long-lived one (`fb_exchange_token`), then `GET /me/accounts?fields=id,name,access_token,instagram_business_account{id,username}&limit=100` lists the Pages (and, for Instagram, the professional accounts linked to them). Every request carries the token in the `Authorization` header, never in the address, so that no error text of the HTTP client holds a token.
3. **What the vault keeps.** The secret is one JSON document: the app id and secret, and the accounts the sign-in reached with the token of each Page. The user token is not kept: it is used for the list and dropped. The tokens of the Pages are the credential; they have no expiry and stop working when the person removes the app, changes the password or loses the role (code `190`), which the connection shows as `needs_sign_in`.
4. **The picker.** The sign-in does not choose the account: a person with five Pages must not have the first one picked for them. After the sign-in the connection is in the state `needs_account` and the answer holds `choices`, each with an `id` and a `title` (the Page name, or the Instagram username with the Page name; network text, so untrusted). `SocialAccountSelect` (a browser session, scope admin, audited) takes the connection and one of its choices, writes the secret again with **that account's token alone** (the tokens of the others are dropped), and answers the account. An id that is not among the choices is `E_VALIDATION`. An agent cannot call it: the choice of where a post goes is the person's (T-56). A connection with one choice is not chosen for the person: the picker shows the one account and the person confirms it.
5. **The interface.** `social.Chooser` is an optional interface of a connector: `Choices(secret) []Account` and `Choose(secret, id) ([]byte, error)`. A connector that does not implement it (YouTube) is as before. `SignedIn` is true only after a choice, so `PublishValidate` and `PublishNow` refuse a connection with no chosen account (`E_VALIDATION` on `connection_id`, hint: choose the account). The input of the job keeps the id of the account the request named (`account`), and the run sends nothing when the connection, at that moment, signs in as another account or has none chosen: a choice or a sign-in changed while the request waited is `E_INVALID_STATE`, and the person publishes again. This holds for every network, YouTube included.
6. **The connection.** `Endpoint` is `https://graph.facebook.com`; its hosts are `graph.facebook.com` and `rupload.facebook.com`. The upload host and the Graph host are in the connection (epic 04): a request to any other host is refused by the gate.

## 18. The limits, as data, and the media of the post

`core/publish.Limits` gains the fields these networks need, all data with the source and the date already carried by `Limits.Source` and `Limits.Read`: `Preset` (the id of the export preset of `core/export` whose limits apply: `instagram-reels` or `facebook-reels`, so the duration, the frame rate, the resolution, the aspect ratio and the size are written once, in `core/export`, with their pages), `VideoCodecs`, `AudioCodecs`, `CaptionMax` (characters of the caption), `HashtagsMax`, `DailyPosts` (the limit of a moving 24 hours, 100 and 30) and `AIDisclosure` (the network takes the flag).

`publish.File` gains `Media`, the facts of the file: width, height, frame rate, duration, video and audio codec. The service takes them from the result of the export job, which measured them on the written file (`engine/export/verify.go`); a file whose export recorded none (an export of an earlier release) is refused with `required` on `output`, hint: export it again. `Validate` checks them with `export.Preset.Check`, so the message states the exact limit and what the file has, and every finding becomes an issue on `output` with the code of the finding in lower case (`duration_too_long`, `aspect_ratio`, `size_too_large`...). `PublishValidate` therefore refuses a file outside a limit before any request (acceptance criterion 2).

| | Instagram Reel | Facebook Reel |
| --- | --- | --- |
| Container, codecs | `mp4`, `mov`; H.264, HEVC; AAC | `mp4`; H.264, HEVC; AAC |
| Duration | 3 s to 15 min | 3 s to 90 s |
| Frame rate | 23 to 60 | 24 to 60 |
| Picture | width up to 1,920; aspect 0.01:1 to 10:1 | at least 540 x 960; aspect 9:16 within 1% |
| File | at most 300 MB | not stated; the product takes at most 1 GB, a choice of the product recorded here |
| Caption | 2,200 characters, 30 hashtags | not stated; the product applies the 2,200 characters and 30 hashtags of Instagram, which are stricter, so what passes cannot fail on length |
| Visibility | `public` | `public` |
| Posts in 24 hours | 100 | 30 |
| Thumbnail | none | none |

Both networks publish to the audience of the account: there is no private Reel, so the visibility `public` is the only one allowed and the person must say it, because Stuarty never defaults to public (SDD-020 section 3). A post that says `private` or leaves it out is refused with the list of the allowed values. The caption is made by `publish.Caption`: the title, a blank line, the description, a blank line, and the tags as hashtags (`#tag`, spaces removed); the Instagram title is the first line of the caption, since Instagram has no title. `Validate` counts the characters and the hashtags of that text.

## 19. Publishing: the steps at Meta and the errors

**Instagram `Publish`.** (1) `GET /<IG_ID>/content_publishing_limit?fields=quota_usage,config`: when the day is used up, `quota` with the reason `publishing_limit` before any upload. (2) `POST /<IG_ID>/media` creates the container (`REELS`, `resumable`, the caption, `share_to_feed=true`, `is_ai_generated` when the flag is set). The container id and the upload address are kept in the session file of section 7.4 (the connection, the file and the post give the key). (3) The file goes to the upload address in one `POST`. (4) `GET /<container>?fields=status_code` every `PollEvery` until `FINISHED`, at most `Polls` times; `ERROR` and `EXPIRED` are `rejected` (a new container on the next attempt); after the last poll the error is `unavailable` with the reason `still_processing`, retryable, and the session is kept, so the same post again does not upload the file again. (5) `POST /<IG_ID>/media_publish`, then `GET /<media id>?fields=permalink`. `Status` reads `GET /<media id>?fields=id,permalink`: found is `published`.

**Facebook `Publish`.** `upload_phase=start`, the file to the upload address (resumed from `bytes_transfered` of the status when a session is found), `upload_phase=finish` with `video_state=PUBLISHED`, then the status polled until `ready`; `processing` after the last poll is the state `processing`, and `Status` asks again.

Both: the token is the Page token of the chosen account, taken from the secret for the length of one operation and never stored elsewhere; a `401` or code `190` is `auth` (no refresh: a Page token is not refreshed, the person signs in again).

**Errors**, mapped to the kinds of section 11, with a stable `reason` that passed the reason pattern, never the text of Meta:

| Meta | Kind | Reason |
| --- | --- | --- |
| `190` (any subcode), HTTP `401` | `auth` | `token_invalid`, `token_expired` (463), `password_changed` (460), `app_removed` (458) |
| `10`, `200` to `299` | `auth` | `permission_missing` |
| `4`, `17`, `341`, `613`, `32` | `quota` | `rate_limit` |
| `9` / `2207042` | `quota` | `publishing_limit` |
| `25` / `2207050` | `not_eligible` | `account_restricted` (not professional, inactive, checkpointed) |
| `4` / `2207051` | `not_eligible` | `spam` |
| `368` | `not_eligible` | `blocked` |
| `352` / `2207026`, `1363040`, `1363127`, `1363128`, `1363129`, `36004` / `2207010`, `100` | `invalid` | `unsupported_format`, `aspect_ratio`, `resolution`, `duration`, `frame_rate`, `caption_too_long`, `invalid_parameter` |
| container `ERROR` or `EXPIRED`, a Reel `error`, `upload_failed` or `expired`, `6000`, `2207052`, the upload host's `ProcessingFailedError` | `rejected` | `processing_failed`, `media_expired`, `upload_failed` (the session is dropped) |
| `2207032`, `2207053`, the upload host's other errors | `unavailable` | `upload_failed` (the file is sent again) |
| `24` (`2207006`, `2207008`), `2207020` | `unavailable` | `media_expired` (the session is dropped and the next attempt makes a new container) |
| `1`, `2`, `-1`, `-2` (`2207001`, `2207003`), `5xx`, no answer | `unavailable` | `""`, `still_processing` |
| `9007` / `2207027` | `unavailable` | `not_ready` |

An error code not in the table is `unavailable` for a `5xx` and `invalid` with the reason `code_<code>` otherwise. The hint a person reads is written by each connector in its `Description.Hints` (section 22), so a Meta error names Meta and the Instagram or Facebook screen to look at, not YouTube.

## 20. AI disclosure

RF-407. The flag exists on both networks: `is_ai_generated` on the Instagram container and on the `finish` step of a Facebook Reel; on YouTube it is `containsSyntheticMedia`. The service sets `ai_disclosure` of the post when the person or the agent said so **or** when any asset the project holds has the origin `generated` (SDD-000 section 16): the product carries the metadata from the generation to the publication, and never lowers a flag the person set. The rule is conservative (every active asset of the project, not only those a clip uses), because a missing disclosure is the worse mistake; the person can read it in the approval request and turn it off by removing the asset. `Description.AIDisclosure` says whether a network takes the flag; the summary the person approves ends with `AI disclosure: set (a generated asset is used)`, `AI disclosure: set` or `AI disclosure: not set`, and with `AI disclosure: wanted, but <network> takes no such flag` for a network that cannot carry one.

## 21. The setup guide and the picker

The guide of section 8 is written per network from the `Description` (steps, scopes, the date and the page). For Meta it says, in English and Portuguese: create an app of the type "Business" at developers.facebook.com; add "Facebook Login for Business" (or Facebook Login); paste `<origin>/social/callback` as a valid OAuth redirect address; add the app id and secret to Stuarty (the secret goes to the vault); convert the Instagram account to a professional account and link it to the Page (Instagram, Settings, Account type, then Page link); keep the app in development mode with yourself as administrator, which is Standard Access; sign in and accept every permission; **choose the account**. It states what section 15 found (own-app use needs no App Review at Standard Access; the resumable upload; no public address; no cover) and what it did not find. The picker is a list of the `choices` of the connection with one button; it draws what the service answers and computes nothing.

## 22. Operations, variables and tests of part 2

- **Operation added:** `SocialAccountSelect` (admin, browser session only, audited as `social.select`), `POST /social/select`; no command and no MCP tool, as `SocialConnectComplete`. `SocialAccount` gains `choices` and the state `needs_account`. `SocialConnectComplete` answers `needs_account` with the choices for a Meta connection. Added to SDD-000 section 1 and to the scope matrix of SDD-007.
- **`Description` gains** `Hints` (kind to hint, written by the connector) and `AIDisclosure`; `service/publish_error.go` no longer names YouTube: the hint of an error is the one of the connector of the network, with a generic one for a kind the connector did not write.
- **`Session` gains** `ID`, the container or video id.
- **Variable:** none new. With `STUARTY_TEST_SOCIAL_URL` set (SDD-000, the table of variables), the connectors `instagram` and `facebook` of the build send everything, Graph and upload and sign-in, to that loopback server, so `meta-connect.spec.ts` runs a whole flow with no Meta account.
- **Tests:** `go test ./core/publish/...` (limits at their boundaries, the caption); `go test ./service/social/meta/... ./service/social/instagram/... ./service/social/facebook/...` (recorded request and response pairs in `testdata/`, the two-step state machine, polling with a ceiling, every error of section 19, no token in any error, log or result); `go test ./service/... -run TestDisclosure`; `go test -run TestSecurity_ ./...` (rows T-60 and T-61 of SDD-007); `web/e2e/specs/meta-connect.spec.ts` and `task test:web`.
- **Not verified by the product's tests, only by the author's own app** (task 9 of the epic): the points of section 15 marked as not stated; that Meta accepts the `POST` of the token exchange; that a real Reel reaches a real account. The steps are in the hand-off of epic 19.

## Part 3: The calendar and the scheduler (epic 20)

## 23. The publication and its targets

Section 10 sends one post now, as a job. A creator plans: which piece goes to which network, and when. That plan is a **publication** (epic 20).

A publication belongs to one project (ADR-0039) and holds what the networks share: a `title`, a `text` (the default caption; the caption of epic 17 is copied into it when that epic is merged, otherwise the person types it), the `export` (the id of a succeeded export job of the project: the video, checked as `PublishNow` checks it) and an optional `thumbnail` (a succeeded still export). It holds one or more **targets**, one per account it goes to. A target is where the plan meets a network:

| Field | Meaning |
| --- | --- |
| `id` | `tgt_` and 10 characters (SDD-000 section 2) |
| `connection_id`, `network` | the account; the network is the kind of the connection, read when the target is made |
| `fields` | the per-network post: `title` and `description` (each empty means the publication's), `tags`, `visibility`, `made_for_kids`, `ai_disclosure`, the members of `PublishArgs` (section 10) |
| `scheduled_at`, `zone` | the instant, in UTC, and the IANA zone the person thought it in (section 25) |
| `late_window_minutes` | 0 (the default) to 1440: how long after its time a post may still be sent without anyone asking (section 25) |
| `send_now` | true for a target the person asked to send as soon as it is approved: its window is a day (section 25); a new time clears it |
| `state` | section 24 |
| `approval_id` | the pending or last request (section 26) |
| `attempts`, `next_attempt_at` | the automatic retries (section 27) |
| `remote_id`, `url`, `remote_state` | the result of a send; `remote_state` is `processing` or `published` for a network that processes a video |
| `error` | the last failure: `code`, `kind`, `message`, `hint`, `retryable`; the text of the network is never in it (T-58) |

A publication is an item of its project (ADR-0039): it is listed in `ProjectDescribe` and in the overview of the project with kind `publication`. It is kept in its own tables (section 29) and not in `project_items`, because its rows have a state machine, leases and a schedule that no other item has, and because the table of items is rebuilt, with a longer `CHECK`, by every epic that adds a kind; the item the surfaces show is read from the publication.

## 24. The state of a target

`draft`, `waiting_approval`, `approved`, `scheduled`, `publishing`, `published`, `failed`, `missed`, `cancelled`. `core/publish.CanTarget(from, to)` says whether a move is allowed; no other move exists.

```text
draft            -> waiting_approval | cancelled
waiting_approval -> approved | scheduled | missed | draft | cancelled     (approved, or approved too late; rejected or expired; cancelled)
waiting_approval -> waiting_approval                              (rescheduled or edited: a new request replaces the old one)
approved         -> publishing | missed | waiting_approval | cancelled
scheduled        -> publishing | missed | waiting_approval | cancelled
publishing       -> published | failed | scheduled                (a transient error goes back to scheduled with a later attempt)
failed           -> waiting_approval | cancelled
missed           -> waiting_approval | cancelled
published, cancelled: final
```

- `draft`: the person is still planning. Nothing was asked, nothing is reserved.
- `waiting_approval`: the target has a time and an approval request is pending. A rejected or an expired request returns the target to `draft` with an `error` that says why.
- `approved`: a person (or a rule the person wrote) said yes and the time has come; the scheduler takes it. `scheduled`: said yes, and the time is in the future. Approving a request moves a target to `scheduled` when its time is after the moment of the decision and to `approved` otherwise; a time that is already past its late window (section 25) goes to `missed`.
- `publishing`: a scheduler holds the lease of the target and the connector is sending. `published`: the network accepted it; `url` and `remote_id` are set.
- `failed`: the connector refused, or a transient error used its attempts, or the send was interrupted too many times; the `error` is actionable and the person can retry (section 28).
- `missed`: the time passed with nothing running, or the approval came too late (section 25). It waits for a person.
- Editing a target that is `waiting_approval`, `approved` or `scheduled` returns it to `waiting_approval` with a new request: what a person approved is exactly what is sent, and an edit makes it another post. A target that is `publishing` or `published` is not edited.

## 25. Time: the zone, daylight saving and a missed time

- **Instants are stored in UTC** (`scheduled_ms`) with the zone name (`zone`) the person chose. A time given with an offset or `Z` is an instant; a time given as a local date and time (`2026-03-29T02:30`) is read in `zone` by `core/publish.LocalInstant`, which returns the instant and what it had to do: `exact`, `gap` (the wall time does not exist because the clock jumped forward: the instant is the first one after the gap, 03:30 for 02:30 in Lisbon on 2026-03-29) or `ambiguous` (the wall time happens twice because the clock went back: the first of the two). The result of an operation names the zone, the local time of the instant and the adjustment, so a person is told, and a surface shows both the local time and the UTC time. An unknown zone is `E_VALIDATION`. A post scheduled for a wall time in the zone keeps that instant when the zone's rules change later: the instant, not the wall time, is the record.
- **On time.** The scheduler sends a target whose time has come. A target is on time while `now - scheduled_at` is at most `Grace` (60 seconds, for the wake-up of the loop) plus its `late_window_minutes`.
- **Missed (RF-406).** A target in `scheduled` or `approved` whose time is further past than that becomes `missed` the next time anything looks at it (the scheduler at start-up and at every wake; the approval of a late request). It is never sent on its own. The person chooses to send it now (`PublicationSendNow`) or to give it a new time (`PublicationSchedule`); both ask for approval again, since what was approved was a time. A person who turned on `late_window_minutes` accepted a late send of up to that many minutes, and a target inside that window is sent; the scheduler never sends beyond it. A target the person asked to send now (`send_now`) has a window of a day, since a request is answered later than the instant it names; giving the target a time clears `send_now`.
- **A target that is already being retried** (section 27) is not judged by its first time: its attempts are bounded by the retry ceiling.

## 26. Approval and cost of a scheduled post

**Scheduling creates an approval request** of the kind `publish.scheduled` (SDD-015), with no job: `payload_ref` is `publication:<pub_id>/<tgt_id>`, the connection is the account and the project is the project. A rule the person wrote for that kind (and, optionally, the connection and the project) approves it at once, as it does a job; with none, it waits for a person. The summary is what the person reads, at most 500 characters: network, account, title, visibility, the file name and size, the time in the zone of the person with the UTC time (`at 2026-11-20 09:00 America/Sao_Paulo (2026-11-20T12:00:00Z)`), whether the post carries the AI disclosure flag in the words of section 20, and the start of the text in what is left.

- The approval is of **exactly that post**: when the request is made the service resolves the files and stores the post as the person was shown it (`snapshot`: the `publishInput` of section 10, with the size and the digest of each file and the `account` of section 17) and a SHA-256 of it (`payload_sha256`). The scheduler sends the snapshot, checks the digest of the snapshot and `approvedFile` checks the files again; a mismatch is `E_INVALID_STATE` and the target is `failed`, never sent. A connection that, at the time of the send, signs in as another account than the one approved, or has none chosen, sends nothing either: the target is `failed` with `E_INVALID_STATE` (section 17). An edit makes a new snapshot and a new request (section 24).
- Approving, rejecting or letting the request expire moves the target in the same transaction (section 24); the request is rejected with the reason `rescheduled`, `edited` or `cancelled by <actor>` when the person or an agent changes the target while it waits, so an old request can never approve a newer plan.
- **Cost (RF-408).** `social.Description.PostCostMicros` and `PostCostCurrency` are what a network charges for one post (none for YouTube, Instagram and Facebook). It is the estimate of the request, so the approval shows it, the ceilings of SDD-015 section 6 apply to it when the request is made and again when it is approved, and approving reserves it in the ledger, keyed by the id of the target. A target that reaches `published` settles it; one that ends `failed`, `missed` or `cancelled` releases it, and so does an edit, which makes a new request.
- **No agent approves.** `ApprovalDecide` is unchanged: a privileged browser session only. An agent can create, edit, schedule and cancel a publication and read its state; the request waits for a person, or for a rule.

## 27. The scheduler

`service/publish.Scheduler` runs in the server process (with the window closed, the desktop shell keeps it alive, see the last bullets). The owner starts it next to the queue workers and stops it with them.

- **One loop.** It sleeps until the next time something is due (the earliest `scheduled_at` or `next_attempt_at` of an open target, or the expiry of a lease), at most 30 seconds, and wakes early when an operation or an approval changes a target. At every wake it asks the store for the open targets and applies `core/publish.Decide(target, now)`: send, wait or miss. A clock is injected (`Clock`), so tests move time.
- **A lease claims a target.** `Claim` is one conditional write: the state is `scheduled` or `approved` and its time has come, or the state is `publishing` and its lease expired, and nothing else holds the lease. It sets `publishing`, the owner (a random id of the scheduler) and `lease_until` (now plus 2 minutes). Two schedulers, in two processes on one workspace or in one, claim a target once: the second write changes no row. After the claim the scheduler reads the target again and sends the snapshot it holds then, never the one the pass read before the claim (an edit approved again in between changed it). The sender renews the lease every 40 seconds from the claim on, also while the target waits for one of the scheduler's send slots (3 at once), writes its result only if it still holds the lease, and stops a send whose lease it lost or could not renew before it ran out. A scheduler never claims again a target it is still sending.
- **A restart mid-publish.** A target left `publishing` whose lease ran out is taken again as an interrupted attempt: it counts one attempt and the connector is called with the same upload key (section 7.4), so a network that resumes continues the upload instead of starting a second one. When the attempts are used, the target is `failed` with the code `interrupted` and a hint to look at the account before retrying.
- **Retries.** Only an error the connector marks as transient (`core/publish.Transient`: the kind `unavailable`) is retried by the scheduler: at most 5 attempts in all, with the delays 1, 2, 4 and 8 minutes. The target goes back to `scheduled` with `next_attempt_at`. Every other kind (`quota`, `auth`, `not_eligible`, `invalid`, `rejected`) and an exhausted ceiling end the target `failed` with an actionable error; the person retries it.
- **The outcome is one transaction**: the state, the result or the error, the settlement of the cost and the event are written together.
- **Nothing runs without a connection.** The sender reaches the network only through `connections.Manager.With`, with the credential of the connection of the target. A trashed project is not sent.
- **On the desktop, with the window closed (epic 21, ADR-0046, D-078).** The desktop shell starts the server as a child process, so the scheduler lives as long as the shell does. With "keep running in the background" on and at least one target `scheduled`, `approved` or `publishing`, closing the window hides it and the shell stays, with the server and the scheduler, behind a tray icon; the tray menu has Open, Pause publishing and Quit (SDD-012 section 21.3 item 7). The limits are stated to the person in the settings and in the first-time dialog: a post goes out only while the computer is on (it is not woken from sleep) and the person is signed in (the keychain that holds the master key belongs to the user session, so there is no system service), and a post whose time passed while Stuarty was not running is not sent on its own: it is `missed` and waits for the person's decision (RF-406, above). The shell and the window share the one owner of the workspace of SDD-005 section 6; a second desktop launch focuses the first, and the lease above is what makes two senders of one target impossible if two ever ran.
- **Pause publishing.** `Scheduler.SetPaused(true)` makes a pass claim and start nothing; a pass still marks the missed targets (a post that waited past its window is `missed` whether or not anyone paused, so nothing goes out late on resume), and what is sending finishes. The state is in memory: a restart of the process is a resume. The shell sets and reads it with `POST /api/v1/system/scheduler`, a route of the desktop shell (SDD-000 section 1, not a service operation): the body `{"action": "status" | "pause" | "resume"}`, the answer `{"paused": bool, "scheduled": n}` where `scheduled` is the number of open targets the scheduler reads (`scheduled`, `approved` or `publishing`, in a project that is not trashed: a send in flight also keeps the shell resident). `status` takes the `admin` bearer token; `pause` and `resume` also need the launcher secret of SDD-007 section 5.3, so an agent with the token reads the state and never stops a post of the person. The shell starts its server in local mode only, so in network mode the route answers 404 `E_NOT_FOUND` to every caller: there the scheduler runs as long as the server does, and no one pauses it.

## 28. Service operations of the publication

Added to SDD-000 section 1 (domain `publish`) and to the scope matrix of SDD-007:

| Operation | Scope | Result |
| --- | --- | --- |
| `PublicationCreate` | edit | a publication with its targets, every target `draft`; the files and the connections are checked, the posts are validated with `Validate` of the network and the issues come back as `E_VALIDATION` |
| `PublicationGet` | read | one publication with its targets |
| `PublicationList` | read | the publications of a project, a state or a time range (`from`, `to`: the targets whose time falls in it, in the order of the time), with a page cursor |
| `PublicationUpdate` | edit | changes the shared fields (a field left out stays), a target's fields (the `fields` given replace the target's own as a whole; none given leaves them) or its time, adds or removes a target; a target that was waiting or approved goes back to `waiting_approval` (section 24) |
| `PublicationSchedule` | render | gives a target its time (`at`, with `zone`, or as an instant) and asks for approval; a target already waiting or scheduled is rescheduled with a new request |
| `PublicationSendNow` | render | the same with the time of now, for a `draft`, `missed` or `failed` target |
| `PublicationRetry` | render | the same for a `failed` target, with its error cleared |
| `PublicationCancel` | edit | cancels one target (or every open target); its request is rejected and its cost released; `E_INVALID_STATE` while it is `publishing` |

A project-limited key is held to the project of the publication. A caller who may not see the project gets `E_NOT_FOUND`. A change on a target that is `publishing` or `published` is `E_INVALID_STATE`. `PublicationSchedule`, `PublicationSendNow` and `PublicationRetry` always ask for approval: an agent that calls them gets a target in `waiting_approval`, and nothing is sent until a person or a rule approves.

## 29. Tables, events and surfaces of the publication

- **Tables** (SDD-005 section 5.15): `publications` and `publication_targets`, from the migration `create_publications_tables`. Identifiers `pub_` and `tgt_` (SDD-000 section 2).
- **Event.** `publication.updated` on the stream `GET /publications/events` (the frames, heartbeat and limits of SDD-015 section 7, with no replay: a client that connects reads the list first). Its data is the publication with its targets. It is sent after every change of a publication, by an operation, an approval or the scheduler.
- **HTTP**: `POST /publications`, `GET /publications`, `GET /publications/{id}`, `PUT /publications/{id}`, `POST /publications/{id}/schedule`, `POST /publications/{id}/send-now`, `POST /publications/{id}/retry`, `POST /publications/{id}/cancel`, and the stream.
- **CLI**: `publication:create`, `publication:list`, `publication:show`, `publication:update`, `publication:schedule`, `publication:send-now`, `publication:retry`, `publication:cancel`.
- **MCP**: no tool runs them (author decision D-094). The `kind` of `job_start` does not fit a plan of several targets, and the budget of NFR-020 is the author's to spend. The eight operations are in the domain `publish`, so `command_help` with `domain` `publish` and the agent guide name them; an agent plans through the CLI or HTTP, as section 13 says for `PublishNow`. What is left, for the epic that gives MCP room: a `publication` tool with an `action`, as `image` has.

## 30. The calendar and the editor

The page `/publish` of the app (`web/resources/js/pages/Publish/`): a month view and a week view of the targets by their time in the zone of the browser, a list view with filters (state, network, project), the editor of a publication (shared fields, one panel per target with its fields, a preview of the title and the text as the network allows them, the time with its zone, the late window), and for each target its state, its approval (a link to the request), the URL when `published` and the error with its hint when `failed`, with the buttons that the state allows (schedule, send now, retry, cancel). A target is dragged to another day or hour to reschedule it, and the keyboard does the same: the target takes focus, and the arrow keys move it by a day or an hour and Enter confirms. The UI computes no state and no time rule: it sends the instant and the zone, and draws what the service answers. Every string is in English and Portuguese.

## 31. Tests of the publication

| What | Test |
| --- | --- |
| The states and their moves; the zone, the gap and the ambiguous hour; missed time with and without the late window; the retry delays | `go test ./core/publish/...` |
| The operations, the edit after approval, the snapshot | `go test ./service/publish/... -run TestPublication` and `TestPublication*` in `web/app/servicetest` |
| Two schedulers claim once; restart mid-publish; a missed time; a retry; the loop with a clock | `go test -race ./service/publish/... -run TestScheduler` and `TestScheduler*` in `web/app/servicetest` |
| The approval request of a target, a rule that approves, a ceiling, the cost; a scheduled post sends only to the account that was approved | `go test ./service/publish/... -run TestApproval` and `-run TestCost`; `TestPublication*` in `web/app/servicetest` |
| The surfaces agree | `go test ./web/contract/...` |
| The calendar, the list and the editor, in both languages, by keyboard, with the fake connector | `web/e2e/specs/publish-calendar.spec.ts` |
