import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";

type Catalog = { [key: string]: string | Catalog };

function platformNames(file: string): Record<string, string> {
	const catalog = JSON.parse(
		readFileSync(join(process.cwd(), "resources", "lang", file), "utf8"),
	) as Catalog;
	const options = (
		(((catalog.ui as Catalog).projects as Catalog).briefing as Catalog)
			.option as Catalog
	).platform as Record<string, string>;
	return options;
}

// The platform presets of the core name a network and a placement ("YouTube Shorts"); the export dialog draws
// those labels as the core gives them. The briefing draws the same networks from the catalogue, so each
// catalogue must spell them as the core does, in every language: a brand name is not translated (VV-260).
function coreLabels(): Record<string, string> {
	const source = readFileSync(
		join(process.cwd(), "..", "core", "export", "preset.go"),
		"utf8",
	);
	const labels: Record<string, string> = {};
	for (const m of source.matchAll(/ID: "([a-z0-9-]+)",[^\n]*?Label: "([^"]+)"/g)) {
		labels[m[1]] = m[2];
	}
	return labels;
}

test("the briefing names the networks as the export presets do, in every language", () => {
	const labels = coreLabels();
	const ids = ["youtube-shorts", "instagram-reels", "facebook-reels", "tiktok"];
	for (const id of ids) expect(labels[id], `core label of ${id}`).toBeDefined();
	for (const file of ["en.json", "pt-BR.json"]) {
		const names = platformNames(file);
		for (const id of ids) expect(names[id], `${file} ${id}`).toBe(labels[id]);
	}
});
