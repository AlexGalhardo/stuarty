import { readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { type APIRequestContext, test as base } from "@playwright/test";

// The workspace the server of playwright.config.ts runs on.
export const home =
	process.env.VUKEM_VIDEO_E2E_HOME ?? join(tmpdir(), "vukem-video-e2e-home");

export interface ProjectSummary {
	id: string;
	name: string;
	width: number;
	height: number;
}

// The local API is open to whoever reads the token file of the workspace
// (SDD-007 section 3), the same way the CLI and the MCP server do it.
async function token(): Promise<string> {
	return (await readFile(join(home, "token"), "utf8")).trim();
}

// listProjects reads the projects through the HTTP API, not through the page
// under test, so a spec checks what the core stored.
export async function listProjects(
	request: APIRequestContext,
): Promise<ProjectSummary[]> {
	const response = await request.get("/api/v1/projects", {
		headers: { Authorization: `Bearer ${await token()}` },
	});
	if (!response.ok()) throw new Error(`GET /projects: ${response.status()}`);
	const body = (await response.json()) as { projects: ProjectSummary[] };
	return body.projects;
}

// The editor authenticates a browser with a launch code redeemed at
// /auth/launch, which sets an HttpOnly session cookie (SDD-007 section 3.4).
// Every page of a spec starts logged in, the way `vukem-video open` does it.
export const test = base.extend({
	page: async ({ page, request }, use) => {
		const launch = await request.post("/api/v1/system/launch", {
			headers: { Authorization: `Bearer ${await token()}` },
		});
		if (!launch.ok())
			throw new Error(`POST /system/launch: ${launch.status()}`);
		const { url } = (await launch.json()) as { url: string };
		await page.goto(url);
		await use(page);
	},
});

export { expect } from "@playwright/test";

// api calls the HTTP API with the token, as the CLI does.
export async function api(
	request: APIRequestContext,
	method: "GET" | "POST",
	path: string,
	data?: unknown,
): Promise<{ status: number; body: unknown }> {
	const response = await request.fetch(`/api/v1${path}`, {
		method,
		data,
		headers: { Authorization: `Bearer ${await token()}` },
	});
	return {
		status: response.status(),
		body: await response.json().catch(() => null),
	};
}
