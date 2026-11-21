import { describe, expect, test } from "vitest";

import { createClient } from "./client";
import { ApiError } from "./errors";
import type {
	Transport,
	TransportRequest,
	TransportResponse,
} from "./transport";
import { peaksUrl, stripUrl } from "./urls";

// A transport that records the requests and answers with a fixed response, so
// each function is checked for its URL, method, timeout and error mapping.
function fake(response: TransportResponse): {
	transport: Transport;
	requests: TransportRequest[];
} {
	const requests: TransportRequest[] = [];
	const transport = {
		send: async (request: TransportRequest) => {
			requests.push(request);
			return response;
		},
	} as unknown as Transport;
	return { transport, requests };
}

const ok = (body: unknown): TransportResponse => ({ status: 200, body });

describe("ALPHA reads and media state", () => {
	test("getCommands and getCommand", async () => {
		const { transport, requests } = fake(ok({ commands: [], text: "" }));
		const client = createClient(transport);
		await client.getCommands();
		await client.getCommand("item.move");
		expect(requests.map((r) => [r.method, r.path, r.timeoutMs])).toEqual([
			["GET", "/commands", 10_000],
			["GET", "/commands/item.move", 10_000],
		]);
	});

	test("getHistory sends the page and the actor, and nothing when there are none", async () => {
		const { transport, requests } = fake(ok({ entries: [] }));
		const client = createClient(transport);
		await client.getHistory("prj_a");
		await client.getHistory("prj_a", {
			actor: "local:cli",
			cursor: "c1",
			limit: 50,
		});
		expect(requests.map((r) => r.path)).toEqual([
			"/projects/prj_a/history",
			"/projects/prj_a/history?actor=local%3Acli&cursor=c1&limit=50",
		]);
		expect(requests.every((r) => r.method === "GET")).toBe(true);
	});

	test("inspectMedia joins the sections", async () => {
		const { transport, requests } = fake(ok({ asset_id: "ast_a" }));
		const client = createClient(transport);
		await client.inspectMedia("prj_a", "ast_a");
		await client.inspectMedia("prj_a", "ast_a", {
			sections: ["probe", "silences"],
			minSilence: "0.5s",
		});
		expect(requests.map((r) => r.path)).toEqual([
			"/projects/prj_a/media/ast_a",
			"/projects/prj_a/media/ast_a?sections=probe%2Csilences&min_silence=0.5s",
		]);
	});

	test("trashMedia and restoreMedia are POSTs with a 30 s timeout", async () => {
		const { transport, requests } = fake(ok({ asset_id: "ast_a", state: "x" }));
		const client = createClient(transport);
		await client.trashMedia("prj_a", "ast_a");
		await client.restoreMedia("prj_a", "ast_a");
		expect(requests.map((r) => [r.method, r.path, r.timeoutMs])).toEqual([
			["POST", "/projects/prj_a/media/ast_a/trash", 30_000],
			["POST", "/projects/prj_a/media/ast_a/restore", 30_000],
		]);
	});

	test("an error envelope becomes an ApiError with its code", async () => {
		const { transport } = fake({
			status: 409,
			body: { code: "E_IN_USE", message: "in use", retryable: false },
		});
		const client = createClient(transport);
		await expect(client.trashMedia("prj_a", "ast_a")).rejects.toBeInstanceOf(
			ApiError,
		);
		await expect(client.trashMedia("prj_a", "ast_a")).rejects.toMatchObject({
			code: "E_IN_USE",
		});
	});

	test("useAsset names the timeline only when one is given (VV-321)", async () => {
		const { transport, requests } = fake(ok({ assets: [] }));
		const client = createClient(transport);
		await client.useAsset("prj_a", "sha256:x");
		await client.useAsset("prj_a", "sha256:x", "prj_b");
		// Strict: a timeline_id member that is present but undefined would stop scopedToTimeline from naming the
		// timeline of the editor, so it must be absent.
		expect(requests.map((r) => [r.method, r.path, r.body])).toStrictEqual([
			["POST", "/projects/prj_a/media", { files: [{ hash: "sha256:x" }] }],
			[
				"POST",
				"/projects/prj_a/media",
				{ files: [{ hash: "sha256:x" }], timeline_id: "prj_b" },
			],
		]);
	});

	test("cancelJob is a POST with a 30 s timeout", async () => {
		const { transport, requests } = fake(
			ok({ job_id: "job_a", state: "cancelled" }),
		);
		await createClient(transport).cancelJob("job_a");
		expect(requests.map((r) => [r.method, r.path, r.timeoutMs])).toEqual([
			["POST", "/jobs/job_a/cancel", 30_000],
		]);
	});
});

