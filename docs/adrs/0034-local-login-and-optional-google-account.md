# ADR-0034: Login stays local; a Google account is an optional connection

- Status: Accepted (2026-11-18)
- Deciders: author
- Amends: ADR-0019 item 3 (no social login)
- Links: author decisions D-057, D-012; ADR-0019, ADR-0033, ADR-0035

## Context

ADR-0019 gives two modes: local (loopback, no login, a local token) and network (login with a password, users created from the command line). The first brief of Stuarty asked for a Google login on the desktop and on a VPS.

In a product whose data is local, a Google login does not protect the files: whoever reaches the machine reaches the workspace. It identifies the user and gives access to Google APIs such as YouTube. Someone must own the OAuth client: the project (a central component, with Google's verification) or each user. Google lets a project whose owner is its only user skip verification (research note `web/07-stack-desktop-web-self-hosted`).

## Decision

1. **The two modes of ADR-0019 stay.** A desktop opens without login; a server asks for a password.
2. **Connecting a Google account is optional.** It uses an OAuth client that the user created in their own Google Cloud project, with the loopback redirect and PKCE on a desktop and an HTTPS redirect on a server. The same client gives access to YouTube (ADR-0035).
3. **On a server, a connected Google account may also be accepted as the login** of the user who connected it. The password stays as the way back in.
4. **There is no official Stuarty OAuth client.** The project ships no client identifier of its own.

## Alternatives

- **Google login required, with the user's own client.** Nobody could open the product before creating a Google Cloud project.
- **Google login required, with an official client.** One click, but the project becomes a party to every login and needs Google's verification.

## Consequences

Positive: the product works with no Internet access and with no Google account.

Negative: two ways in on a server must be tested.

Negative: creating a Google Cloud project is friction; the Connections section needs a guided setup that checks each step.
