import { describe, expect, test } from "vitest";

import {
	parseChoice,
	parseSidebar,
	sidebarForm,
	toggled,
} from "./sidebarState";

describe("sidebar state", () => {
	test("anything but collapsed reads as expanded", () => {
		expect(parseSidebar("collapsed")).toBe("collapsed");
		expect(parseSidebar("expanded")).toBe("expanded");
		expect(parseSidebar(null)).toBe("expanded");
		expect(parseSidebar("wide")).toBe("expanded");
	});

	test("only a stored choice is a choice", () => {
		expect(parseChoice("collapsed")).toBe("collapsed");
		expect(parseChoice("expanded")).toBe("expanded");
		expect(parseChoice(null)).toBeNull();
		expect(parseChoice("wide")).toBeNull();
	});

	test("toggled flips the state", () => {
		expect(toggled("expanded")).toBe("collapsed");
		expect(toggled("collapsed")).toBe("expanded");
	});

	test("a phone has the drawer whatever was chosen", () => {
		expect(sidebarForm("expanded", true, true)).toBe("drawer");
		expect(sidebarForm(null, true, true)).toBe("drawer");
	});

	test("with no choice, up to 1280 px is the rail and above it the full form", () => {
		expect(sidebarForm(null, false, true)).toBe("rail");
		expect(sidebarForm(null, false, false)).toBe("full");
	});

	test("a stored choice wins where there is a rail", () => {
		expect(sidebarForm("expanded", false, true)).toBe("full");
		expect(sidebarForm("collapsed", false, false)).toBe("rail");
	});
});
