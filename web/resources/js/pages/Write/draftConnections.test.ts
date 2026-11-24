import { describe, expect, test } from "vitest";

import type { Connection, ProviderView } from "../../editor/api/types";
import { aiTextConnections } from "./draftConnections";

function connection(id: string, kind: string, accepts: string[]): Connection {
	return {
		id,
		kind,
		label: id,
		endpoint: "https://example.test",
		hosts: [],
		accepts,
		created_at: "2026-11-24T00:00:00Z",
		last_used_at: null,
		secret_set: true,
	};
}

function provider(
	id: string,
	capabilities: string[],
	connections: string[],
): ProviderView {
	return {
		id,
		name: id,
		capabilities,
		credential_kinds: ["api_key"],
		default_endpoint: "",
		docs_url: "",
		connections,
	};
}

describe("the connections the draft offers", () => {
	test("a publishing connection that accepts text is not an AI connection", () => {
		const connections = [
			connection("cnx_ai", "openai", ["text"]),
			connection("cnx_site", "site", ["text"]),
		];
		const providers = [provider("openai", ["text"], ["cnx_ai"])];
		expect(aiTextConnections(providers, connections).map((c) => c.id)).toEqual([
			"cnx_ai",
		]);
	});

	test("a provider with no text capability, or a connection that does not accept text, is left out", () => {
		const connections = [
			connection("cnx_img", "images", ["text"]),
			connection("cnx_nottext", "openai", ["image"]),
			connection("cnx_ok", "openai", ["text", "image"]),
		];
		const providers = [
			provider("images", ["image"], ["cnx_img"]),
			provider("openai", ["text", "image"], ["cnx_nottext", "cnx_ok"]),
		];
		expect(aiTextConnections(providers, connections).map((c) => c.id)).toEqual([
			"cnx_ok",
		]);
	});

	test("no provider connection means an empty list", () => {
		expect(
			aiTextConnections([], [connection("cnx_site", "site", ["text"])]),
		).toEqual([]);
	});
});
