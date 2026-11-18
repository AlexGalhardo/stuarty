import { describe, expect, test } from "vitest";

import type { Change } from "../api/types";
import {
	errorBody,
	FixtureServer,
	fakeId,
	flush,
	fullItem,
	ManualTimers,
	makeDocument,
	seeded,
} from "../fixtures/fixtureServer";
import { type Harness, itemMoved, startHarness } from "../fixtures/harness";
import { createRuntime } from "./runtime";

// SDD-006 section 11 check 14: the reconciliation harness. In every case the
// final document and revision equal the one of the server.

const ITEM_1 = fakeId("itm", 1);
const ITEM_2 = fakeId("itm", 2);

async function expectInSync(h: Harness): Promise<void> {
	await h.runtime.sync.settled();
	const state = h.runtime.documents.getState();
	expect(state.revision).toBe(h.server.revision);
	expect(state.document).toEqual(h.server.document);
	expect(h.mismatches).toEqual([]);
}

function badEvent(h: Harness): Record<string, unknown> {
	return {
		project_id: h.server.projectId,
		revision: 1,
		base_revision: 0,
		kind: "edit",
		actor: "local:cli",
		diff: [itemMoved(ITEM_1, 999, 5)],
	};
}

describe("boot", () => {
	test("loads the document and opens the stream with since", async () => {
		const h = await startHarness();
		const state = h.runtime.documents.getState();
		expect(state.status).toBe("ready");
		expect(state.connection).toBe("live");
		expect(state.revision).toBe(0);
		expect(h.server.openedWith).toEqual([0]);
		expect(state.endFrame).toBe(h.server.endFrame);
		await expectInSync(h);
	});
});

describe("a load the core refuses", () => {
	test("keeps the code, the message and the hint for the banner", async () => {
		const server = new FixtureServer(makeDocument({}));
		server.failures.push({
			match: "GET /projects/",
			status: 500,
			body: {
				...errorBody(
					"E_VALIDATION",
					"this project was saved by a newer version",
				),
				hint: "update vukem-video",
			},
		});
		const runtime = createRuntime({
			projectId: server.projectId,
			actor: "local:ui",
			mode: "local",
			transport: server,
			timers: new ManualTimers(),
		});
		await runtime.sync.start();
		await flush();
		await runtime.sync.settled();
		const state = runtime.documents.getState();
		expect(state.status).toBe("failed");
		expect(state.loadError).toEqual({
			code: "E_VALIDATION",
			message: "this project was saved by a newer version",
			hint: "update vukem-video",
		});
	});

	test("a transport failure before any document is a failed load, not an offline editor", async () => {
		const server = new FixtureServer(makeDocument({}));
		server.setNetworkDown(true);
		const runtime = createRuntime({
			projectId: server.projectId,
			actor: "local:ui",
			mode: "local",
			transport: server,
			timers: new ManualTimers(),
		});
		await runtime.sync.start();
		await flush();
		await runtime.sync.settled();
		const state = runtime.documents.getState();
		expect(state.status).toBe("failed");
		expect(state.document).toBeNull();
		expect(state.loadError?.code).toBe("E_NETWORK");
	});

	test("a load that succeeds leaves no error", async () => {
		const h = await startHarness();
		expect(h.runtime.documents.getState().loadError).toBeNull();
	});
});

describe("reconciliation of the response and the event", () => {
	test("response first, then the event of the same revision", async () => {
		const h = await startHarness();
		const { result, event } = h.server.commit([itemMoved(ITEM_1, 0, 30)], {
			publish: "none",
		});
		h.runtime.sync.receiveEdit(result);
		expect(h.runtime.documents.getState().revision).toBe(1);
		h.server.deliver(event);
		await expectInSync(h);
	});

	test("event first, then the response", async () => {
		const h = await startHarness();
		const { result } = h.server.commit([itemMoved(ITEM_1, 0, 30)]);
		expect(h.runtime.documents.getState().revision).toBe(1);
		h.runtime.sync.receiveEdit(result);
		await expectInSync(h);
	});

	test("a duplicated event is applied once", async () => {
		const h = await startHarness();
		const { event } = h.server.commit([itemMoved(ITEM_1, 0, 30)], {
			publish: "none",
		});
		h.server.deliver(event);
		h.server.deliver(event);
		await expectInSync(h);
	});

	test("a dry run changes nothing", async () => {
		const h = await startHarness();
		const { result } = h.server.commit([itemMoved(ITEM_1, 0, 30)], {
			publish: "none",
		});
		h.runtime.sync.receiveEdit({ ...result, applied: false, dry_run: true });
		expect(h.runtime.documents.getState().revision).toBe(0);
	});
});

