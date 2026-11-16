# Security review 1.0 of Vukem Video against SDD-007

## Scope and method

- Repository: `C:/dev/vukem-video-worktrees/orch`, branch `release/v0.21.0`, commit `29e5aacc`.
- Date: 2026-11-16.
- Reviewer: independent; wrote none of the code. Inputs were `docs/sdds/007-security-model.md`
  (read in full) and the published repository, nothing else.
- Method: for each threat T-01 to T-37 the stated control was traced to the code that enforces it.
  Every finding below was verified by opening the cited lines. Behaviour that SDD-007 or an ADR
  documents as by design (residual risks of section 14) was not reported.
- Commands run (read-only): `git rev-parse --short HEAD`, `grep`, file reads, and
  `go run ./internal/tools/threatcheck` (exit 0: "every threat has its tests and every operation
  its scope"). `git status` was clean afterwards. `go vet` and `go test -run TestSecurity_` were
  not run.
- Areas read: HTTP guard, authentication, launch and handshake (`web/app/http`, `web/app/auth`),
  key store and service authorization (`service/caller.go`, `service/keys_ops.go`,
  `service/trash_empty.go`, `service/tools_install.go`, `service/doctor.go`), import path guard
  (`service/pathguard`), template, OTIO, font and bundle file reads, FFmpeg argv checker
  (`engine/ffmpeg/ffargs`), untrusted text (`service/untrusted`), outbound clients
  (`internal/outbound`), CLI and workspace client (`web/app/cli`, `service/workspace`), desktop
  shell bridge, navigation, launch and swap (`desktop/shell`), token file ACL, user font
  validation (`engine/raster/userfont.go`), workflows under `.github/workflows` and the
  container Dockerfile.
- No file in the repository was found that tries to instruct the reviewer; the prompt-injection
  strings that exist (`docs/adrs/0020-*`, SDD-007, generated site pages) are documented examples.

## Findings

| ID | Title | Severity | Threat | Evidence | Recommendation |
| --- | --- | --- | --- | --- | --- |
| SR1-01 | Network-mode member session is "privileged" in `Doctor` | medium | T-29, T-04 | `service/doctor.go:174`, `service/tools_install.go:120`, `web/app/auth/network.go:108` | Require the `admin` scope (or local mode) for `privileged`, not `Browser` alone |
| SR1-02 | Font parse timeout abandons a goroutine that keeps running | low (uncertain) | T-21 | `engine/raster/userfont.go:256`, `engine/raster/userfont.go:281` | Bound the parse itself, or cap concurrent and leaked parses |
| SR1-03 | Frames-in-flight cap keyed by a client-chosen label | low | T-18 | `web/app/http/ratelimit.go:223`, `web/app/http/api.go:169` | Key the cap by credential (token digest or session id) |
| SR1-04 | Resolved import path is not re-checked for UNC form | info (uncertain) | T-11 | `service/pathguard/pathguard.go:271`, `service/pathguard/pathguard.go:319` | Run the UNC and device checks on the resolved path too |
| SR1-05 | SDD-007 text is corrupted and its acceptance record is stale | info | T-35, T-10, T-27 | `docs/sdds/007-security-model.md:158`, `docs/sdds/007-security-model.md:1000` | Rewrite the T-35 row; refresh 15.1 rows 25 and 29 |

### SR1-01: network-mode member session is "privileged" in `Doctor`

SDD-007 section 5.2 (row `Doctor`) says: "Server paths and environment details only for local
mode or `admin`; others get versions and capability facts without paths". Section 7.3 adds that
"a member of a network server never learns the server's directories".

`Doctor` decides with `privileged := humanForTools(c) && c.ProjectID == ""`
(`service/doctor.go:174`). `humanForTools` returns true for `c.Browser || c.Operator || (key and
admin)` (`service/tools_install.go:120-122`). In network mode every logged-in user's session
caller is built with `Browser: true` whatever the role (`web/app/auth/network.go:107-108`). A
`member` (scopes read, edit, media, render, no admin) therefore gets:

- the absolute path of the FFmpeg binary in `ffmpeg.binary` (`service/doctor.go:409-411`) and the
  path of `vukem-render` (`rendererChecks`, `service/doctor.go:778`);
- the `environment` check with the names of every set `VUKEM_VIDEO_*` variable and the values of
  the plain ones (`service/doctor.go:287-289`, `environmentCheck`);
- the right to run `verify` (`service/doctor.go:176`), which re-hashes FFmpeg, ffprobe and the
  helpers; the HTTP route passes `Verify` from the request (`web/app/http/view.go:221`).

The workspace path is not leaked (it is set only in local mode, `service/doctor.go:189`) and the
HTTP route does not pass `Bundle` or `GC`, so the impact is bounded to server path and
configuration disclosure plus a serialised CPU cost. The comment on the decision says "The mode
does not decide it", which is the root cause: in network mode `Browser` means "logged in", not
"the person running the machine". The servicetest suite covers network keys
(`web/app/servicetest/doctor_test.go:165-200`) but no network member session.

Recommendation: compute `privileged` as local mode with (`Browser` or `Operator`), or a caller
holding `admin` and not project-limited; add a `TestSecurity_T29_` case with a network member
session that asserts no path and no `environment` check, and that `verify` is refused.

### SR1-02: font parse timeout abandons a goroutine that keeps running

T-21 and section 7.7 item 3 state "a total deadline of 5 seconds", and section 14 item 9 accepts
that a hostile font "can burn CPU for up to its deadline". `guardedParse` starts the parse in a
goroutine (`engine/raster/userfont.go:256`) and on the timer returns `FontParseTimeout`
(`engine/raster/userfont.go:281`), but a goroutine cannot be cancelled: a parse that loops keeps
one core busy until the process exits, beyond the deadline. Repeated `FontImport`, bundle or
template imports (heavy limit 10 a minute) would accumulate such goroutines.

Uncertain: this matters only if the parser (`font.ParseTTF` and the glyph reads of `parseFace`)
can loop or run for a long time on crafted input after the structural checks; the review did not
find such an input. The structural checks before it are sound (size, magic, `numTables` cap,
bounds, overlap).

Recommendation: either make the residual risk text say "until restart", or bound the work: count
abandoned parses and refuse further font validation while one is outstanding (a process-wide
semaphore of 1 with the deadline), so the cost stays at one core at most.

### SR1-03: frames-in-flight cap keyed by a client-chosen label

Section 4.4 says one caller may have 2 frames being rendered at once and that "a client label
never selects a bucket" (stated for the heavy limiter, whose code follows it in
`service/ratelimit.go` `heavyBucket`). The frame cap uses `h.limits.acquire(caller.Actor, route)`
(`web/app/http/ratelimit.go:223`). In local mode the bearer actor is `local:<label>` where the
label comes from the `Vukem-Video-Client` header (`web/app/http/api.go:169`), so one holder of
the local token can open any number of buckets by rotating labels and lift the 2-frame cap.

