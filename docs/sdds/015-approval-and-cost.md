# SDD-015: The approval queue, rules and cost ceilings

- Status: Draft (2026-11-19). Written by epic 05; the code follows in the same epic.
- Depends on: ADR-0032 (publishing and costly generation wait for a person), ADR-0018 (jobs), ADR-0020 (a privileged browser session), ADR-0033, SDD-000 (names), SDD-005 (jobs and audit), SDD-006 (events), SDD-007 (threat model, scope matrix), SDD-014 (connections)
- Implements: PRD-002 RF-132, RF-135, RF-136; PRD-004 RF-362, RF-363, RF-364, RF-365
- Owner area: `core/approval/`, `service/approval/`, the approval operations and the job wiring in `service/`, the controllers, commands and tool in `web/app/`, the approval queue and the rules screen in `web/resources/js/dashboard/approval/`

## Contents

1. Overview
2. The decision (`core/approval`)
3. The records and the tables
4. Service operations and who may call them
5. Jobs: the state `waiting_approval`
6. The cost ledger and the ceilings
7. Events
8. Errors
9. Audit
10. Surfaces
11. Tests

## 1. Overview

An agent or a schedule can prepare a publication or a costly generation. It does not run: it becomes an item of the approval queue, with what will be sent, where, and the estimated cost. A person approves or rejects it, or a rule the person wrote approves it without asking. Spending is bounded by ceilings that no approval and no rule can raise (ADR-0032 items 3 to 5).

Four rules carry the design:

1. **The decision is pure and lives in `core/approval`.** `Decide` takes an action, the rules, the ceilings and what was spent today, and answers one of three outcomes. No adapter and no screen decides.
2. **A person decides, an agent never does.** `ApprovalDecide`, `ApprovalRuleSet`, `ApprovalRuleRemove` and `CostCeilingSet` need a privileged browser session (the `Caller.Browser` of SDD-007 section 5.3, the same notion that empties the trash). The local token, an API key and the agent surfaces are refused with `E_PRIVILEGED_SESSION`. The check is in the service, not in the adapters.
3. **A job that needs approval does not start.** It is created in the state `waiting_approval`, makes no outbound call and spends nothing until a person (or a rule) approves it.
4. **A ceiling is a hard stop.** An action whose estimate is over a ceiling is refused with an actionable error and records nothing. A person approving a request does not lift a ceiling: the person raises the ceiling first.

This epic builds the mechanism and proves it with a test job kind. Publishing (epics 18 and 20) and generation (epic 09) register their own kinds.

## 2. The decision (`core/approval`)

```go
type Money struct {
	Micros   int64  // millionths of the currency unit; 0 is no cost
	Currency string // three upper-case letters; required when Micros > 0
}

type Action struct {
	Kind       string // the job kind, for example "publish.post" or "generate.image"
	Connection string // the destination connection id; empty when the action has none
	Project    string // the project id; empty when the action belongs to none
	Cost       Money  // the estimate
}

type Rule struct {
	ID, Kind, Connection, Project string // Connection and Project empty mean any
	Currency                      string
	PerItem, PerDay               *int64 // micros; nil means not set
	Enabled                       bool
}

type Ceiling struct {
	Project         string // empty is the workspace
	Currency        string
	PerItem, PerDay *int64 // micros; nil means not set
}

type Spent struct {
	Workspace int64            // spent today, in the currency of the action
	Project   int64            // spent today on the project of the action
	ByRule    map[string]int64 // spent today on the actions each rule covers
}

func Decide(a Action, rules []Rule, ceilings []Ceiling, spent Spent) Outcome
```

`Outcome` is `NeedsPerson`, `AutoApproved{RuleID}` or `Refused{Reason}`. The order of the checks is fixed:

1. **The action is valid**: a non-negative cost, a currency when the cost is above zero, a kind. Otherwise `Refused` with the reason `invalid`.
2. **The ceilings, first and always.** An action with a cost above zero is checked against every ceiling in its currency that applies to it: the workspace ceiling, and the ceiling of its project. A ceiling with `PerItem` refuses an estimate above it (reason `per_item`); a ceiling with `PerDay` refuses when `spent + estimate` is above it (reason `per_day`). The refusal names the scope (`workspace` or `project`), the limit, what is spent and the estimate. An action with no cost is never refused by a ceiling.
3. **The rules.** A rule matches when it is enabled and its kind equals the action's kind, and its connection and its project are empty or equal the action's. For an action with a cost above zero the rule must also have `PerItem` set, the same currency, an estimate within `PerItem`, and, when `PerDay` is set, `ByRule[rule] + estimate` within `PerDay`; a rule with no `PerItem` never approves a costly action, so a rule written for posts cannot approve a generation. When several rules match, the most specific wins (the one with a connection and a project, then one of the two, then neither); among equals the first in the order given, which is the order of creation. The outcome is `AutoApproved` with that rule's id.
4. Otherwise `NeedsPerson`.

