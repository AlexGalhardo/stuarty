import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import { devFixture } from "../editor/boot/devFixture";
import { SavedIndicator } from "./TopBar";

const messages = devFixture(1).bootstrap.messages;

function render(revision: number | null): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en">
			<SavedIndicator revision={revision} />
		</I18nProvider>,
	);
}

describe("image editor SavedIndicator", () => {
	test("names the time of the open revision once a document is on screen", () => {
		expect(render(3)).toMatch(/Saved at \d{1,2}:\d{2}/);
	});

	test("is plain text with no icon, as the final design draws it", () => {
		const html = render(3);
		expect(html).not.toContain("<svg");
		expect(html).toContain("data-saved-at");
	});

	test("says nothing before the document is loaded", () => {
		expect(render(null)).not.toContain("Saved");
	});
});
