import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";
import { I18nProvider } from "@/lib/i18n";
import { devFixture } from "../boot/devFixture";
import { fakeId } from "../fixtures/fixtureServer";
import { startHarness } from "../fixtures/harness";
import { EditorProvider } from "../state/EditorContext";
import type { EditorRuntime } from "../state/runtime";
import { TimelineRows } from "./TimelineTable";

const messages = devFixture(1).bootstrap.messages;

function render(runtime: EditorRuntime): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en">
			<EditorProvider runtime={runtime}>
				<TooltipProvider>
					<TimelineRows onChosen={() => undefined} />
				</TooltipProvider>
			</EditorProvider>
		</I18nProvider>,
	);
}

describe("the list view of the timeline", () => {
	test("is a table with a row per item and a header per column", async () => {
		const h = await startHarness({ tracks: 2, itemsPerTrack: 3 });
		const html = render(h.runtime);
		expect(html).toContain("<table");
		expect(html.match(/<th scope="col"/g)?.length).toBe(7);
		expect(html.match(/data-item-row=/g)?.length).toBe(6);
		expect(html).toContain("Timeline items, 6 rows");
	});

	test("says which items are selected in words, not by colour", async () => {
		const h = await startHarness({ tracks: 1, itemsPerTrack: 2 });
		expect(render(h.runtime)).not.toContain(">Selected<");
		h.runtime.uiWriter.select([fakeId("itm", 1)]);
		expect(render(h.runtime).match(/>Selected</g)?.length).toBe(1);
	});

	test("says so when the timeline has no items", async () => {
		const h = await startHarness({ tracks: 1, itemsPerTrack: 0 });
		expect(render(h.runtime)).toContain("The timeline has no items yet.");
	});
});