describe("ALPHA snap points, fonts, system and audit", () => {
	test("getSnapPoints sends rev and words only when given", async () => {
		const { transport, requests } = fake(ok({ revision: 3, points: [] }));
		const client = createClient(transport);
		await client.getSnapPoints("prj_a");
		await client.getSnapPoints("prj_a", { revision: 3, words: true });
		await client.getSnapPoints("prj_a", { words: false });
		expect(requests.map((r) => [r.method, r.path, r.timeoutMs])).toEqual([
			["GET", "/projects/prj_a/snap-points", 10_000],
			["GET", "/projects/prj_a/snap-points?rev=3&words=1", 10_000],
			["GET", "/projects/prj_a/snap-points?words=0", 10_000],
		]);
	});

	test("getFonts, getSystem and listAudit", async () => {
		const { transport, requests } = fake(ok({}));
		const client = createClient(transport);
		await client.getFonts();
		await client.listAssets("image");
		await client.getSystem();
		await client.getSystem({ full: true, verify: true });
		await client.listAudit({ projectId: "prj_a", cursor: "c1", limit: 20 });
		expect(requests.map((r) => [r.method, r.path, r.timeoutMs])).toEqual([
			["GET", "/fonts", 10_000],
			["GET", "/assets?kind=image", 10_000],
			["GET", "/system", 10_000],
			["GET", "/system?full=1&verify=1", 10_000],
			["GET", "/audit?project_id=prj_a&cursor=c1&limit=20", 10_000],
		]);
	});

	test("a failure maps to ApiError", async () => {
		const { transport } = fake({
			status: 403,
			body: { code: "E_FORBIDDEN", message: "no", retryable: false },
		});
		await expect(createClient(transport).listAudit()).rejects.toMatchObject({
			code: "E_FORBIDDEN",
		});
	});
});

describe("ALPHA tools and keys", () => {
	test("tools and keys use the documented routes, methods and timeouts", async () => {
		const { transport, requests } = fake(ok({}));
		const client = createClient(transport);
		await client.getTools();
		await client.installTool({ name: "whisper", model: "base" });
		await client.createKey({ name: "ci", scopes: ["read"] });
		await client.listKeys();
		await client.listUsers();
		await client.revokeKey("key_a/b");
		expect(
			requests.map((r) => [r.method, r.path, r.timeoutMs, r.body]),
		).toEqual([
			["GET", "/system/tools", 10_000, undefined],
			[
				"POST",
				"/system/tools/install",
				30_000,
				{ name: "whisper", model: "base" },
			],
			["POST", "/keys", 30_000, { name: "ci", scopes: ["read"] }],
			["GET", "/keys", 10_000, undefined],
			["GET", "/users", 10_000, undefined],
			["DELETE", "/keys/key_a%2Fb", 30_000, undefined],
		]);
	});

	test("revert, the diff and checkpoints use the documented routes", async () => {
		const { transport, requests } = fake(ok({}));
		const client = createClient(transport);
		await client.getCheckpoints("prj_a");
		await client.createCheckpoint("prj_a", { name: "two texts" });
		await client.removeCheckpoint("prj_a", "two/texts");
		await client.getHistoryDiff("prj_a", { from: 2, until: 4 });
		await client.getHistoryDiff("prj_a", { from: 2 });
		await client.revertTo("prj_a", { checkpoint: "two texts", dry_run: true });
		expect(
			requests.map((r) => [r.method, r.path, r.timeoutMs, r.body]),
		).toEqual([
			["GET", "/projects/prj_a/checkpoints", 10_000, undefined],
			["POST", "/projects/prj_a/checkpoints", 30_000, { name: "two texts" }],
			["DELETE", "/projects/prj_a/checkpoints/two%2Ftexts", 30_000, undefined],
			["GET", "/projects/prj_a/diff?from=2&until=4", 10_000, undefined],
			["GET", "/projects/prj_a/diff?from=2", 10_000, undefined],
			[
				"POST",
				"/projects/prj_a/revert",
				30_000,
				{ checkpoint: "two texts", dry_run: true },
			],
		]);
	});

	test("a refused key call maps to ApiError", async () => {
		const { transport } = fake({
			status: 404,
			body: { code: "E_NOT_FOUND", message: "no", retryable: false },
		});
		await expect(createClient(transport).listKeys()).rejects.toMatchObject({
			code: "E_NOT_FOUND",
		});
	});
});

