import { expect, test } from "vitest";

import { safeNext } from "./safeNext";

test("a path of this site is kept, anything else goes to the projects", () => {
	expect(safeNext("/editor/prj_abc")).toBe("/editor/prj_abc");
	expect(safeNext(null)).toBe("/projects");
	expect(safeNext("https://example.com/x")).toBe("/projects");
	expect(safeNext("//example.com")).toBe("/projects");
	expect(safeNext("/example.com")).toBe("/projects");
	expect(safeNext("editor")).toBe("/projects");
});
