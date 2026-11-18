# ADR-0019: Authentication: a local token on loopback, login and scoped keys on the network

- Status: Accepted (2026-11-14); item 3 (no social login) amended by [ADR-0034](0034-local-login-and-optional-google-account.md) (2026-11-18)
- Deciders: author (D-012), architect
- Links: author decision D-012, agents and MCP §9, ADR-0008, ADR-0020, SDD-007

## Context

D-012: no login when the server runs on the local machine; login and API keys with scopes when it is exposed on the network. "No login on localhost" is not "no protection": any web page the user opens can send requests to `127.0.0.1`, and DNS rebinding lets a remote page reach a local server. The MCP security guidance requires local servers to use stdio or a token, and the Resolve MCP checks Host and Origin on loopback. vukem provides sessions, social login and opaque tokens with abilities.

## Decision

1. The server has two modes, chosen by its bind address. **Local mode**: bound to `127.0.0.1` or `::1`. **Network mode**: bound to anything else. Binding to a non-loopback address without network-mode configuration is a start-up error.
2. **Local mode.** At first start (D-012 says install time; there is no installer, so the first start is that moment) the server generates a random local token and stores it in the workspace, in its own `token` file readable only by the owner (`runtime.json` holds only the process id, address and start time and is not secret). The CLI and the stdio MCP server read the token from that file and send it only after the server proves it knows the token. The browser never receives the token: the server opens it through a one-time, short-lived launch URL that sets a same-site, HTTP-only session cookie. Every API request must carry the token (bearer header) or that cookie. Requests are also rejected unless the `Host` header is the loopback address and port, and state-changing requests unless `Origin` matches.
3. **Network mode.** Login is mandatory (vukem sessions). There is no open registration, social login or email in the MVP: the operator creates users from the command line, and each project belongs to the user who created it. Programmatic access uses API keys created by a logged-in user, with scopes and optionally limited to one project. TLS is required, directly or through a reverse proxy, with one exception for a container on the user's own machine: plain HTTP when the public address is exactly `localhost` or `127.0.0.1` (SDD-007 §4.3).
4. Scopes: `read`, `edit`, `media`, `render`, `admin`. Each command and endpoint declares the scope it needs in the command registry.
5. Every operation records an **actor**: `user:<id>`, `key:<id>` or `local` with a client label the caller supplies (for example `local:claude-code`). The actor is stored with the operation (ADR-0006).
6. A token or key is never accepted from a query string, never logged and never returned by a read endpoint.

## Alternatives

- **Login always.** One model, friction for local use and for an agent's first command.
- **No authentication until BETA.** Fast, leaves a self-hosted server open.
- **Trust loopback without a token.** Exposed to cross-site requests and DNS rebinding.

## Consequences

Positive: zero-configuration local use that is still closed to other origins.

Negative: two modes to test; the security tests must cover both.

Negative: on a shared machine, another local user who can read the workspace directory can read the token; the directory permissions are the boundary and this is documented.