describe("binary URL builders", () => {
	test("strip and peaks sit on the media prefix", () => {
		expect(stripUrl("prj_a", "ast_a")).toBe(
			"/api/v1/projects/prj_a/media/ast_a/strip",
		);
		expect(peaksUrl("prj_a", "ast_a")).toBe(
			"/api/v1/projects/prj_a/media/ast_a/peaks",
		);
	});
});

describe("brand kit, fonts, export options and bundles", () => {
	test("brand kit and font import", async () => {
		const { transport, requests } = fake(ok({}));
		const client = createClient(transport);
		await client.getBrandKit();
		await client.setBrandKit({ colors: { accent: "#FF3366" } });
		await client.importFont("C:/fonts/Brand.ttf");
		expect(
			requests.map((r) => [r.method, r.path, r.timeoutMs, r.body]),
		).toEqual([
			["GET", "/brand-kit", 10_000, undefined],
			["PUT", "/brand-kit", 30_000, { colors: { accent: "#FF3366" } }],
			["POST", "/fonts", 30_000, { path: "C:/fonts/Brand.ttf" }],
		]);
	});

	test("startExport sends args only for the options that are set", async () => {
		const { transport, requests } = fake(ok({}));
		const client = createClient(transport);
		await client.startExport("prj_x");
		await client.startExport("prj_x", { format: "gif" });
		await client.startExport("prj_x", { format: "wav" }, true);
		await client.startExport("prj_x", { format: undefined });
		expect(requests.map((r) => r.body)).toEqual([
			{ kind: "export" },
			{ kind: "export", args: { format: "gif" } },
			{ kind: "export", args: { format: "wav" }, dry_run: true },
			{ kind: "export" },
		]);
	});

	test("bundle export and import", async () => {
		const { transport, requests } = fake(ok({}));
		const client = createClient(transport);
		await client.exportBundle("prj_x", false);
		await client.importBundle("C:/b.vvbundle", true);
		expect(requests.map((r) => [r.method, r.path, r.body])).toEqual([
			[
				"POST",
				"/projects/prj_x/bundle",
				{ wait_ms: 20_000, include_log: false },
			],
			["POST", "/projects/import", { path: "C:/b.vvbundle", as_new: true }],
		]);
	});
});

describe("createProject and updateProject", () => {
	const goal = {
		specific: "Publish one short video",
		measurable: "A hundred views",
		achievable: "The footage exists",
		relevant: "It brings an audience",
		time_bound: "Publish on Tuesday",
		due: "2999-12-31",
	};

	test("posts the name, the description and the goal to the JSON API", async () => {
		const { transport, requests } = fake(ok({ project_id: "prj_a" }));
		const input = { name: "Launch teaser", description: "d".repeat(64), goal };
		const result = await createClient(transport).createProject(input);
		expect(result.project_id).toBe("prj_a");
		expect(requests[0]).toMatchObject({
			method: "POST",
			path: "/projects",
			body: input,
		});
	});

	test("patches only the members that change", async () => {
		const { transport, requests } = fake(ok({ project_id: "prj_a" }));
		await createClient(transport).updateProject("prj_a", { name: "New name" });
		expect(requests[0]).toMatchObject({
			method: "PATCH",
			path: "/projects/prj_a",
			body: { name: "New name" },
		});
	});
});