A rule that is over its own ceiling does not match, so the action goes to a person: only a ceiling refuses.

## 3. The records and the tables

Identifiers (SDD-000 section 2): `apr_` for a request, `rul_` for a rule, `led_` for a ledger entry; each is the prefix, an underscore and 10 lower-case Crockford base32 characters.

**An approval request** (`approval_requests`):

| Field | Type | Rule |
| --- | --- | --- |
| `id` | string | `apr_...` |
| `actor` | string | who prepared the action (`key:`, `local:` or `user:`, SDD-000 section 3) |
| `owner_user_id` | string | network mode: the user whose session or key prepared it; empty in local mode |
| `kind` | string | the kind of the action |
| `summary` | string | what will be sent or generated, 1 to 500 characters of valid UTF-8. It is what a person reads before approving, so it must not be able to look like something else: every control character (Unicode category Cc, which holds the C1 controls) except a line feed, every format character (Cf: the bidirectional controls, the zero-width characters, the byte order mark, the tag characters), the line and paragraph separators and every character `service/untrusted` removes are refused. It is text a job kind or an agent wrote, so a surface that shows it to an agent delimits it as untrusted text (SDD-007 section 8). The `reason` of a rejection follows the same rule |
| `connection_id` | string or null | the destination; the request refuses to be approved once the connection is gone |
| `project_id` | string or null | |
| `job_id` | string or null | the job that waits; a request is made only for a job in this epic |
| `payload_ref` | string | what will be sent: `job:<id>` here; a later epic may name a draft or a document |
| `estimate` | `{micros, currency}` | the estimated cost; `micros` 0 for no cost |
| `state` | string | `pending`, `approved`, `rejected` or `expired` |
| `decided_by` | string or null | the actor of the person, or `rule:<id>` for an automatic approval |
| `rule_id` | string or null | the rule that approved it |
| `reason` | string | the note of the person when rejecting, at most 500 characters |
| `created_at`, `decided_at`, `expires_at` | time | RFC 3339, UTC. A pending request expires 7 days after it was made |

A request is `expired` when it is pending at `expires_at`; the service marks it when it lists or decides, rejects its job (`E_APPROVAL_EXPIRED` in the job error) and records the audit row. Only `pending` moves, and it moves once: the update is conditional on the state, so two people approving at the same time decide once.

**An approval rule** (`approval_rules`): `id`, `kind`, `connection_id` (empty means any), `project_id` (empty means any), `currency`, `per_item_micros`, `per_day_micros` (null when not set), `enabled`, `created_by`, `created_at`. At most 100 rules (`E_LIMIT_EXCEEDED`). `kind` is 1 to 80 characters of `[a-z0-9_.:-]`. A rule is created and changed only from a privileged session.

**A ceiling** (`cost_ceilings`): `project_id` (empty is the workspace), `currency`, `per_item_micros`, `per_day_micros`, `updated_by`, `updated_at`; one row per project and currency. These are the workspace settings of ADR-0032 item 5. With no row there is no ceiling in that scope and currency: a fresh workspace spends nothing, because every costly action still waits for a person.

**A ledger entry** (`cost_ledger`): `id`, `request_id`, `job_id`, `connection_id`, `project_id`, `kind`, `currency`, `estimate_micros`, `actual_micros` (null until the job ends), `state` (`reserved`, `settled` or `released`), `day` (the UTC date of the reservation, `YYYY-MM-DD`), `created_at`, `settled_at`.

Migrations: `create_approval_tables` (the four tables above), and `jobs_waiting_approval`, which rebuilds `jobs` to allow the state `waiting_approval`, to drop the closed list of kinds (a kind registered by a feature epic must be storable; the service validates the kind) and to add `approval_id`. SQLite cannot change a `CHECK` in place, and nothing references `jobs`.

