# ADR-0038: Image editing is a second document type on the same core and the same renderer

- Status: Accepted (2026-11-18)
- Deciders: author
- Links: author decision D-062; ADR-0002, ADR-0003, ADR-0005, ADR-0012, ADR-0029, ADR-0031; SDD-013; PRD-004

## Context

Stuarty needs an image editor with the most used fifth of GIMP and Photoshop, mainly so that agents can finish images during a production: thumbnails, assets for a video, blog images, sales images.

The renderer of ADR-0029 already composes layers with blend modes, affine transforms, crop, opacity, colour adjustment, blur, filters, shape masks, shadows, outlines and shaped text, with a bit-exact CPU path and a GPU path. A layered image is one frame of that renderer. The research found no established API for layered, deterministic image editing by agents: the image tools agents have today call generative models, which redraw the whole picture (research note `web/01-engine-imagem`). GIMP and Krita are GPL and cannot be reused here.

## Decision

1. **An image is a document of non-destructive layers**: image, text, shape, adjustment and group layers, each with a transform, an opacity, a blend mode, a mask and effects. It is not a paint program.
2. **Its rules live in `core/`** as a second document type with its own operation catalogue, applied in atomic batches with undo, a lint and a text view, on the four surfaces (ADR-0031).
3. **The renderer of ADR-0029 draws it.** Core builds a one-frame plan; the same Rust module renders it in the browser and in the export. No second compositor and no browser canvas library decides a pixel.
4. **The first delivery is the minimal editor**: image, text and shape layers, templates with slots, and export to PNG, JPEG and WebP through the CPU path, enough to make a thumbnail. Later epics add adjustments, filters, more blend modes, masks and the UI tools.
5. **Operations that need a model** (remove a background, inpaint, upscale, expand) are provider calls (ADR-0036); their result enters the document as an ordinary image layer with its provenance.
6. **Out of scope:** painting and artistic brushes, CMYK and ICC profiles, 16 and 32 bits per channel, HDR, RAW, full smart objects, actions and scripts.

## Alternatives

- **A separate image engine written from scratch.** Rejected: it duplicates composition, text and the guarantee that the preview equals the exported file.
- **Embed an existing editor.** Rejected: an agent could not operate it, and the best known one is closed source.

## Consequences

Positive: one renderer and one set of parity gates for video and image; every new filter is written once.

Negative: finishing the renderer of ADR-0029 comes before the image editor grows past its minimal form.

Negative: the renderer needs new work (levels, curves, sharpen, alpha and clipping masks, gradients, image decoding and encoding) and possibly new crates, each needing the author's approval.
