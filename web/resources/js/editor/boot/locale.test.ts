import { describe, expect, test } from "vitest";

import { switchLocale } from "./locale";

function deps(ok: boolean) {
	const calls: string[] = [];
	return {
		calls,
		deps: {
			pathname: "/editor/prj_x",
			post: async (locale: string) => {
				calls.push(`post ${locale}`);
				return ok;
			},
			assign: (url: string) => {
				calls.push(`assign ${url}`);
			},
		},
	};
}

describe("switchLocale", () => {
	test("posts the choice, then reloads at the same playhead and item", async () => {
		const { calls, deps: d } = deps(true);
		const reloading = await switchLocale(
			"pt-BR",
			{ frame: 375, item: "itm_a1b2c3d4e5" },
			d,
		);
		expect(reloading).toBe(true);
		expect(calls).toEqual([
			"post pt-BR",
			"assign /editor/prj_x?t=375f&item=itm_a1b2c3d4e5",
		]);
	});

	test("keeps no query at frame 0 and with nothing selected", async () => {
		const { calls, deps: d } = deps(true);
		await switchLocale("en", { frame: 0, item: null }, d);
		expect(calls[1]).toBe("assign /editor/prj_x");
	});

	test("does not reload when the post failed", async () => {
		const { calls, deps: d } = deps(false);
		expect(await switchLocale("en", { frame: 9, item: null }, d)).toBe(false);
		expect(calls).toEqual(["post en"]);
	});
});
