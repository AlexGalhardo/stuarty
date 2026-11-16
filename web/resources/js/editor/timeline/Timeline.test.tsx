import { renderToString } from "react-dom/server";
import { describe, expect, test } from "vitest";

import { TooltipProvider } from "@/components/ui/tooltip";
import { I18nProvider } from "@/lib/i18n";
import { devFixture } from "../boot/devFixture";
import { fakeId } from "../fixtures/fixtureServer";
import { startHarness } from "../fixtures/harness";
import TopBar from "../layout/TopBar";
import { EditorProvider } from "../state/EditorContext";
import type { EditorRuntime } from "../state/runtime";
import Timeline from "./Timeline";

const messages = devFixture(1).bootstrap.messages;

function render(runtime: EditorRuntime, node: React.ReactNode): string {
	return renderToString(
		<I18nProvider messages={messages} locale="en">
			<EditorProvider runtime={runtime}>
				<TooltipProvider>{node}</TooltipProvider>
			</EditorProvider>
		</I18nProvider>,
	);
}

describe("Timeline", () => {
	test("draws every track and item as a labelled block", async () => {
		const h = await startHarness({ tracks: 3, itemsPerTrack: 4 });
		const html = render(h.runtime, <Timeline />);
		expect(html.match(/role="gridcell"/g)?.length).toBe(12);
		// One row of cells and one header row per track, and the ruler row.
		expect(html.match(/role="row"/g)?.length).toBe(7);
		expect(html.match(/data-track-id=/g)?.length).toBe(3);
		expect(html).toContain("Title 5");
		expect(html).toContain("IMG_0421.MOV");
		expect(html).toContain('aria-label="Timeline"');
		expect(html).toContain("--ppf:1");
		expect(html).toContain("00:00:00");
	});

	test("a track without items still has a cell in its row", async () => {
		const h = await startHarness({ tracks: 2, itemsPerTrack: 0 });
		const html = render(h.runtime, <Timeline />);
		expect(html.match(/data-track-id=/g)?.length).toBe(2);
		expect(html.match(/role="gridcell"/g)?.length).toBe(2);
	});

	test("marks the selected item and remote highlights", async () => {
		const h = await startHarness();
		const id = fakeId("itm", 1);
		h.runtime.uiWriter.select([id]);
		const html = render(h.runtime, <Timeline />);
		const selected = html.match(/aria-selected="true"/g) ?? [];
		expect(selected.length).toBe(1);
		expect(html).toContain("ring-ed-selection");
	});

	test("a project above the threshold mounts the window, the selection and the focus", async () => {
		const h = await startHarness({ tracks: 5, itemsPerTrack: 40 });
		const count = (html: string): number =>
			html.match(/role="gridcell"/g)?.length ?? 0;
		const windowed = count(render(h.runtime, <Timeline />));
		expect(windowed).toBeGreaterThan(0);
		expect(windowed).toBeLessThan(200);
		expect(render(h.runtime, <Timeline />)).not.toContain(
			`data-item-id="${fakeId("itm", 40)}"`,
		);
		// Item 40 is the last of track 1, far past the window.
		h.runtime.uiWriter.select([fakeId("itm", 40)]);
		h.runtime.uiWriter.focusItem(fakeId("itm", 80));
		const pinned = render(h.runtime, <Timeline />);
		expect(count(pinned)).toBe(windowed + 2);
		expect(pinned).toContain(`data-item-id="${fakeId("itm", 40)}"`);
		expect(pinned).toContain(`data-item-id="${fakeId("itm", 80)}"`);
	});

	test("at the threshold every item is mounted", async () => {
		const h = await startHarness({ tracks: 5, itemsPerTrack: 30 });
		const html = render(h.runtime, <Timeline />);
		expect(html.match(/role="gridcell"/g)?.length).toBe(150);
	});

	test("an empty project shows the empty message", async () => {
		const h = await startHarness({ tracks: 0 });
		expect(render(h.runtime, <Timeline />)).toContain("The timeline is empty");
	});
});

describe("TopBar", () => {
	test("shows the name, the revision and the connection as text", async () => {
		const h = await startHarness();
		const html = render(h.runtime, <TopBar onLocaleChange={() => {}} />);
		expect(html).toContain("Launch teaser");
		expect(html).toContain("rev 0");
		expect(html).toContain("Live");
		expect(html).toContain('href="/projects"');
	});
});
