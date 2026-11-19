import { describe, expect, it } from "vitest";

import { ApiError } from "../../editor/api/errors";
import type { Transport, TransportRequest } from "../../editor/api/transport";
import { libraryClient } from "./client";

function fake(
	status: number,
	body: unknown,
): { transport: Transport; sent: TransportRequest[] } {
	const sent: TransportRequest[] = [];
	return {
		sent,
		transport: {
			send: async (request) => {
				sent.push(request);
				return { status, body };
			},
			openStream: () => ({ close: () => {} }),
		},
	};
}

describe("libraryClient", () => {
	it("lists with what a person may see and searches inside one kind", async () => {
		const { transport, sent } = fake(200, { entries: [], hits: [] });
		const client = libraryClient(transport);
		await client.list("skill");
		await client.search("prompt", "an intro & more");
		expect(sent[0]).toMatchObject({
			method: "GET",
			path: "/library/skills?include_unreviewed=true",
		});
		expect(sent[1].path).toBe(
			"/library/search?query=an%20intro%20%26%20more&kinds=prompt&include_unreviewed=true",
		);
	});

	it("writes with the verbs and paths of the API and escapes a name", async () => {
		const { transport, sent } = fake(200, {});
		const client = libraryClient(transport);
		await client.writeMemory({ description: "d" });
		await client.removeMemory("a b");
		await client.writeSkill("hook-writer", {
			description: "d",
			body: "b",
			files: null,
		});
		await client.reviewSkill("hook-writer", "sha256:abc");
		await client.writePrompt("intro", { description: "d", body: "b" });
		await client.renderPrompt("intro", { topic: "cats" });
		await client.exportSkills([]);
		await client.reindex();
		expect(sent.map((r) => `${r.method} ${r.path}`)).toEqual([
			"POST /library/memory",
			"DELETE /library/memory/a%20b",
			"PUT /library/skills/hook-writer",
			"POST /library/skills/hook-writer/review",
			"PUT /library/prompts/intro",
			"POST /library/prompts/intro/render",
			"POST /library/skills/export",
			"POST /library/reindex",
		]);
		expect(sent[2].body).toMatchObject({ files: null });
		// the mark carries the hash of the text the person read
		expect(sent[3].body).toEqual({ content_hash: "sha256:abc" });
		expect(sent[5].body).toEqual({ values: { topic: "cats" } });
	});

	it("turns an error answer into an ApiError that keeps the code and the hint", async () => {
		const { transport } = fake(403, {
			code: "E_FORBIDDEN",
			message: "only a person at the browser marks a skill as read.",
			hint: "open the skill",
			retryable: false,
		});
		await expect(
			libraryClient(transport).reviewSkill("a", "sha256:abc"),
		).rejects.toMatchObject({ code: "E_FORBIDDEN" });
		await expect(
			libraryClient(transport).reviewSkill("a", "sha256:abc"),
		).rejects.toBeInstanceOf(ApiError);
	});
});