## 4. Service operations and who may call them

Eight operations on `service.Service`, in `service/approval_ops.go`:

| Operation | Scope | Caller | Result |
| --- | --- | --- | --- |
| `ApprovalList` | read | any | the requests, newest first, filtered by `state` (default `pending`), with a page cursor |
| `ApprovalGet` | read | any | one request; `E_APPROVAL_NOT_FOUND` for an unknown id |
| `ApprovalDecide` | read | a privileged browser session | the request decided. `decision` is `approve` or `reject`, with an optional `reason` |
| `ApprovalRuleList` | read | any | the rules, oldest first |
| `ApprovalRuleSet` | admin | a privileged browser session | the rule created (no `id`) or replaced (an `id`) |
| `ApprovalRuleRemove` | admin | a privileged browser session | the rule removed |
| `CostSummary` | read | any | the ceilings, what is spent today (workspace and per project) and the last 30 days per connection and per project |
| `CostCeilingSet` | admin | a privileged browser session | the ceiling set or, with both limits null, removed |

- **The privileged session.** `service.Scopes` lists `ApprovalDecide` as `read`, as it lists `KeyCreate`; the service then requires `Caller.Browser`. In local mode a browser session is privileged only when its launch code was issued with the desktop launcher secret (SDD-007 section 3.4 item 7), so the desktop app is and a session opened by `stuarty open` is not; the local bearer token never is, whatever label it sends. In network mode a logged-in session is. An API key is never privileged. In network mode the person must also hold the `admin` scope, or be the user the request belongs to (`owner_user_id`); the rule, ceiling and cost-setting operations need `admin`. A refusal for a caller that is not a privileged session is `E_PRIVILEGED_SESSION`, whose hint says to use the Approvals page of the app; a refusal for a missing scope is `E_FORBIDDEN`.
- **Visibility in network mode.** A caller without the `admin` scope lists and reads only the requests of their own user (`owner_user_id`); an administrator reads every request. In local mode every caller reads every request.
- **A project-limited key** may list and read only the requests of its project, and is refused `CostSummary`, the rules and every write (they name no project).
- **Decide approving.** The service checks, in this order: the request exists and is `pending` (`E_INVALID_STATE` otherwise); its connection still exists (`E_CONNECTION_NOT_FOUND`); the ceilings still allow it (section 6: `E_COST_CEILING`, the request stays `pending`). In one transaction it then marks the request `approved`, reserves the cost in the ledger, moves the job from `waiting_approval` to `queued` and enqueues it. Approving is not retried: a second approval of the same request is `E_INVALID_STATE`.
- **Decide rejecting.** The request becomes `rejected` with the reason, the job becomes `cancelled` (the job error is `E_CANCELLED` with the reason), and nothing is reserved.
- **A rule never lowers a check.** A rule is evaluated when the action is submitted, in the same way as a person would see it, and a ceiling applies to it first (section 2).

## 5. Jobs: the state `waiting_approval`

A job kind that needs approval is registered at start-up in `service.Options.JobKinds`, a list of `ApprovableKind`:

```go
type ApprovableKind struct {
	Name  string
	Scope string // the scope needed to submit it; render when empty
	// Prepare checks the arguments and describes the action: the summary, the destination
	// connection, the project, the estimate. It makes no outbound call and opens nothing.
	Prepare func(ctx context.Context, req JobStartRequest) (Prepared, error)
	// Run is the job once approved. It reaches the network only through connections.Manager.With
	// and reports the real cost, also when it fails after spending.
	Run func(ctx context.Context, job JobRun) (JobOutcome, error)
}
```

The product registers none in this epic; the tests register a test kind. `JobStart` with a registered kind:

1. authorizes with the scope of the kind, applies the heavy-operation limit, and calls `Prepare`;
2. calls `Decide` with the rules, the ceilings and the spent amounts;
3. `Refused` is `E_COST_CEILING` and nothing is written;
4. `NeedsPerson` creates the job in `waiting_approval` and the request `pending` in one transaction, writes the audit row `approval.request` and answers with the job (`state` `waiting_approval`, `approval_id` set, and a `text` that says a person must approve it in the app). It sends nothing to the queue;
5. `AutoApproved` does what approving does (section 4), with `decided_by` `rule:<id>` and `rule_id` set, writes the audit row `approval.auto` naming the rule, and answers with the job `queued`.

