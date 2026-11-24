import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, test } from "vitest";

const web = join(dirname(fileURLToPath(import.meta.url)), "..");
const css = readFileSync(join(web, "resources/css/app.css"), "utf8").replace(
	/\r\n/g,
	"\n",
);
const start = css.indexOf(":root,\n.dark {");
const aliases = css.slice(start, css.indexOf("\n}", start));

describe("aliases of the former tokens", () => {
	test.each([
		["accent", "accent"],
		["success", "success"],
		["warning", "warning"],
		["agent", "agent"],
		["primary", "accent"],
		["muted", "surface-sunken"],
		["card", "surface"],
		["background", "background"],
	])("--%s points to --color-%s in both themes", (former, token) => {
		expect(start).toBeGreaterThan(-1);
		expect(aliases).toContain(`--${former}: var(--color-${token});`);
	});
});
