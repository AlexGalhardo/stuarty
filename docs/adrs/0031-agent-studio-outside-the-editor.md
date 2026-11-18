# ADR-0031: The agent studio lives outside the editor

- Status: Accepted (2026-11-18)
- Deciders: author (D-041, D-042, D-043, D-044)
- Links: ADR-0020, SDD-014, PRD-004

## Context

Vukem Video is an editing toolkit. Agents and people use the same operations. Author decision D-041 keeps generation, provider integrations and the choice of engine on the agent side. Author decisions D-042, D-043 and D-044 add a production folder, Markdown learnings and a one-step way to recover from an agent mistake. None of that is a feature of the editor binary. ADR-0030 already records the Gio desktop window, so this decision is ADR-0031.

## Decision

1. The editor stays a toolkit with no outbound capability. ADR-0020 is unchanged and is cited, not amended.
2. Generation of images, video, voice, music and sound effects, and the credentials of those providers, live on the agent side, through each provider's own MCP server or CLI. The product never stores a provider key.
3. The agent chooses the engine per request: HyperFrames, Remotion, or the editor. The editor is used when footage must be cut, captioned, mixed or adjusted.
4. A production folder holds every video, whatever the engine. The layout is SDD-014.
5. The studio kit is a Claude Code plugin in `studio/`, in the Agent Skills format, versioned with the product's release tags.
6. Learnings are Markdown: one file per video and a global memory folder. The format is SDD-014.
7. Outputs of an external engine enter the editor only as files through `media_import`.

## Alternatives

- **Native provider plugins in `service/`.** Rejected: it reopens ADR-0020 and puts provider keys in the product.
- **A project in the editor for every video.** Rejected: it forces the editor into flows that do not need it.
- **Skills in a separate repository.** Rejected: two versions to keep in step with the generated editor skill.

## Consequences

Positive: plan 002 and later plans cite this ADR and SDD-014 instead of a conversation. A reviewer can reject a provider client in `service/`.

Negative: the agent, not the product, must install provider tools and hold their keys.

Negative: HyperFrames and Remotion are not bundled. Plan 008 records their licences and the versions the author approves.