With `dry_run` the service calls `Prepare` and `Decide` and answers the job it would create (`waiting_approval`, or `queued` when a rule covers it), or the ceiling error; it writes nothing. A submission for a connection that does not exist is `E_CONNECTION_NOT_FOUND`, and a summary that carries a control or invisible character is refused (section 3).

A job in `waiting_approval` is counted as active by `JobList` (state `active` and the default list), is not counted against the concurrency limits (it does not run), and is not touched by the restart recovery of SDD-005 section 14.5 (it is not `running`). `JobCancel` on it rejects the request (reason `cancelled by <actor>`) and cancels the job in the same transaction. Trashing the project of a waiting job does the same.

**The queue is bounded per submitter.** At most 50 requests per submitter wait at once (`MaxPendingPerSubmitter`), and at most 1000 in the workspace (`MaxPendingApprovals`). The submitter is the user in network mode, so every key of one user counts together, and otherwise the actor. A submission past either limit is `E_LIMIT_EXCEEDED` and writes nothing; one agent filling its share does not block another submitter (`TestSecurity_T51_*`).

When an approved job ends, the ledger is settled from the state the job reached (section 6). The job kinds `ingest`, `transcribe`, `export` and `frame` do not need approval and are not touched.

## 6. The cost ledger and the ceilings

- **Before.** An approved request reserves its estimate in the ledger (`reserved`, with `estimate_micros`).
- **After**, once the outcome of the run is final and never before: a job that succeeded is `settled` at the cost it reports, or at its estimate when it reports none; one that failed or was cancelled is `settled` at the cost it reports and is `released` only when it spent nothing, and one cancelled while still `queued` never ran and is `released`; one that was **requeued** (a shutdown interrupted it) keeps its reservation, since it will run again and must stay counted against the ceilings, and what the interrupted run reports as already spent is recorded as a `settled` entry of its own. The real cost is always recorded, and it may be above the estimate. When no reserved entry is left at the end, a `settled` entry is recorded, so what was spent is never lost.
- **Before it spends.** A job that starts running with no reserved entry (released by an earlier end, or lost) has the ceilings checked again and its cost reserved again before `Run` is called; a job that no longer fits fails with `E_COST_CEILING` and does not run (`TestSecurity_T50_*`).
- **Spent today** is, for the UTC day of the reservation, the sum of `actual_micros` of settled entries and of `estimate_micros` of reserved ones, in the currency asked. The workspace figure sums every entry; the project figure the entries of the project; the figure of a rule the entries of the kind, the connection and the project the rule covers. Released entries count for nothing.
- **Enforcement** is `Decide` (section 2), called when an action is submitted and again when a person approves it, since the spent amount changes while a request waits. A ceiling that is hit gives `E_COST_CEILING`, whose `details` hold `scope`, `reason` (`per_item` or `per_day`), `limit_micros`, `spent_micros`, `estimate_micros` and `currency`, and whose hint says what to do: shorten the work, wait for the next UTC day, or ask the person to raise the ceiling in the app. Nothing is written when a ceiling refuses: no job, no request, no ledger entry.
- **`CostSummary`** returns the ceilings, `today` (workspace and per project), and `days` (the last 30 days, per connection and per project) from the ledger. It is a read of numbers the service computed: the page shows them and adds nothing.

## 7. Events

Two events, on a stream of the approval queue that is not tied to a project, `GET /approvals/events` (the same frames, heartbeat and limits as SDD-006 section 3, with no replay: a client that connects first receives the pending requests as `approval.requested` events):

| Event name | When | Payload essentials |
| --- | --- | --- |
| `approval.requested` | a request was made, or exists when the stream opens | the request (id, actor, kind, summary, connection, project, estimate, state, times) |
| `approval.decided` | a request was approved, rejected or expired, by a person, a rule, a cancel or the clock | the request, with `state`, `decided_by` and `rule_id` |

The summary of the payload is untrusted text and is sent as a field of the request, never as markup. The stream needs the `read` scope, counts as one of the streams of the process (128) and, in network mode, of the key (16), and shows a caller without the `admin` scope only the requests of its own user (section 4; a user of network mode has a topic of its own). A key limited to one project is refused with `E_FORBIDDEN`: it lists its project's requests with `approval:list --project`. The job of a request also publishes `job.updated` on its project's stream as always.

## 8. Errors

Added to SDD-000 section 8:

