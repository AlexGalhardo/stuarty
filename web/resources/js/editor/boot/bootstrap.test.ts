import { describe, expect, test } from "vitest";

import { parseBootstrap } from "./bootstrap";

const base = {
	project_id: "prj_abcdefghjk",
	mode: "local",
	api_base: "/api/v1",
	locale: "en",
	messages: {},
};

describe("parseBootstrap session", () => {
	test("keeps what the server says about the session", () => {
		const parsed = parseBootstrap(
			JSON.stringify({ ...base, session: { privileged: true } }),
		);
		expect(parsed.session).toEqual({ privileged: true });
	});

	test("has no session when the server did not send one, so no scene bar is drawn", () => {
		expect(parseBootstrap(JSON.stringify(base)).session).toBeUndefined();
		expect(
			parseBootstrap(
				JSON.stringify({ ...base, session: { privileged: "yes" } }),
			).session,
		).toBeUndefined();
	});
});
