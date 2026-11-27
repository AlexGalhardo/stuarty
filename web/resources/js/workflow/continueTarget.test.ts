import { describe, expect, test } from "vitest";

import { continueTarget, isDocumentNavigation } from "./continueTarget";

describe("continueTarget", () => {
	test("a text step opens the text, a media step the editor and the others the project or the page", () => {
		expect(continueTarget("writing", "prj_aaaaaaaaaa", "itm_aaaaaaaaaa")).toBe(
			"/write?project=prj_aaaaaaaaaa&text=itm_aaaaaaaaaa",
		);
		expect(continueTarget("script", "prj_aaaaaaaaaa")).toBe(
			"/projects/prj_aaaaaaaaaa",
		);
		expect(continueTarget("editing", "prj_aaaaaaaaaa")).toBe(
			"/editor/prj_aaaaaaaaaa",
		);
		expect(continueTarget("approval", "prj_aaaaaaaaaa")).toBe("/approvals");
		expect(continueTarget("publication", "prj_aaaaaaaaaa")).toBe("/publish");
		expect(continueTarget("briefing", "prj_aaaaaaaaaa")).toBe(
			"/projects/prj_aaaaaaaaaa",
		);
	});

	test("marks video editor routes as full-document navigations", () => {
		expect(isDocumentNavigation("/editor/prj_aaaaaaaaaa")).toBe(true);
		expect(isDocumentNavigation("/projects/prj_aaaaaaaaaa")).toBe(false);
		expect(isDocumentNavigation("/write?project=prj_aaaaaaaaaa")).toBe(false);
	});
});
