import { describe, expect, test } from "vitest";

import fullLayout from "./FullLayout.tsx?raw";
import { EDITOR_FRAME } from "./ImageEditor";

// The image editor takes the whole window, as the final design draws it and as the video editor does (SDD-012
// section 24): the frame is FullLayout, with no dashboard sidebar or top bar, and the editor fills what the
// scene bar at the foot leaves. If that changes, the page scrolls and the scene bar covers the foot.
describe("image editor frame", () => {
	test("is the window, with the editor filling the column above the scene bar", () => {
		expect(fullLayout).toContain("h-dvh");
		expect(fullLayout).toContain("<ConnectedSceneBar />");
		expect(EDITOR_FRAME).toContain("flex-1");
		expect(EDITOR_FRAME).toContain("min-h-0");
	});

	test("keeps a floor so a short window scrolls the page instead of crushing the canvas", () => {
		expect(fullLayout).toContain("min-h-[34rem]");
	});
});
