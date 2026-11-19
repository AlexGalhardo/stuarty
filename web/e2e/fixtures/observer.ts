import { execFileSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { home } from "../specs/helpers";

// The fixtures of the observer test of P1-QA-001 (NFR-002): the real CLI
// binary that drives the project, and the statistics of the delays.

const REPO = resolve(import.meta.dirname, "..", "..", "..");

// The same key the server of playwright.config.ts runs with, so the CLI and the
// server read one workspace configuration.
const APP_KEY = `base64:${Buffer.alloc(32, "e2e").toString("base64")}`;

let built: string | undefined;

// buildCli compiles the binary once per run.
export function buildCli(): string {
	if (built !== undefined) return built;
	const dir = join(tmpdir(), "stuarty-e2e-cli");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, process.platform === "win32" ? "app.exe" : "app");
	execFileSync(
		"go",
		["build", "-o", out, "github.com/AlexGalhardo/stuarty/web/cmd/app"],
		{ cwd: REPO, stdio: "ignore" },
	);
	built = out;
	return out;
}

// cliMove moves an item with `edit` of the CLI, as an agent in a terminal
// does, and returns when the command has exited.
export function cliMove(
	bin: string,
	projectId: string,
	itemId: string,
	startFrame: number,
): void {
	execFileSync(
		bin,
		[
			"edit",
			"--project",
			projectId,
			"--workspace",
			home,
			"item.move",
			`item_id=${itemId}`,
			`start=${startFrame}f`,
		],
		{
			env: {
				...process.env,
				APP_KEY,
				STUARTY_MIN_FREE_BYTES: "1",
				STUARTY_HOME: home,
			},
			stdio: "ignore",
		},
	);
}

// percentile of a sample, the nearest-rank method.
export function percentile(values: number[], p: number): number {
	const sorted = [...values].sort((a, b) => a - b);
	return sorted[Math.max(0, Math.ceil(p * sorted.length) - 1)] as number;
}
