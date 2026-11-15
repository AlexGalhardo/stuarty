import { expect, test } from "vitest";

import { safeNext } from "./safeNext";

test("a path of this site is kept, anything else goes to the projects", () => {
	expect(safeNext("/editor/prj_abc")).toBe("/editor/prj_abc");
	expect(safeNext("/editor/prj_abc?t=45#x")).toBe("/editor/prj_abc?t=45#x");
	expect(safeNext(null)).toBe("/projects");
	expect(safeNext("https://example.com/x")).toBe("/projects");
	expect(safeNext("//example.com")).toBe("/projects");
	expect(safeNext("/\\example.com")).toBe("/projects");
	expect(safeNext("editor")).toBe("/projects");
});

test("parser tricks that resolve to another origin are refused", () => {
	expect(safeNext("/\t/example.com")).toBe("/projects");
	expect(safeNext("/\n/example.com")).toBe("/projects");
	expect(safeNext("/%5Cexample.com")).toBe("/%5Cexample.com");
	expect(safeNext("/\u0000//example.com")).toBe("/projects");
});
