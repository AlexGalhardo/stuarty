# ADR-0012: Text and shapes are rasterized by Go code with bundled fonts

- Status: Accepted (2026-11-14); library choice confirmed by the Prototype measurement below (P1-RND-003, 2026-11-14)
- Deciders: author (D-001, D-014, D-022), architect
- Links: post-mortems §1, §6, stack trade-offs §3, ADR-0003, SDD-004

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

## Measurement (P1-RND-003, 2026-11-14)

`go-text/typesetting` v0.3.5 with `golang.org/x/image/vector` is kept. The fallback (`tdewolff/canvas`) is not needed, and the text contract is frozen with the scope below. The code is `engine/raster`; the tests named here are in it.

**Speed** (Ryzen 7 5700X3D, Windows 11, Go 1.27.4, `go test -bench Caption`). The caption of the reference scene (six words on two lines, Inter 700 at 64 px, stroke 5, shadow blur 6, one highlighted word with its box, 1080 wide frame): 3.7 ms on a cache miss, 4.9 ms with one core, 10.8 ms at oversampling 2; 6 µs on a cache hit. The budgets are 5 ms and 0.1 ms. Shaping and layout are a small part of the miss; the time goes to filling the glyph outlines, the stroke and the shadow blur. The Linux and macOS numbers are recorded in SDD-004 section 12 after the first CI run.

**Emoji.** The shaping library draws colour glyphs when a font has them, but none of the six bundled families has an emoji, and no free colour emoji font small enough to bundle was approved. An emoji is therefore drawn with the `.notdef` glyph of the font (Inter draws a box that reads "NO GLYPH"), the rest of the line is laid out normally, and a sequence (a ZWJ family, a skin tone) is drawn as one box per code point because the font has no ligature for it. `TestEmojiDrawsTheNotdefGlyph`, the golden case `emoji-sequence-and-notdef`. **Not supported in the Prototype**; the lint reports characters the font lacks (SDD-004 section 5.1 item 9).

**Right-to-left.** The bidirectional algorithm and the visual reordering of runs work: Hebrew inside English text is placed right to left between the English words, and a paragraph that starts right to left starts on the right (`TestRightToLeftRunsAreReordered`, `TestRightToLeftParagraphStartsOnTheRight`, the golden case `right-to-left-mixed`). The shaping of Arabic (joining forms) and Hebrew could **not** be verified with real letters, because none of the six families has Arabic or Hebrew glyphs: every letter is `.notdef` (`TestRightToLeftScriptsAreOutsideTheBundledFonts` fails the day a family gains them). The same holds for Devanagari, Thai and CJK. **Not supported in the Prototype**: they need a font, which is user fonts (BETA, RF-053) or a new bundled family approved by the author.

**Supported scope of the text contract.** Latin and Latin Extended (accents, Vietnamese) in all six families, plus Cyrillic in Inter, Montserrat and Playfair Display, plus Greek in Inter; digits, punctuation and common symbols as each font has them. Everything else is `.notdef`.

**Size.** The six files add 2.3 MB to the binary (Inter 877 KB, Montserrat 745 KB, Playfair Display 301 KB, JetBrains Mono 187 KB, Anton 171 KB, Bebas Neue 61 KB).

**Determinism.** The golden bitmaps are compared byte for byte on the three operating systems in CI. The first macOS arm64 run showed that the filling step of `golang.org/x/image/vector` is not bit-identical between amd64 (SSE accumulation) and arm64 (Go loop). The fallback this ADR names, "our own rasterization path", was applied in its smallest form: `engine/raster/internal/scan` is a copy of the portable path of that module, used everywhere. The geometry avoids the standard library's `Sin`, `Cos`, `Atan2`, `Hypot` and `Acos`, and rounds outline points to a whole font unit so that the last bit of the shaping library's float32 variable-font arithmetic cannot show. Details in SDD-004 section 5.1.
