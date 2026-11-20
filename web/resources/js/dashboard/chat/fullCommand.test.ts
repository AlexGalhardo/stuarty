import { describe, expect, it } from "vitest";

import { fullCommand, parseArgs } from "./fullCommand";

describe("fullCommand", () => {
	it("joins the program and the arguments", () => {
		expect(fullCommand("/usr/bin/opencode", ["acp"])).toBe(
			"/usr/bin/opencode acp",
		);
	});

	it("quotes what holds a blank or a quote, so nothing hides in a space", () => {
		expect(
			fullCommand("C:\\Program Files\\Agent\\agent.exe", ["--flag", "a b"]),
		).toBe('"C:\\\\Program Files\\\\Agent\\\\agent.exe" --flag "a b"');
		expect(fullCommand("agent", ['say "hi"'])).toBe('agent "say \\"hi\\""');
		expect(fullCommand("agent", [""])).toBe('agent ""');
	});
});

describe("parseArgs", () => {
	it("is one argument per line, blank lines left out", () => {
		expect(parseArgs("acp\n\n  --experimental \r\nx y\n")).toEqual([
			"acp",
			"  --experimental",
			"x y",
		]);
		expect(parseArgs("")).toEqual([]);
	});
});
