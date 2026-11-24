import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import { RevealNoticeText, revealKindOf } from "./reveal";

const messages = {
	"ui.assets.reveal.shown": "Opened the folder of the file.",
	"ui.assets.reveal.copied": "Path copied:",
};

function markup(notice: Parameters<typeof RevealNoticeText>[0]["notice"]) {
	return renderToString(
		<I18nProvider messages={messages} locale="en" locales={["en"]}>
			<RevealNoticeText notice={notice} />
		</I18nProvider>,
	);
}

describe("what the page offers for the folder of a file", () => {
	test("the desktop app shows the folder, the browser in local mode copies the path", () => {
		expect(revealKindOf("local", true)).toBe("show");
		expect(revealKindOf("local", false)).toBe("copy");
	});

	// SDD-011 section 14.10: no route, tool or page reveals a path in network mode.
	test("network mode offers nothing, with or without a desktop bridge", () => {
		expect(revealKindOf("network", false)).toBeNull();
		expect(revealKindOf("network", true)).toBeNull();
	});
});

describe("the notice after a reveal", () => {
	test("says nothing when nothing happened", () => {
		expect(markup(null)).toBe("");
	});

	test("says the folder was opened, with no path", () => {
		const html = markup({ kind: "shown" });
		expect(html).toContain("Opened the folder of the file.");
		expect(html).toContain('role="status"');
	});

	// A folder named with a bidirectional override must not reorder the page around it.
	test("draws the path that was copied isolated, as text, and in a polite status", () => {
		const html = markup({
			kind: "copied",
			display: "/home/a\\u{202E}b/<img src=x onerror=window.__pwned=1>",
		});
		expect(html).toContain("Path copied:");
		expect(html).toContain("[unicode-bidi:isolate]");
		expect(html).toContain('dir="auto"');
		expect(html).not.toContain("<img src=x");
		expect(html).toContain("&lt;img src=x");
		expect(html).toContain('role="status"');
	});
});
