import { renderToString } from "react-dom/server";
import { describe, expect, test, vi } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import type { LibraryAsset } from "../editor/api/types";
import { MediaCard } from "./MediaCard";

vi.mock("@inertiajs/react", () => ({ Head: () => null, Link: () => null }));

const PAYLOAD = "<img src=x onerror=window.__pwned=1>";

const messages = {
	"ui.assets.origin.imported": "Imported",
	"ui.assets.origin.generated": "Generated",
	"ui.assets.origin.fetched_by_agent": "Fetched by an agent",
	"ui.assets.kind.audio": "Audio",
	"ui.assets.kind.video": "Video",
	"ui.assets.reusable_badge": "Reusable",
};

const asset = (extra: Partial<LibraryAsset> = {}): LibraryAsset => ({
	hash: `sha256:${"a".repeat(64)}`,
	name: "reaction.mov",
	kind: "video",
	bytes: 10,
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
				bytes: 3 * 1024 * 1024,
				origin: { kind: "generated" },
			}),
		);
		expect(html).toContain("00:58.000");
		expect(html).toContain("3 MB");
		expect(html).toContain("Generated");
	});

	test("a file the service did not mark reusable carries no reusable badge", () => {
		expect(markup(asset())).not.toContain("Reusable");
	});
});
