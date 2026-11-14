import { readFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { APIRequestContext } from "@playwright/test";

// Why the specs of the pages are skipped: the controller of /projects calls
// service.Service, which is not built yet.
export const PENDING_SERVER =
	"waiting for the /projects controller and the HTTP API (P1-SRV-003, P1-SRV-005)";

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