describe("ordering and gaps", () => {
	test("N+1 before N is buffered and drained in order", async () => {
		const h = await startHarness();
		const one = h.server.commit([itemMoved(ITEM_1, 0, 30)], {
			publish: "none",
		});
		const two = h.server.commit([itemMoved(ITEM_2, 105, 200)], {
			publish: "none",
		});
		h.server.deliver(two.event);
		expect(h.runtime.documents.getState().revision).toBe(0);
		h.server.deliver(one.event);
		await expectInSync(h);
		expect(h.server.openedWith).toEqual([0]);
	});

	test("a missing revision causes a resync after 2 seconds", async () => {
		const h = await startHarness();
		h.server.commit([itemMoved(ITEM_1, 0, 30)], { publish: "none" });
		const two = h.server.commit([itemMoved(ITEM_2, 105, 200)], {
			publish: "none",
		});
		h.server.deliver(two.event);
		await h.timers.advance(1999);
		expect(h.runtime.documents.getState().revision).toBe(0);
		await h.timers.advance(1);
		await flush();
		await expectInSync(h);
		expect(h.server.openedWith).toEqual([0, 2]);
	});

	test("a before-value mismatch causes a resync", async () => {
		const h = await startHarness();
		h.server.deliver(badEvent(h));
		await flush();
		await expectInSync(h);
		expect(h.server.openedWith.length).toBe(2);
	});

	test("a truncated diff causes a resync", async () => {
		const h = await startHarness();
		h.server.commit([itemMoved(ITEM_1, 0, 30)], { truncated: true });
		await flush();
		await expectInSync(h);
		expect(h.runtime.documents.getState().revision).toBe(1);
		expect(h.server.openedWith).toEqual([0, 1]);
	});

	test("resync.required causes a resync", async () => {
		const h = await startHarness();
		h.server.commit([itemMoved(ITEM_1, 0, 30)], { publish: "none" });
		h.server.deliver(
			{ project_id: h.server.projectId, revision: 1, reason: "gap_too_large" },
			"resync.required",
		);
		await flush();
		await expectInSync(h);
	});

	test("three resyncs without progress end in failed", async () => {
		const h = await startHarness();
		for (let i = 0; i < 4; i++) {
			h.server.deliver(badEvent(h));
			await flush();
			await h.runtime.sync.settled();
		}
		expect(h.runtime.documents.getState().status).toBe("failed");
	});
});

describe("the mechanical patch against the server", () => {
	test("200 random batches leave document and revision equal to the server", async () => {
		const h = await startHarness({ tracks: 4, itemsPerTrack: 6 });
		const random = seeded(42);
		let created = 1000;
		for (let n = 0; n < 200; n++) {
			const doc = h.server.document;
			const ids = Object.keys(doc.items);
			const pick = random();
			const diff: Change[] = [];
			if (pick < 0.5 && ids.length > 0) {
				const id = ids[Math.floor(random() * ids.length)];
				const before = doc.items[id].start;
				diff.push(
					itemMoved(id, before, before + 1 + Math.floor(random() * 50)),
				);
			} else if (pick < 0.7) {
				created++;
				const id = fakeId("itm", created);
				const track = Object.keys(doc.tracks)[0];
				diff.push({
					kind: "item.added",
					id,
					object: fullItem({
						id,
						type: "text",
						track,
						start: created,
						duration: 30,
						text: "x",
					}),
				});
			} else if (pick < 0.85 && ids.length > 1) {
				const id = ids[Math.floor(random() * ids.length)];
				diff.push({
					kind: "item.removed",
					id,
					object: JSON.parse(JSON.stringify(doc.items[id])),
				});
			} else if (pick < 0.95) {
				const [trackId] = Object.keys(doc.tracks);
				const before = doc.tracks[trackId].hidden ?? null;
				diff.push({
					kind: "track.updated",
					id: trackId,
					changes: { hidden: [before, before === true ? null : true] },
				});
			} else {
				diff.push({
					kind: "project.updated",
					id: h.server.projectId,
					changes: { name: [doc.name, `Name ${n}`] },
				});
			}
			h.server.commit(diff, { actor: n % 2 === 0 ? "local:ui" : "local:cli" });
			await flush(2);
		}
		await expectInSync(h);
		expect(h.runtime.documents.getState().revision).toBe(200);
	});
});

describe("remote edits (RF-118)", () => {
	test("another actor is recorded and its items highlighted for 1.5 s", async () => {
		const h = await startHarness();
		h.server.commit([itemMoved(ITEM_1, 0, 30)], {
			actor: "local:claude-code",
		});
		const activity = h.runtime.activity.getState();
		expect(activity.entries[0].actor).toBe("local:claude-code");
		expect(activity.entries[0].counts.items).toBe(1);
		expect(activity.highlighted).toEqual([ITEM_1]);
		await h.timers.advance(1499);
		expect(h.runtime.activity.getState().highlighted).toEqual([ITEM_1]);
		await h.timers.advance(1);
		expect(h.runtime.activity.getState().highlighted).toEqual([]);
	});

	test("the own actor leaves no trace in the indicator", async () => {
		const h = await startHarness();
		h.server.commit([itemMoved(ITEM_1, 0, 30)], { actor: "local:ui" });
		expect(h.runtime.activity.getState().entries).toEqual([]);
	});
});

describe("project state", () => {
	test("project.updated trashes the project in the read model", async () => {
		const h = await startHarness();
		h.server.deliver(
			{ project_id: h.server.projectId, state: "trashed", at: "x" },
			"project.updated",
		);
		expect(h.runtime.documents.getState().state).toBe("trashed");
	});
});
