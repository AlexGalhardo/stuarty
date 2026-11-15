# ADR-0022: A template is a project with named slots

- Status: Accepted (2026-11-14)
- Deciders: author (D-009, D-014, D-019), architect
- Links: [brainstorming D-009, D-014](../brainstorming.md), [features](../search/features-20-80.md), ADR-0005, ADR-0009, SDD-001, SDD-002

## Context

D-009 puts reusable templates in the MVP. Templates are central to use case 2 (commercial videos) and are how an agent produces a consistent result from little input. D-014 limits bundled media to open fonts, so a template cannot ship footage or music. CapCut's community reverse-engineered its draft format precisely to automate templates, which shows that a declarative project format is what makes templates possible.

## Decision

1. A template is an ordinary timeline document with some items marked as **slots**. A slot has a name, a kind (`media`, `text`, `caption`), a description, a required flag and, for media, a duration policy. Named colours are a separate map on the template that styles refer to (SDD-001 §3.10).
2. A template contains no media. Media slots are placeholders; text slots carry default text. Fonts come from the bundled set (ADR-0012).
3. Two service operations implement the feature (SDD-000 §1). `TemplateSave` copies a project into the template library, replacing the assets of marked items by placeholders. `TemplateInstantiate` creates a new project from a template and a map of slot name to value; missing required slots are reported. A media value names an asset of any project the caller can see; the asset is re-added to the new project, and one the caller cannot see is `E_NOT_FOUND`, exactly as one that does not exist. Marking slots is done with the document commands `slot.mark` and `slot.unmark` (SDD-002 §B.9).
4. Filling a slot later is an ordinary command (`slot.fill`), so a human can instantiate with gaps and fill in the UI.
5. Templates live in the workspace and can be exported and imported as a file (a document plus any user fonts), which is how they are shared.
6. A media slot's duration policy is explicit: `fit` (the item keeps its duration and the media must be at least that long) or `stretch_timeline` (the item takes the remaining duration of the media and following items ripple).
7. The lint reports unfilled required slots as errors, so an export of an incomplete instance fails with a clear message.
8. Three starter templates ship with the product (D-019), as documents without media: a vertical cut with captions, a product teaser with animated text, and a quote card. They are embedded in the binary, copied into the template library of a new workspace, and are the fixtures of the template tests.

## Alternatives

- **A separate template format.** Two formats to migrate and two renderers of meaning.
- **Templates as code** (functions that build a timeline). Powerful for developers, opaque to the UI and unsafe to share.
- **Bundle a starter pack of media.** Excluded by D-014.

## Consequences

Positive: no new engine concept; a template renders through the same plan and commands.

Positive: an agent can list a template's slots as text and fill them in one call.

Negative: templates depend on a stable document schema, so this is the last ALPHA feature (D-009); a schema migration must also migrate stored templates.

Negative: without bundled media, templates ship as "bring your own footage", which is less impressive out of the box than commercial products.
