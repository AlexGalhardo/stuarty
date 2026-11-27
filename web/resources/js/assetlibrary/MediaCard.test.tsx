import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import type { LibraryAsset } from "../editor/api/types";
import { assetEditorTarget, MediaCard } from "./MediaCard";

vi.mock("@inertiajs/react", () => ({ Head: () => null, Link: () => null }));

const PAYLOAD = "<img src=x onerror=window.__pwned=1>";

const messages = {
	"ui.assets.origin.imported": "Imported",
	"ui.assets.origin.generated": "Generated",
	"ui.assets.origin.fetched_by_agent": "Fetched by an agent",
	"ui.assets.kind.audio": "Audio",
	"ui.assets.kind.video": "Video",
	"ui.assets.reusable_badge": "Reusable",
	"ui.assets.size.unknown": "Size unknown",
	"ui.assets.state.importing": "Still importing",
	"ui.assets.state.missing": "File missing",
	"ui.assets.added": "Added {date}",
};

const asset = (extra: Partial<LibraryAsset> = {}): LibraryAsset => ({
	hash: `sha256:${"a".repeat(64)}`,
	name: "reaction.mov",
	kind: "video",
	bytes: 10,
	size_text: "10 B",
	state: "ready",
	projects: 1,
	...extra,
});

function markup(a: LibraryAsset): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			<ul>
				<MediaCard asset={a} selected={false} onOpen={() => undefined} />
			</ul>
		</I18nProvider>,
	);
}

describe("a card of the asset library", () => {
	test("draws the poster the service points to, and the origin, the mark and the tags", () => {
		const html = markup(
			asset({
				origin: { kind: "fetched_by_agent" },
				reusable: true,
				tags: ["b-roll"],
				poster: { timeline_id: "prj_x", asset_id: "ast_1" },
			}),
		);
		expect(html).toContain(
			"/api/v1/projects/prj_x/media/ast_1/thumbnail?width=320",
		);
		expect(html).toContain("Fetched by an agent");
		expect(html).toContain("Reusable");
		expect(html).toContain("b-roll");
	});

	test("an audio file has no picture and says its kind", () => {
		const html = markup(
			asset({
				kind: "audio",
				poster: { timeline_id: "prj_x", asset_id: "ast_1" },
			}),
		);
		expect(html).not.toContain("<img");
		expect(html).toContain("Audio");
	});

	test("a file with no library row reads as imported", () => {
		expect(markup(asset())).toContain("Imported");
	});

	// SDD-007 section 8.5: the name and the tags are text that a person or an agent wrote.
	test("renders the name and the tags as text, never as markup", () => {
		const html = markup(asset({ name: PAYLOAD, tags: [PAYLOAD] }));
		expect(html).not.toContain("<img src=x");
		expect(html).toContain("&lt;img src=x");
	});
});

describe("the facts on a card", () => {
	test("a video says its length and size, and the origin is a word", () => {
		const html = markup(
			asset({
				duration_ms: 58_000,
				bytes: 3_145_728,
				size_text: "3.1 MB",
				origin: { kind: "generated" },
			}),
		);
		expect(html).toContain("00:58.000");
		expect(html).toContain("3.1 MB");
		expect(html).toContain("Generated");
	});

	// The page formats nothing: whatever the service wrote is what is drawn (SDD-011 section 14.9).
	test("draws the size text of the service and does not compute one from the bytes", () => {
		const html = markup(
			asset({ bytes: 1_000_000, size_text: "from the service" }),
		);
		expect(html).toContain("from the service");
		expect(html).not.toContain("1 MB");
		expect(html).not.toContain("976");
	});

	test("a file still importing or missing says so instead of a size of 0", () => {
		const importing = markup(
			asset({ bytes: null, size_text: "", state: "importing" }),
		);
		expect(importing).toContain("Size unknown");
		expect(importing).toContain("Still importing");
		expect(importing).not.toContain("0 B");
		const missing = markup(
			asset({ bytes: null, size_text: "", state: "missing" }),
		);
		expect(missing).toContain("File missing");
		expect(missing).toContain('data-state="missing"');
	});

	test("a file over 4 GiB draws what the service wrote", () => {
		const html = markup(asset({ bytes: 4_294_967_296, size_text: "4.3 GB" }));
		expect(html).toContain("4.3 GB");
	});

	test("says the day the file was added, so two files of the same name can be told apart", () => {
		const html = markup(asset({ created_at: "2026-11-20T10:00:00Z" }));
		expect(html).toContain('data-testid="asset-added"');
		expect(html).toContain("Added");
	});

	test("the length over the poster is drawn with theme tokens, not raw black and white", () => {
		const html = markup(asset({ duration_ms: 4_000 }));
		const chip = html.match(
			/<span[^>]*data-testid="media-duration"[^>]*>/,
		)?.[0];
		expect(chip).toBeDefined();
		expect(chip).toContain("bg-surface");
		expect(chip).toContain("text-text");
		expect(chip).not.toMatch(/bg-black|text-white/);
	});

	test("a file the service did not mark reusable carries no reusable badge", () => {
		expect(markup(asset())).not.toContain("Reusable");
	});

	test("maps video and image assets to their direct editor routes", () => {
		const hash = `sha256:${"b".repeat(64)}`;
		const encoded = encodeURIComponent(hash);
		expect(
			assetEditorTarget(
				asset({
					hash,
					kind: "video",
					poster: { timeline_id: "prj_1234567890", asset_id: "ast_1" },
				}),
			),
		).toBe(`/editor/prj_1234567890?asset=${encoded}`);
		expect(assetEditorTarget(asset({ hash, kind: "video" }))).toBe(
			`/edit-video?asset=${encoded}`,
		);
		expect(assetEditorTarget(asset({ hash, kind: "image" }))).toBe(
			`/edit-image?asset=${encoded}`,
		);
		expect(assetEditorTarget(asset({ hash, kind: "audio" }))).toBeNull();
	});
});