| Code | Meaning | HTTP |
| --- | --- | --- |
| `E_PRIVILEGED_SESSION` | the operation is for a person in a privileged browser session; the caller is the local token, a key or an agent surface | 403 |
| `E_APPROVAL_NOT_FOUND` | the request or the rule does not exist | 404 |
| `E_COST_CEILING` | the estimate is over a ceiling; `details` as in section 6; nothing was written or spent | 422 |
| `E_APPROVAL_EXPIRED` | in the error of a job whose request expired before a person answered | 409 |

A request that is not `pending` is `E_INVALID_STATE`; a bad field is `E_VALIDATION` with `path`; the limits are `E_LIMIT_EXCEEDED`.

## 9. Audit

Rows of the table `audit` (SDD-005 section 5.9), written in the transaction of the action:

| Action | Written by | Subject | `details` |
| --- | --- | --- | --- |
| `approval.request` | a submission that needs a person | the request id | the kind, the connection and the estimate |
| `approval.auto` | a submission a rule approved | the request id | the `rule` id, the kind and the estimate |
| `approval.approve`, `approval.reject` | a person | the request id | the reason of a rejection |
| `approval.expire` | the clock | the request id | |
| `approval.cancel` | `JobCancel` of a waiting job | the request id | |
| `approval.rule.set`, `approval.rule.remove` | a person | the rule id | the kind, the connection, the project and the limits |
| `approval.ceiling.set` | a person | the project id or empty | the currency and the limits |

## 10. Surfaces

- HTTP: `GET /approvals`, `GET /approvals/{id}`, `POST /approvals/{id}/decide`, `GET /approval-rules`, `POST /approval-rules`, `DELETE /approval-rules/{id}`, `GET /cost`, `PUT /cost/ceilings`, and the stream `GET /approvals/events`.
- CLI: `approval:list`, `approval:show`, `approval:decide`, `approval:rule:list`, `approval:rule:set`, `approval:rule:remove`, `cost:show`, `cost:ceiling`. The four that need a person are commands so that an agent that tries gets the stable error and its hint, instead of "unknown command"; run with the local token they always fail with `E_PRIVILEGED_SESSION`.
- MCP: one read-only tool, `approval`, that lists the pending requests or reads one with `id` (536 bytes of the 19,500 of the whole catalogue, SDD-003 section 4.1: the 18 tools total 19,440, so the next tool needs the budget revised); an agent submits through `job_start` and reads the state with `job_get` or this tool. It has no argument that decides, and any other member is refused by the input schema. Deciding, writing a rule and setting a ceiling have no tool (RF-136).
- Agents: `job_start` of a registered kind answers the job in `waiting_approval` with `approval_id`; the job `text` says so. The generated skill of `agent:docs` says that the agent waits, polls `job_get` and never decides.
- The UI page `/approvals` shows the queue (summary, destination, cost, actor, age), approves and rejects, lists the rules and the ceilings and writes them, and shows the cost summary. It is the only caller that is a privileged session, and it computes nothing: amounts are formatted, not summed (SDD-012 section 1).

## 11. Tests

| What | Test |
| --- | --- |
| Every branch of the decision, the ceilings at their boundaries, rule precedence | `core/approval` table tests |
| A caller with the local token or a key is refused by every privileged operation | `TestApproval_PrivilegedOperationsRefuseTheLocalToken` |
| An agent cannot approve its own request, on any surface | `TestSecurity_T48_AgentCannotApproveItsOwnRequest` |
| An agent cannot create, change or remove a rule | `TestSecurity_T49_AgentCannotWriteARule` |
| A ceiling cannot be bypassed by an approval, a rule, a split or a currency | `TestSecurity_T50_CostCeilingCannotBeBypassed` |
| A waiting job makes no outbound call and spends nothing | `TestJobApproval_WaitingJobDoesNotRun` |
| Approve, reject, cancel and expire | `TestJobApproval_Approve`, `_Reject`, `_Cancel`, `_Expire` |
| A rule approves, and the audit names it | `TestJobApproval_RuleAutoApproves` |
| Per item, per day, per project | `TestCeiling_PerItem`, `TestCeiling_PerDay`, `TestCeiling_PerProject` |
| The surfaces agree | `web/contract` parity and the privileged-operations test over the Service, HTTP and the CLI |
| The queue in the browser, live, in both languages | `web/e2e/specs/approval.spec.ts` |
