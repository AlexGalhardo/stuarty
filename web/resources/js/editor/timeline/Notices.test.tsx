import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { I18nProvider } from "@/lib/i18n";
import { devFixture } from "../boot/devFixture";
import { startHarness } from "../fixtures/harness";
import { EditorProvider } from "../state/EditorContext";
import Notices, { MAX_NOTICES_SHOWN } from "./Notices";

const messages = devFixture(1).bootstrap.messages;

describe("Notices", () => {
	test("a run of failed edits shows a few notices that let the pointer through", async () => {
		const h = await startHarness();
		for (let i = 0; i < 5; i++) {
			h.runtime.noticeWriter.push({
				kind: "error",
				code: "E_OVERLAP",
				message: `edit ${i} failed`,
			});
		}
		const html = renderToString(
			<I18nProvider messages={messages} locale="en">
				<EditorProvider runtime={h.runtime}>
					<Notices />
				</EditorProvider>
			</I18nProvider>,
		);
		expect(html.match(/role="alert"/g)?.length).toBe(MAX_NOTICES_SHOWN);
		// The newest are the ones drawn.
		expect(html).toContain("edit 4 failed");
		expect(html).not.toContain("edit 1 failed");
		// The stack is click-through; the dismiss buttons opt back in.
		expect(html).toMatch(/class="pointer-events-none fixed/);
		expect(html.match(/pointer-events-auto/g)?.length).toBe(MAX_NOTICES_SHOWN);
	});
});
