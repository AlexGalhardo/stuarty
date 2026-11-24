import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import type { LibraryAsset } from "../editor/api/types";
import { SoundRow } from "./SoundRow";

vi.mock("../editor/timeline/items/waveform", () => ({
	PEAKS_RETRY_MS: 5000,
	watchPeaks: () => () => undefined,
	columnPeaks: () => new Float32Array(0),
}));

const PAYLOAD = "<img src=x onerror=window.__pwned=1>";

const messages = {
	"ui.assets.origin.imported": "Imported",
	"ui.assets.reusable_badge": "Reusable",
	"ui.assets.sound.waveform": "Waveform of {name}",
	"ui.assets.sound.player": "Play {name}",
	"ui.assets.sound.no_waveform": "No waveform for this file.",
	"ui.assets.size.unknown": "Size unknown",
	"ui.assets.state.importing": "Still importing",
	"ui.assets.state.missing": "File missing",
	"ui.assets.added": "Added {date}",
};

const sound = (extra: Partial<LibraryAsset> = {}): LibraryAsset => ({
	hash: `sha256:${"b".repeat(64)}`,
	name: "sting.wav",
	kind: "audio",
	bytes: 2_097_152,
	size_text: "2.1 MB",
	state: "ready",
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

	test("shows the duration the service reported, as a timecode, and the size the service wrote", () => {
		const html = markup(sound());
		expect(html).toContain("01:05.250");
		expect(html).toContain("2.1 MB");
		expect(html).not.toContain("Size unknown");
	});

	test("a file with no known size says so and why, never 0 B", () => {
		const importing = markup(
			sound({ bytes: null, size_text: "", state: "importing" }),
		);
		expect(importing).toContain("Size unknown");
		expect(importing).toContain("Still importing");
		expect(importing).not.toContain("0 B");
		const missing = markup(
			sound({ bytes: null, size_text: "", state: "missing" }),
		);
		expect(missing).toContain("File missing");
		expect(missing).toContain('data-state="missing"');
	});

	test("tells two sounds of the same name apart by size and day", () => {
		const html = markup(sound({ created_at: "2026-11-20T10:00:00Z" }));
		expect(html).toContain("2.1 MB");
		expect(html).toContain("Added");
	});

	test("renders the name and the tags as text, never as markup", () => {
		const html = markup(sound({ name: PAYLOAD, tags: [PAYLOAD] }));
		expect(html).not.toContain("<img src=x");
		expect(html).toContain("&lt;img src=x");
	});
});

describe("the facts on a row", () => {
	test("the origin and the reuse mark are words with an icon", () => {
		const html = markup(
			sound({ origin: { kind: "imported" }, reusable: true }),
		);
		expect(html).toContain("Imported");
		expect(html).toContain("Reusable");
		expect(html).toContain("<svg");
	});
});
