import { afterEach, describe, expect, it, vi } from "vitest";

import { openComposition } from "./openComposition";

describe("openComposition", () => {
	afterEach(() => vi.unstubAllGlobals());

	it("opens the timeline it uses with the way back in the address", () => {
		const assign = vi.fn();
		vi.stubGlobal("window", { location: { assign } });
		openComposition("prj_a000000001", "prj_b000000001");
		expect(assign).toHaveBeenCalledWith(
			"/editor/prj_b000000001?from=prj_a000000001",
		);
	});
});
