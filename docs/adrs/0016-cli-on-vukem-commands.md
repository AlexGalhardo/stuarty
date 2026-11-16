# ADR-0016: The CLI is built on vukem's command registry and is the primary agent surface

- Status: Accepted (2026-11-14)
- Deciders: architect
- Links: agents and MCP §5, stack trade-offs §6, ADR-0008, SDD-003

## Context

The stack draft proposed cobra with commands like `cutroom render` and `cutroom apply`. vukem already compiles application commands into the app binary through the `vukem.Command` interface (name `namespace:action`, standard `flag` package, exit codes through `ExitError`), and the same binary also serves HTTP. For coding agents the evidence favours a CLI with JSON output and a skill over MCP: Playwright recommends its CLI with skills for most coding agents, and Shotstack's CLI conventions (exit codes 0, 1 and 2, JSON output, a blocking watch, an offline validate) are a model worth copying.

## Decision

1. The CLI is the binary `vukem-video`. Product commands are `vukem.Command` implementations registered by the application; no cobra.
2. Command names follow vukem's `namespace:action` form: `project:list`, `project:create`, `project:show`, `edit`, `command:help`, `history`, `media:import`, `media:inspect`, `view`, `lint`, `export`, `job:show`, `mcp`, `serve`. The list mirrors the MCP tools one to one; the full contract is in SDD-003.
3. Every product command accepts `--json` and then prints exactly one JSON document on standard output and nothing else. Without it, output is a compact human-readable text.
4. Exit codes: `0` success; `1` permanent error (invalid input, precondition failed, not found); `2` transient error that is safe to retry (workspace locked, queue full, revision conflict). The JSON error carries the stable error code.
5. Every write accepts `--dry-run`. Every command that starts a job accepts `--wait` and `--timeout`.
6. `edit` reads the batch from a file or standard input (`--ops -`), so an agent never has to escape JSON in arguments.
7. Paths given to the CLI are resolved to absolute paths by the CLI before they reach `Service`.
8. Help is short: `vukem-video command:help <name>` prints one command's schema, example and errors.
9. The CLI obtains its `Service` locally or from a running server, automatically (ADR-0008).

## Alternatives

- **cobra.** Familiar and well maintained, but it would be a second command mechanism in a binary that already has one, and its subcommand tree would not be listed by vukem's own help.
- **A separate CLI binary that only talks HTTP.** Clean separation, but it would need a server for every use.

## Consequences

Positive: one binary, one command mechanism, one release artifact.

Negative: vukem's command names use a colon and its help format is vukem's. Whether vukem's command runner lets a command control standard output completely (needed for `--json`) and whether built-in vukem commands can be hidden from the product help is unverified; a Prototype task checks it and logs any gap in `vukem-feedback.md`.

Negative: the CLI and the MCP tools are two descriptions of the same operations; both are generated from the registry to keep them identical.
