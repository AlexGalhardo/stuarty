import digest from "../../../../../../engine/renderhost/renderer.wasm.sha256?raw";

// The core module of the renderer, the file the Go host embeds. The build
// copies it to a content-hashed same-origin asset; the worker checks its
// SHA-256 against the digest `scripts/build-renderer.sh embed` pinned.
export const RENDERER_MODULE_URL = new URL(
	"../../../../../../engine/renderhost/renderer.wasm",
	import.meta.url,
).href;

export const RENDERER_DIGEST = digest.trim();
