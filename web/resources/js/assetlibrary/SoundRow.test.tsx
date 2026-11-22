import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import type { LibraryAsset } from "../editor/api/types";
import { SoundRow } from "./SoundRow";

vi.mock("../editor/timeline/items/waveform", () => ({
	loadPeaks: () => new Promise(() => undefined),
	columnPeaks: () => new Float32Array(0),
	isPeaksPending: () => false,
}));

const PAYLOAD = "<img src=x onerror=window.__pwned=1>";

const messages = {
	"ui.assets.origin.imported": "Imported",
	"ui.assets.reusable_badge": "Reusable",
	"ui.assets.sound.waveform": "Waveform of {name}",
	"ui.assets.sound.player": "Play {name}",
	"ui.assets.sound.no_waveform": "No waveform for this file.",
};

const sound = (extra: Partial<LibraryAsset> = {}): LibraryAsset => ({
	hash: `sha256:${"b".repeat(64)}`,
	name: "sting.wav",
	kind: "audio",
	bytes: 2_097_152,
	duration_ms: 65_250,
	projects: 1,
	poster: { timeline_id: "prj_x", asset_id: "ast_1" },
	...extra,
});

function markup(a: LibraryAsset): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			<ul>
				<SoundRow asset={a} selected={false} onOpen={() => undefined} />
			</ul>
		</I18nProvider>,
	);
}

describe("a row of Sound Assets", () => {
	test("has a player on the proxy of the file and a waveform from the server", () => {
		const html = markup(sound({ tags: ["music"] }));
		expect(html).toContain('src="/api/v1/projects/prj_x/media/ast_1/proxy"');
		expect(html).toContain("<audio");
		expect(html).toContain('data-testid="waveform"');
		expect(html).toContain('aria-label="Play sting.wav"');
		expect(html).toContain("music");
	});

	test("shows the duration the service reported, as a timecode, and the size", () => {
		const html = markup(sound());
		expect(html).toContain("01:05.250");
		expect(html).toContain("2 MB");
	});

	test("renders the name and the tags as text, never as markup", () => {
		const html = markup(sound({ name: PAYLOAD, tags: [PAYLOAD] }));
		expect(html).not.toContain("<img src=x");
		expect(html).toContain("&lt;img src=x");
	});
});
