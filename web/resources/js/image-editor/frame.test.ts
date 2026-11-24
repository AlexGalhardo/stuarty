import { describe, expect, test } from "vitest";

import layout from "../layouts/DashboardLayout.tsx?raw";
import sceneBar from "../shell/SceneBar.tsx?raw";
import topBar from "../shell/TopBar.tsx?raw";
import { EDITOR_FRAME } from "./ImageEditor";

// The editor is sized to what the dashboard frame leaves of the window (SDD-012 section 24): if the top bar,
// the scene bar or the padding of the main region changes height, the sum below must change with it, or the
// page scrolls and the scene bar covers the foot of the editor.
describe("image editor frame", () => {
	test("leaves room for the top bar, the scene bar and the padding, from the md breakpoint", () => {
		expect(topBar).toContain("sticky top-0");
		expect(topBar).toMatch(/\bh-14\b/);
		expect(sceneBar).toContain("md:h-14");
		expect(layout).toContain("md:p-6");
		// 3.5 rem + 3.5 rem + 2 x 1.5 rem
		expect(EDITOR_FRAME).toContain("md:h-[calc(100dvh-10rem)]");
		// 3.5 rem + 3.5 rem + 2 x 1 rem, under md
		expect(EDITOR_FRAME).toContain("h-[calc(100dvh-9rem)]");
		expect(layout).toContain("p-4");
	});

	test("keeps a floor so a short window scrolls the page instead of crushing the canvas", () => {
		expect(EDITOR_FRAME).toContain("min-h-[28rem]");
	});
});
