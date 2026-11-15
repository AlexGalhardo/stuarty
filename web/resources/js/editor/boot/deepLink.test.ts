import { describe, expect, test } from "vitest";

import {
	FixtureServer,
	fakeId,
	flush,
	ManualTimers,
	makeDocument,
} from "../fixtures/fixtureServer";
import { startHarness } from "../fixtures/harness";
import { createRuntime } from "../state/runtime";
import {
	type DeepLinkDeps,
	deepLinkQuery,
	parseDeepLink,
	parseTime,
	startDeepLink,
} from "./deepLink";

const fps = { num: 30, den: 1 };

describe("parseTime", () => {
	test("frames, seconds and a clock", () => {
		expect(parseTime("375f", fps)).toBe(375);
		expect(parseTime("12.5s", fps)).toBe(375);
		expect(parseTime("00:00:12.500", fps)).toBe(375);
		expect(parseTime("00:01:00", fps)).toBe(1800);
	});

	test("rejects what is not a time", () => {
		expect(parseTime("soon", fps)).toBeNull();
		expect(parseTime("-3s", fps)).toBeNull();
		expect(parseTime("", fps)).toBeNull();
	});
});

describe("parseDeepLink", () => {
	test("reads t and item, and drops an item that is not an id", () => {
		expect(parseDeepLink("?t=12.5s&item=itm_7k2m9x4q1b", fps)).toEqual({
			frame: 375,
			item: "itm_7k2m9x4q1b",
		});
		expect(parseDeepLink("?item=<script>", fps)).toEqual({
			frame: null,
			item: null,
		});
		expect(parseDeepLink("", fps)).toEqual({ frame: null, item: null });
	});

	test("writes the place back as frames", () => {
		expect(deepLinkQuery(0, null)).toBe("");
		expect(deepLinkQuery(375, "itm_7k2m9x4q1b")).toBe(
			"?t=375f&item=itm_7k2m9x4q1b",
		);
	});
});

function fakeDeps(search: string) {
	const urls: string[] = [];
	const scrolled: string[] = [];
	const timers: { callback: () => void; ms: number; live: boolean }[] = [];
	const deps: DeepLinkDeps = {
		search,
		replaceUrl: (q) => urls.push(q),
		scrollIntoView: (id) => scrolled.push(id),
		setTimeout: (callback, ms) => {
			timers.push({ callback, ms, live: true });
			return timers.length - 1;
		},
		clearTimeout: (handle) => {
			timers[handle as number].live = false;
		},
	};
	const run = (ms: number): void => {
		for (const t of timers) {
			if (t.live && t.ms === ms) {
				t.live = false;
				t.callback();
			}
		}
	};
	return { deps, urls, scrolled, run };
}

describe("startDeepLink", () => {
	test("sets the playhead, selects the item and scrolls to it", async () => {
		const h = await startHarness({ tracks: 1, itemsPerTrack: 2 });
		const id = fakeId("itm", 1);
		const f = fakeDeps(`?t=60f&item=${id}`);
		startDeepLink(h.runtime, f.deps);
		expect(h.runtime.ui.getState().playheadFrame).toBe(60);
		expect(h.runtime.ui.getState().selection.items).toEqual([id]);
		f.run(0);
		expect(f.scrolled).toEqual([id]);
	});

	test("an item that the document does not have is ignored", async () => {
		const h = await startHarness();
		const f = fakeDeps("?item=itm_zzzzzzzzzz");
		startDeepLink(h.runtime, f.deps);
		expect(h.runtime.ui.getState().selection.items).toEqual([]);
	});

	test("a selection is written at once, the playhead after it rests for 1 s", async () => {
		const h = await startHarness({ tracks: 1, itemsPerTrack: 2 });
		const f = fakeDeps("");
		startDeepLink(h.runtime, f.deps);

		h.runtime.uiWriter.select([fakeId("itm", 1)]);
		expect(f.urls).toEqual([`?item=${fakeId("itm", 1)}`]);

		h.runtime.uiWriter.setPlayhead(10);
		h.runtime.uiWriter.setPlayhead(20);
		expect(f.urls.length).toBe(1);
		f.run(1000);
		expect(f.urls.at(-1)).toBe(`?t=20f&item=${fakeId("itm", 1)}`);
		expect(f.urls.length).toBe(2);
	});

	test("waits for the document before it applies the link", async () => {
		const server = new FixtureServer(makeDocument({}));
		const runtime = createRuntime({
			projectId: server.projectId,
			actor: "local:ui",
			mode: "local",
			transport: server,
			timers: new ManualTimers(),
		});
		const f = fakeDeps("?t=5f");
		startDeepLink(runtime, f.deps);
		expect(runtime.ui.getState().playheadFrame).toBe(0);
		await runtime.sync.start();
		await flush();
		await runtime.sync.settled();
		expect(runtime.ui.getState().playheadFrame).toBe(5);
	});
});
