// The unit tests of this folder read the module the Go host embeds. The
// project has no Node typings, so the one function they use is declared here.
declare module "node:fs" {
	export function readFileSync(path: URL): Uint8Array<ArrayBuffer>;
	export function readFileSync(path: URL, encoding: "utf8"): string;
}
