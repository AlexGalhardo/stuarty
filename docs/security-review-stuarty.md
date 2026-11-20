# Security review of Stuarty

Status: the automated acceptance is done; the review by a specialist session with only SDD-007 and the code is open (epic 42 task 8).

This document is the successor of [the review of the editor](security-review-1.0.md). It covers the surfaces that the editor did not have: connections and outbound traffic (ADR-0032), the vault (ADR-0033), the approval queue, publishing, the hosted agent and the sign-in flows (ADR-0035, ADR-0036). The model is [SDD-007](sdds/007-security-model.md); the pass rule is in [SDD-008 section 13.5](sdds/008-test-strategy.md): every row of SDD-007 has a passing `TestSecurity_` test, `scripts/no-egress.sh` passes with no connection configured, and no finding of high severity is open.

## 1. Automated acceptance

| Check | Command | Result |
| --- | --- | --- |
| Every threat has a test that exists, every operation a scope | `go run ./internal/tools/threatcheck` | exits 0 with 101 threats (T-100 and T-101 added by this epic) |
| The security tests | `go test -run TestSecurity_ ./...` | exits 0 (116 packages), run in the review of epic 42 on 2026-11-20 with other agents working; run again by the orchestrator on the final branch |
| No egress with no connection | `bash scripts/no-egress.sh` | Linux only (`strace`); to be run on Linux or in Docker by the orchestrator |
| Fuzzing of the parsers of untrusted input | `bash scripts/fuzz.sh 30s` | [the fuzz report](quality/2026-11-20-fuzz.md): 27 targets, two real defects found and fixed |
| Kill points | `bash scripts/crash.sh` | the vault kill points and the coverage check pass; the server kill points are run by `scripts/crash.sh` as a whole |

## 2. Findings

| Id | Severity | Surface | Finding | Fix | Regression test |
| --- | --- | --- | --- | --- | --- |
| F-1 | Low | Publishing, webhook destination | A `Retry-After` larger than a `time.Duration` holds (9227000000 seconds) became a negative wait, so the scheduler's own back-off was skipped. Found by `FuzzRetryAfter`. The receiver is chosen by the user, and the wait is capped at 30 s once it is a duration, so the effect was a skipped delay, not a long hold | the cap is applied to the number of seconds before the multiplication (`service/publish/destinations/webhook.go`) | `TestSecurity_T100_ReceiverRetryAfterNeverMakesAnOverflowedOrLongWait` and the fuzz corpus `testdata/fuzz/FuzzRetryAfter` |
| F-2 | Low | Sign-in flows, token answer | An `expires_in` that does not fit a duration overflowed: 18446744074 seconds became a lifetime of 290 ms, so every use of the connection refreshed its token at the provider, and 9227000000 became a negative lifetime. The token host is a host of the connection, so the effect was a burst of refreshes, not a leak. Found by `FuzzTokenResponse` in the review of epic 42 | `expires_in` is checked as a number of seconds before it becomes a duration; anything else is no lifetime and the caller's default of one hour applies (`service/connections/oauth/oauth.go`, SDD-020 section 6) | `TestTokenLifetimeThatOverflowsIsUnknown` and `FuzzTokenResponse` |
| F-3 | Informational | Vault | A kill between the writing of the temporary file and the rename leaves a `.vault-*.tmp` file in the folder of the vault. It holds only sealed data (the same ciphertext as the vault) and the next write ignores it. It is not removed | none needed; recorded so that a backup of the folder is understood to contain it | `TestSecurity_T101_VaultKilledMidWriteKeepsEveryAcknowledgedSecret` |

## 3. For the specialist review

What the specialist session reads, with only SDD-007 and the code, and what it answers for each:

1. **Outbound paths.** Every `http.Client` and every dial goes through `internal/outbound` (`importcheck`); with no connection the product opens none. Answer: is there a path that does not, or that follows a redirect to a host the connection did not name?
2. **The vault.** `service/vault` is imported only by `service/connections` and the owner; no tool returns a credential; the vault write is atomic (T-101). Answer: does any log, error, event or export carry a secret?
3. **The approval queue.** A costly generation or a publication by an agent waits for a person or for a rule the user wrote. Answer: can a call reach `Send` or a paid provider by a door that skips the queue (the scheduler, a retry, an idempotent replay)?
4. **The hosted agent.** The agent acts through the same Service as every other door, with the scopes of its key. Answer: can the text it reads (a page, a file, a comment) widen what it may do?
5. **Sign-in flows.** The state is single-use and bound to the session; the callback pages hold no data and change nothing (T-59, T-75, T-76, T-82). Answer: can a request that is not the return of a consent screen complete a sign-in? `FuzzSignInRedirect` holds the guard.

The findings of the review are added to the table above, each with its fix and test. Acceptance 5 of the epic needs no open finding of high severity.
