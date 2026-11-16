import { afterEach, describe, expect, test, vi } from "vitest";

const reload = vi.fn();
vi.mock("@inertiajs/react", () => ({
	http: { onRequest: vi.fn() },
	router: { reload: () => reload() },
}));

import { reconcileLang } from "./lang";

afterEach(() => {
	vi.useRealTimers();
	vi.unstubAllGlobals();
	reload.mockReset();
});

function browser(rendered: string, stored: string | null): void {
	vi.stubGlobal("document", { documentElement: { lang: rendered } });
	vi.stubGlobal("localStorage", { getItem: () => stored });
}

describe("reconcileLang", () => {
	test("reloads once when the stored language differs from the rendered one", () => {
		browser("en", "pt-BR");
		reconcileLang();
		expect(reload).toHaveBeenCalledTimes(1);
	});

	test("does nothing when they agree or nothing is stored", () => {
		browser("pt-BR", "pt-br");
		reconcileLang();
		browser("en", null);
		reconcileLang();
		expect(reload).not.toHaveBeenCalled();
	});

	test("tries again while Inertia has no page yet (Firefox and WebKit)", () => {
		vi.useFakeTimers();
		browser("en", "pt-BR");
		reload
			.mockImplementationOnce(() => {
				throw new TypeError("no page");
			})
			.mockImplementationOnce(() => {
				throw new TypeError("no page");
			});
		reconcileLang();
		vi.advanceTimersByTime(200);
		expect(reload).toHaveBeenCalledTimes(3);
	});
});
