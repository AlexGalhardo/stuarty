import { describe, expect, test } from "vitest";

import { reconcileUrl, switchLocale } from "./locale";

function deps() {
	const calls: string[] = [];
	return {
		calls,
		deps: {
			pathname: "/editor/prj_x",
			store: (locale: string) => {
				calls.push(`store ${locale}`);
			},
			assign: (url: string) => {
				calls.push(`assign ${url}`);
			},
		},
	};
}

describe("switchLocale", () => {
	test("keeps the choice in the browser, then reloads at the same playhead and item", () => {
		const { calls, deps: d } = deps();
		switchLocale("pt-BR", { frame: 375, item: "itm_a1b2c3d4e5" }, d);
		expect(calls).toEqual([
			"store pt-BR",
			"assign /editor/prj_x?t=375f&item=itm_a1b2c3d4e5&lang=pt-BR",
		]);
	});

	test("keeps only the language at frame 0 and with nothing selected", () => {
		const { calls, deps: d } = deps();
		switchLocale("en", { frame: 0, item: null }, d);
		expect(calls[1]).toBe("assign /editor/prj_x?lang=en");
	});
});

describe("reconcileUrl", () => {
	const at = { pathname: "/editor/prj_x", search: "?t=9f" };

	test("asks for the stored language when the page came back in another", () => {
		expect(reconcileUrl("en", "pt-BR", at)).toBe(
			"/editor/prj_x?t=9f&lang=pt-BR",
		);
	});

	test("does nothing with no stored choice or a page already in it", () => {
		expect(reconcileUrl("en", null, at)).toBeNull();
		expect(reconcileUrl("pt-BR", "pt-br", at)).toBeNull();
	});

	test("asks once: a language the server lacks cannot loop", () => {
		expect(reconcileUrl("en", "xx", { ...at, search: "?lang=xx" })).toBeNull();
	});
});
