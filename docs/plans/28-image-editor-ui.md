# 28: Image editor in the UI

## Goal

A creator opens "Edit Image", sees their thumbnail on a canvas, moves a title, swaps a photo, applies their brand colours, and exports: with the same undo history their agent uses, so either of them can continue the other's work.

## Context and related decisions

- ADR-0038, ADR-0041 (one web UI), ADR-0007.
- Author decisions D-062, D-075. PRD-004 RF-341, RF-342, NFR-065.
- SDD-012 (thin client), SDD-021, `docs/design-system.md`.

## Scope

- Modules: `web/resources/js/pages/EditImage/`, `web/resources/js/image-editor/` (new: canvas, layer panel, inspector, toolbar), `web/routes/pages.go`, `web/app/controllers/` (page controller), `web/resources/lang/*.json`, SDD-012 (an image editor section), the design system.
- Everything the catalogue of epic 27 offers, operable by pointer and by keyboard.

## Out of scope

- Adjustments, filters and masks controls (they arrive with epic 29, in the same inspector).
- Operations with a model (epic 30).
- Freehand drawing, pixel selections.

## Dependencies

07, 27.

## Technical design

- **Thin client**: every gesture becomes a command batch sent to the server; the canvas shows what the server's plan says. During a drag the UI previews with a transient transform of the already rendered layer, and the result is replaced by the server's answer on release (the same pattern the timeline uses); `bun run check:thin-client` covers the new folders.
- **Canvas**: the rendered document comes from the renderer. If epic 26 is merged, the same worker draws the one-frame plan in the browser; otherwise the server renders previews at screen size on request and the UI shows the image, and the switch to the worker is one task of whichever epic merges second. Zoom, pan, fit, guides and snapping to canvas and layer edges (snap targets come from the server's description of the layers).
- **Layer panel**: tree with drag to reorder and group, visibility, lock, rename.
- **Inspector**: fields per layer kind bound to `layer.transform`, `layer.set`, `text.set`, `shape.set`, `layer.crop`; numeric fields accept the same units as the video inspector.
- **Brand kit** (RF-342): the existing brand kit (logo, colours, fonts) is offered as swatches, font choices and a "place logo" action.
- **Templates**: "New from template" lists image templates and asks for the slot values.
- **Export dialog**: format, preset size, quality; the lint findings are shown before export with a link to each offending layer.
- **History**: undo and redo per actor, and a visible list of recent operations with their actor (person or agent).
- **Accessibility**: every action has a keyboard path; the layer panel is a tree with the proper roles; the canvas selection is announced.

## Tasks

- [x] 1. Add the image editor section to SDD-012 and its components to the design system. Verify: `task lint:md` exits 0.
- [x] 2. Build the page, the document loading and the canvas with zoom and pan. Verify: `task test:web` passes with the new component tests.
- [x] 3. Implement selection, move, scale and rotate on the canvas with snapping. Verify: `web/e2e/specs/image-transform.spec.ts` passes and `bun run check:thin-client` passes.
- [x] 4. Build the layer panel. Verify: `web/e2e/specs/image-layers.spec.ts` passes.
- [x] 5. Build the inspector for image, text, shape and fill layers. Verify: `web/e2e/specs/image-inspector.spec.ts` passes.
- [x] 6. Add the brand kit actions and "new from template". Verify: `web/e2e/specs/image-template-brand.spec.ts` passes.
- [x] 7. Build the export dialog with lint findings. Verify: `web/e2e/specs/image-export.spec.ts` passes and the downloaded file equals the CLI export of the same document.
- [x] 8. Add history, keyboard paths and both languages. Verify: `bun run check:i18n`, `bun run check:contrast` and `web/e2e/specs/a11y-image-editor.spec.ts` (axe and keyboard) pass.
- [ ] 9. Walk the editor in the Windows shell. Verify: screenshots in the hand-off.

## Test strategy

- Unit (Vitest): canvas maths for display only (zoom, pan, hit testing), panel state.
- End to end: each area above, plus a mixed session where an agent command arrives over SSE while the person edits.
- Accessibility: axe and keyboard-only specs.

## Acceptance criteria

1. A person can build the thumbnail of the reference scenario of epic 27 using only the UI, and export it.
2. A command sent by an agent appears on the canvas without reloading, and the person can undo their own last step without undoing the agent's.
3. Every operation of the epic 27 catalogue is reachable from the UI by pointer and by keyboard.
4. The exported file from the UI is byte-identical to the CLI export of the same revision.
5. `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n`, `bun run check:contrast` and `task test:web` pass; axe reports no violation.
6. The editor works in the Wails shell on Windows.

## Risks and mitigation

- **Rules leaking into the canvas code** (snapping, alignment). Mitigation: the server provides snap targets and performs alignment; the thin-client check.
- **Preview latency when the server renders.** Mitigation: transient transforms during a drag; the worker path of epic 26.
- **STOP** before adding a canvas or drag library.

## Size

L

## Hand-off for review

The diff of `epic/28-image-editor-ui`; screenshots in both themes; the outputs of the acceptance commands; which preview path is in use (worker or server).