Impact is low: it needs the local token, which already has every scope, and each frame run keeps
its 30 s deadline. It is still a gap against the stated per-caller ceiling (T-18).

Recommendation: key the in-flight cap by the credential, as the request limiter below it already
does (`"token:"+digest(tok)` or `"session:"+digest(id)`).

### SR1-04: resolved import path is not re-checked for UNC form

Section 7.1 step 4 refuses UNC and device paths so that Windows never connects to another host
(T-11). `Clean` applies this to the caller's string, then `resolve` calls `filepath.EvalSymlinks`
(`service/pathguard/pathguard.go:271`), and with no import roots the real path is opened as is
(`service/pathguard/pathguard.go:319`). A symbolic link on a local drive that targets
`\\host\share\file` would therefore be followed. With roots set, the root check confines it.

Uncertain and low value to an attacker: planting such a link needs the symlink privilege
(developer mode or admin) or an existing link, and the remote host is already contacted during
`EvalSymlinks`. It is reported as info so the defence holds by construction.

Recommendation: after `resolve`, refuse a result that starts with `\\` or `//` (and the extended
prefixes) on Windows, before any `Lstat` or `Open` of it; add the case to
`TestSecurity_T11_URLAndUNCPathsRefused` where the platform allows creating the link.

### SR1-05: SDD-007 text is corrupted and its acceptance record is stale

