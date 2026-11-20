import { expect, test } from "vitest";

import en from "../../lang/en.json";
import pt from "../../lang/pt-BR.json";

// The platform presets of the core (core/export/preset.go) name a network and a placement, and the export
// dialog draws those labels as the core gives them. The briefing draws the same networks from the catalogue,
// so each catalogue must spell them as the core does, in every language: a brand name is not translated
// (VV-260).
const CORE_LABELS: Record<string, string> = {
	"youtube-shorts": "YouTube Shorts",
	"instagram-reels": "Instagram Reels",
	"facebook-reels": "Facebook Reels",
	tiktok: "TikTok",
};

test("the briefing names the networks as the export presets do, in every language", () => {
	for (const [language, catalog] of [
		["en", en],
		["pt-BR", pt],
	] as const) {
		const names: Record<string, string> =
			catalog.ui.projects.briefing.option.platform;
		for (const [id, label] of Object.entries(CORE_LABELS)) {
			expect(names[id], `${language} ${id}`).toBe(label);
		}
	}
});
