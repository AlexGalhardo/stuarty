import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import videoInspector from "../editor/inspector/Inspector.tsx?raw";
import PanelTitle from "./PanelTitle";

// A sticky title sticks at the padding edge of the panel that scrolls, so a title that takes back the 12 px of
// padding with -mt-3 sticks at -top-3: top-0 left a band of 12 px of scrolled content above it (VV-402).
describe("panel titles of the editors", () => {
	test("the shared title sticks at -top-3", () => {
		const html = renderToString(<PanelTitle>Layers</PanelTitle>);
		expect(html).toContain("sticky -top-3");
		expect(html).toContain("-mt-3");
	});

	test("the title of the video inspector sticks like the shared one", () => {
		expect(videoInspector).toContain("sticky -top-3 z-10 -mx-3 -mt-3");
		expect(videoInspector).not.toContain("sticky top-0");
	});
});
