# ADR-0036: One AI provider layer with four ways to connect

- Status: Accepted (2026-11-18)
- Deciders: author
- Amends: ADR-0021 (transcription becomes one kind of provider call)
- Links: author decisions D-059, D-061; ADR-0032, ADR-0033, ADR-0037; PRD-004

## Context

Stuarty uses AI for text (scripts, copy, blog posts), images, voice and video. The user brings their own access; the project never pays for, resells or intermediates usage. The research read the vendors' rules on 2026-11-18 (research note `web/03-integracao-ia`):

- An **API key** works with every provider that has an API.
- **OpenAI** offers "Sign in with ChatGPT" to open source and locally hosted applications: an eligible subscriber uses their plan through an OAuth client with no client secret, on a laptop or on a self-hosted machine.
- **OpenRouter** offers an OAuth flow with PKCE that gives the application an API key of the user's account.
- **Anthropic** does not permit third-party applications to offer Claude.ai login or to route requests through subscription credentials. It permits running the unmodified Claude Code, each user authenticating themselves.
- **Midjourney** has no official API and forbids automation.

## Decision

1. **One provider registry in `service/`**, with one adapter per provider behind one interface per capability: text, image, voice, video, transcription.
2. **Four ways to connect, chosen per provider:** an API key; Sign in with ChatGPT; the OpenRouter OAuth flow; and the user's local agent (ADR-0037). Credentials go to the vault (ADR-0033).
3. **Never:** a Claude.ai subscription login inside Stuarty, a credential shared between users, or automation of a product that has no API. Midjourney is not integrated.
4. **Image, voice and video generation are all in scope from the first provider epics.** Every call is a job with an estimate before, a recorded cost after and the ceilings and approval of ADR-0032.
5. **The model catalogue is read from the provider at run time** where the provider publishes one; prices and model names are never fixed in code as facts.
6. **Provenance is kept.** An asset made by a provider records the provider, the model, the prompt and the date, so the networks' AI disclosure fields can be filled.
7. **The first set of providers** is chosen by the provider epic and approved by the author.

## Alternatives

- **Leave every provider to the user's agent**, as author decision D-041 did. Rejected by the author: a person without an agent could generate nothing from the UI.
- **API keys only.** Rejected by the author: subscribers would pay twice.

## Consequences

Positive: a subscriber of ChatGPT or of an agent uses what they already pay for, inside each vendor's rules.

Negative: vendor policies change (Anthropic changed its enforcement several times in 2026, and OpenAI's programme is new); no feature may depend on a single way to connect.

Negative: cost control is not optional: video generation is expensive enough for an agent to spend real money by mistake.
