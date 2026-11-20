import { afterEach, describe, expect, it, vi } from "vitest";

import { confirmDiscard, hasUnsaved, markUnsaved } from "./unsaved";

describe("unsaved text", () => {
	afterEach(() => {
		markUnsaved("a", false);
		vi.unstubAllGlobals();
	});

	it("asks nothing while nothing is unsaved", () => {
		const confirm = vi.fn(() => false);
		vi.stubGlobal("window", { confirm });
		expect(confirmDiscard("leave?")).toBe(true);
		expect(confirm).not.toHaveBeenCalled();
	});

	it("asks, and goes on only when the person agrees", () => {
		markUnsaved("a", true);
		expect(hasUnsaved()).toBe(true);
		vi.stubGlobal("window", { confirm: () => false });
		expect(confirmDiscard("leave?")).toBe(false);
		vi.stubGlobal("window", { confirm: () => true });
		expect(confirmDiscard("leave?")).toBe(true);
	});
});
