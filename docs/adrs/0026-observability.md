# ADR-0026: Observability: local structured logs, nothing sent out by default

- Status: Accepted (2026-11-14)
- Deciders: author (D-016), architect
- Links: [brainstorming D-016](../brainstorming.md), ADR-0006, ADR-0018, SDD-007

## Context

The stack draft listed OpenTelemetry and Sentry, with Sentry opt-in on the desktop. D-016: no telemetry by default; whoever hosts can turn on OpenTelemetry to their own collector; no Sentry, no usage analytics. vukem has structured logging in the core and OpenTelemetry as a separate module.

## Decision

1. The binary makes **no outbound network request by default**. The only requests it ever makes are the ones a user or operator configured: the model and helper downloads by an explicit install step (ADR-0021, announced before they happen), a configured transcription API, and, in the desktop application, a user-requested update check (three kinds of outbound request: helper install, transcription API, update check). A test asserts that a default start and a full edit-and-export scenario open no outbound connection.
2. Logs are structured, written to standard error and to a rotating file in the workspace. Each line carries the request or job identifier, the actor and the project. Media file names and transcript text are not logged above debug level.
3. The **operation log** (ADR-0006) is the audit trail of edits; the **jobs table** (ADR-0018) is the record of long work. Neither is duplicated into logs.
4. OpenTelemetry tracing and metrics are available through vukem's `drivers/otel`, off unless the operator sets an endpoint.
5. No Sentry and no analytics, in the binary or in the documentation site.
6. For bug reports, `vukem-video doctor --bundle` writes a local archive with versions, the FFmpeg configure line, detected encoders, recent logs and, only if the user passes a flag, a project document. The user decides whether to attach it to an issue.

## Alternatives

- **Opt-in Sentry.** Useful for real-media crashes; needs an account, a privacy policy and care with file names.
- **Anonymous usage telemetry.** Real data for the 20/80 cut; rejected by the author.

## Consequences

Positive: "nothing leaves your machine" is a tested property, which matters for a local-first tool that handles people's footage.

Negative: the maintainers learn about failures only from issues; the diagnostic bundle is the mitigation.

Negative: there is no data on which features are used.