- The T-35 row (`docs/sdds/007-security-model.md:158`) repeats its "How it happens" sentence five
  times, repeats mitigation clauses, and lists its tests four times with missing separators
  between the copies. The contract for the desktop update channel is hard to read and review.
- The acceptance record of 15.1 says no workflow has a `no-egress` or `strace` job and that
  dependency review and the FFmpeg freshness job are missing (`docs/sdds/007-security-model.md:1000`,
  `:1004`). `ci.yml` now has `no-egress` (line 717, `scripts/no-egress.sh` under `strace`) and
  `dependency-review` (line 739), and `ffmpeg-freshness.yml` exists. Section 12 already says
  "built", so the document contradicts itself.

Recommendation: rewrite the T-35 row once (one sentence per control, one test list) and add a new
acceptance record or mark rows 25 and 29 of 15.1 as superseded.

## Threats checked with no finding

- T-01: `guardWith` checks `Sec-Fetch-Site`, exact `Origin`, no CORS header, `OPTIONS` 405
  (`web/app/http/guard.go:62-72`, `:181-207`); cookie writes need `Origin` or `Sec-Fetch-Site`
  and `application/json` (`web/app/http/api.go:287-310`); cookie `SameSite=Strict`
  (`web/app/http/session.go:37-40`).
- T-02: loopback peer after unmapping, no zone, relay headers refused, `Host` byte-equal to three
  values with the listening port (`web/app/http/guard.go:116-179`).
- T-03: token created `O_EXCL` 0600, ACL restricted before the write, then renamed
  (`service/workspace/token.go:81-105`); Windows check requires protected DACL and only user,
  SYSTEM, Administrators allow entries (`service/workspace/token_windows.go:73-122`).
- T-04: credential query parameters refused (`web/app/http/guard.go:74-86`); access log carries no
  query or header (`web/app/http/middleware/accesslog.go`); launcher secret removed from the
  environment at start (`web/app/owner/owner.go:312-314`).
- T-05: `authorize` refuses a missing caller and a missing scope; scope table in
  `service/caller.go:52-99`; bundle import checks both edit and media
  (`service/bundle_import.go:86-91`); `threatcheck` passes.
- T-06: project limit in `authorizeScope` (`service/caller.go:134-137`); templates filtered by
  `visible` or shared (`service/template.go:298`); bitmap keys validated and looked up per
  project (`service/plan.go:272-278`).
- T-07, T-08: `untrusted.Clean` removes controls, bidi, invisible and tag characters and reduces
  joiner runs; `Escape` writes `&` and `<` (`service/untrusted/clean.go`).
- T-10: outbound clients only in `internal/outbound`; forbidigo bans direct dials and clients
  (`.golangci.yml:142-147`); the only exceptions are loopback clients of the desktop shell and
  `open` (`desktop/shell/launch.go:47`, `web/cmd/app/open.go:62`), which send no secret before
  the handshake.
- T-11, T-12: `Clean` refuses schemes, UNC, extended prefixes, ADS, reserved names, `..`
  (`service/pathguard/pathguard.go:57-148`); roots compared per component; `os.OpenRoot`; Lstat
  plus `SameFile` on the handle (`:256-327`); network mode answers alike (`late`, `:333-356`).
- T-13: export download served from the job row, slugged name of letters and digits only
  (`service/export_file.go:109-147`, `web/app/http/files.go`).
