# ADR-0012: Text and shapes are rasterized by Go code with bundled fonts

- Status: Accepted (2026-11-14); library choice to be confirmed by a Prototype measurement
- Deciders: author (D-001, D-014, D-022), architect
- Links: [post-mortems §1, §6](../search/post-mortems.md), [stack trade-offs §3](../search/stack-tradeoffs.md), ADR-0003, SDD-004

## Context

Text is where preview and export diverge most in the editors studied: fallback fonts when a document is serialized (Remotion #11315), different fonts between Linux and macOS (Remotion #5984), a report of libass ignoring `fontsdir` and using system fonts, Reframe turning captions into hundreds of `drawtext` filters. Captions are mandatory for use case 1 and animated text is central to use case 2.

## Decision

1. `engine/raster` is the only place text is laid out and drawn. It takes a text or caption item's content and style and produces RGBA bitmaps plus metrics (bounding box, baseline, per-word boxes).
2. Shaping uses `go-text/typesetting` (pure Go port of HarfBuzz with bidi and line segmentation); glyph outlines are rasterized with `golang.org/x/image/vector`. Both are permissively licensed and need no cgo.
3. Only **bundled fonts** are used: six open families shipped inside the binary, each with its licence file (D-022): Inter (the default), Montserrat, Anton, Bebas Neue, Playfair Display and JetBrains Mono. The licence of each family and the weights shipped are verified at the source by a ROADMAP task before the fonts are committed. System fonts are never read. A project refers to a font by family and weight from that set; user-supplied font files arrive in BETA with the brand kit (RF-053); until then the `fonts` map of a document stays empty.
4. Bitmaps are cached by a hash of content, style, size and rasterizer version, and are served to the browser as images. Both executors draw the same bitmap (ADR-0003).
5. Per-word highlight in captions is produced as bitmaps per state, positioned by the frame plan; the plan, not the rasterizer, decides which is visible at each frame.
6. Simple shapes (`rect`, `rounded_rect`, `ellipse`) are drawn by the same package.
7. Animation of a text layer (position, scale, rotation, opacity) is applied by the executors to the bitmap. The bitmap is rasterized at the largest scale the animation reaches, so scaling up never blurs up to an effective pixel scale of 4; beyond it the bitmap is upscaled and softens.

## Alternatives

- **FFmpeg `drawtext` or libass for export, browser text for preview.** Two layouts; the divergence the research documents.
- **Browser canvas text, captured and sent to the server.** Requires a browser to render, against D-001.
- **HarfBuzz and FreeType through cgo.** Reference quality, but cgo breaks vukem's no-cgo rule and static cross-compilation.

## Consequences

Positive: text is pixel-identical in preview and export by construction.

Positive: the same output on every machine, since no system font is involved.

Negative: text features are what our code implements: no arbitrary CSS, no font effects beyond fill, stroke, shadow and background box.

Negative: colour emoji and complex scripts depend on what `go-text/typesetting` supports, which was not verified. A Prototype task measures emoji, right-to-left text and rendering speed before the text contract is frozen; if the library falls short, the fallback is `tdewolff/canvas` or a narrower documented scope.

Negative: bundled fonts add to the binary size.
