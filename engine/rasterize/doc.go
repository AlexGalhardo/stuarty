// Package rasterize draws the bitmaps of text, captions and shapes with the Rust rasterizer
// (renderer/stuarty-renderer-text, SDD-004 section 5.1, SDD-013 section 3.11), which runs in the core
// module of the renderer under wazero (engine/renderhost). The rasterizer is the reference: the bytes
// it draws are the bytes every cache, golden and export holds. The package encodes a plan.BitmapSpec
// into the request of the module, keeps the bytes of the user fonts a request needs, and decodes the
// bitmap and the failures.
package rasterize