- T-14, T-15: `ffargs.Check` runs before every ffmpeg and ffprobe exec
  (`engine/ffmpeg/run.go:209`, `:262`); grammar is an allowlist with `-protocol_whitelist file,pipe`
  first, `-format_whitelist`, `-sn -dn`, `file:` or `pipe:` paths, filter allowlist with typed
  values (`engine/ffmpeg/ffargs/check.go`, `grammar.go`, `filter.go`).
- T-16, T-37: `checkNotRoot` in `Runner.exec` (`engine/ffmpeg/run.go:309-312`); the GPU helper is
  refused under the sandbox (`engine/ffmpeg/helper.go:58-60`).
- T-20: template archive policy: size, entry count, total and per-entry caps, allowlisted flat
  names after NFC and case folding, no symlink or directory entries, font entries checked against
  their hash (`service/archive_policy.go`); desktop update zip extracted through `os.Root` with
  link targets confined (`desktop/shell/swap.go:257-389`).
- T-21 (structure): size, magic, `numTables` at most 64, bounds and overlap checks before parsing
  (`engine/raster/userfont.go:102-226`); see SR1-02 for the deadline.
- T-22: no `innerHTML`, `outerHTML`, `insertAdjacentHTML`, `document.write`,
  `dangerouslySetInnerHTML`, `eval` or `new Function` in `web/resources/js` outside tests; login
  `next` is same-origin only (`web/resources/js/lib/safeNext.ts`).
- T-26: helper download client is HTTPS only, host allowlist at every hop, public addresses only,
  no proxy, at most 5 redirects (`internal/outbound/outbound.go:66-118`).
- T-27: every `uses:` is pinned by a 40-character SHA with a version comment (a CI step enforces
  it, `ci.yml:676`); every workflow defaults to `contents: read`; write scopes only on release
  jobs (`release.yml:133-136`, `:311`, `:363`, `:411-415`); untrusted values reach `run:` only
  through `env:`; container base images pinned by digest and FFmpeg by SHA-256
  (`.config/docker/vukem-video.Dockerfile:15-17`, `:79-80`).
- T-28: `TrashEmpty` needs the `admin` scope and a browser session or a key
  (`service/trash_empty.go:40-63`); local cookie sessions are privileged only from a code issued
  with the launcher secret, compared in constant time on SHA-256 digests
  (`web/app/auth/launch.go:35-44`, `web/app/http/session.go:51-60`).
- T-30: client verifies the HMAC proof before sending the token (`service/workspace/open.go:219`,
  `:252`, `:369-396`) and sends it only through the loopback-only dialer
  (`internal/outbound/outbound.go:21-40`); a remote `VUKEM_VIDEO_URL` over plain HTTP is refused
  unless loopback (`service/workspace/open.go:138-160`); desktop shell verifies the proof before
  the launch request (`desktop/shell/launch.go:49-51`, `:84-114`).
- T-32: launch codes single use, 120 s, 5 failures lock 60 s, digests compared in constant time
  (`web/app/auth/launch.go:99-160`); network keys checked by vukem `Tokens.Find` and disabled
  users refused (`web/app/auth/users.go:190-197`).
- T-33: key operations require a logged-in browser session in network mode
  (`service/keys_ops.go:106-122`); effective key scopes are intersected with the owner's present
  role (`web/app/auth/keys.go:244-252`).
- T-35 (desktop shell navigation): only `http://127.0.0.1|localhost:<port>` is allowed, bridge
  messages are dropped from any other origin, reveal accepts only an existing absolute regular
  file and runs without a shell (`desktop/shell/navigate.go`, `desktop/shell/app/bridge.go:26-41`,
  `desktop/shell/reveal.go`); bridge replies are JSON-encoded into the script.
- T-34, T-36, T-23, T-24, T-25, T-31: not traced line by line in this review; their named tests
  exist (`threatcheck` passes) and nothing contradicting them was seen in the code read.
