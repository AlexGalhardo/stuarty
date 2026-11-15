// The test hooks of SDD-012 section 20.4 must not reach a user: the one that
// changes the project (window.__vv.dispatch) is written inside a VITE_E2E
// branch, and a production bundle must not contain it.
//
//   bun run check:hooks     builds the end-to-end bundle and the production
//                           bundle and greps both
//
// The marker is a string literal inside that branch (editor/boot/debugHooks.ts).
// The Vite plugin of the project fixes the output folder, so the builds run in
// it, the production one last: the folder is left as `bun run build` leaves it.

import { readdirSync, readFileSync, statSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const MARKER = "vv-e2e-dispatch";
const webRoot = join(dirname(fileURLToPath(import.meta.url)), "..");
const outDir = join(webRoot, "resources", "public", "build");

function bundleText(dir: string): string {
	let text = "";
	for (const name of readdirSync(dir)) {
		const path = join(dir, name);
		if (statSync(path).isDirectory()) text += bundleText(path);
		else if (name.endsWith(".js")) text += readFileSync(path, "utf8");
	}
	return text;
}

function build(mode: "production" | "e2e"): void {
	const args = ["vite", "build"];
	if (mode === "e2e") args.push("--mode", "e2e");
	const result = Bun.spawnSync(["bunx", ...args], { cwd: webRoot, stdout: "ignore", stderr: "pipe" });
	if (result.exitCode !== 0) {
		console.error(result.stderr.toString());
		throw new Error(`vite build (${mode}) failed`);
	}
}

let failed = false;
for (const [mode, expected] of [
	["e2e", true],
	["production", false],
] as const) {
	build(mode);
	const found = bundleText(outDir).includes(MARKER);
	const ok = found === expected;
	console.log(`check-hooks: ${mode} bundle ${found ? "has" : "has no"} ${MARKER}: ${ok ? "ok" : "WRONG"}`);
	if (!ok) failed = true;
}
if (failed) process.exit(1);
