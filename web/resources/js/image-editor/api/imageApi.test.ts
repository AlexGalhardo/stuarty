import { describe, expect, test } from "vitest";

import type { Transport } from "../../editor/api/transport";
import { createImageApi } from "./imageApi";

// A transport that records the requests and answers 200 with an empty object.
function recording(): {
	transport: Transport;
	sent: { method: string; path: string; body?: unknown }[];
} {
	const sent: { method: string; path: string; body?: unknown }[] = [];
	const transport = {
		send: async (request: { method: string; path: string; body?: unknown }) => {
			sent.push({
				method: request.method,
				path: request.path,
				body: request.body,
			});
			return { status: 200, body: {} };
		},
	} as unknown as Transport;
	return { transport, sent };
}

// Epic 50: the calls of the history panel go to the routes of SDD-003 section 6.3 for an image.
describe("the revert, the diff and the named versions of an image", () => {
	test("each call is the route of the API, with the name encoded", async () => {
		const { transport, sent } = recording();
		const api = createImageApi(transport);

		await api.checkpoints("img_aaaaaaaaaa");
		await api.createCheckpoint("img_aaaaaaaaaa", "before the agent");
		await api.removeCheckpoint("img_aaaaaaaaaa", "a b");
		await api.diff("img_aaaaaaaaaa", 3);
		await api.diff("img_aaaaaaaaaa", 3, 5);
		await api.revert("img_aaaaaaaaaa", { revision: 2, dry_run: true });
		await api.revert("img_aaaaaaaaaa", { checkpoint: "x", base_revision: 7 });

		expect(sent).toEqual([
			{ method: "GET", path: "/images/img_aaaaaaaaaa/checkpoints" },
			{
				method: "POST",
				path: "/images/img_aaaaaaaaaa/checkpoints",
				body: { name: "before the agent" },
			},
			{ method: "DELETE", path: "/images/img_aaaaaaaaaa/checkpoints/a%20b" },
			{ method: "GET", path: "/images/img_aaaaaaaaaa/diff?from=3" },
			{ method: "GET", path: "/images/img_aaaaaaaaaa/diff?from=3&until=5" },
			{
				method: "POST",
				path: "/images/img_aaaaaaaaaa/revert",
				body: { revision: 2, dry_run: true },
			},
			{
				method: "POST",
				path: "/images/img_aaaaaaaaaa/revert",
				body: { checkpoint: "x", base_revision: 7 },
			},
		]);
	});
});
