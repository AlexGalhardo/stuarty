import { describe, expect, test } from "vitest";

import { effectiveSidebar, parseSidebar, toggled } from "./sidebarState";

describe("sidebar state", () => {
	test("anything but collapsed reads as expanded", () => {
		expect(parseSidebar("collapsed")).toBe("collapsed");
		expect(parseSidebar("expanded")).toBe("expanded");
		expect(parseSidebar(null)).toBe("expanded");
		expect(parseSidebar("wide")).toBe("expanded");
	});

	test("toggled flips the state", () => {
		expect(toggled("expanded")).toBe("collapsed");
		expect(toggled("collapsed")).toBe("expanded");
	});

	test("a narrow window collapses it whatever was chosen", () => {
		expect(effectiveSidebar("expanded", true)).toBe("collapsed");
		expect(effectiveSidebar("expanded", false)).toBe("expanded");
		expect(effectiveSidebar("collapsed", false)).toBe("collapsed");
	});
});
