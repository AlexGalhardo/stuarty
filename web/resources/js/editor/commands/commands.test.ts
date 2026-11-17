import { describe, expect, test } from "vitest";

import type { TransportRequest } from "../api/transport";
import type { EditRequest, MediaEntry } from "../api/types";
import { flush } from "../fixtures/fixtureServer";
import { type Harness, startHarness } from "../fixtures/harness";
import { parsePastedPaths } from "../library/importPaths";
import { createActions } from "./actions";
import {
	addClipOperations,
	captionOperations,
	moveOperations,
	renameOperations,
	splitOperations,
	trimEndOperations,
	trimOperations,
	withNextFree,
} from "./builders";

const ids = (): (() => string) => {
	let n = 0;
	return () => `trk_new${n++}`;
};

interface Sent {
	request: EditRequest;
}

// An edit handler that counts the requests and answers with the server commit,
// or with a scripted error.
function scripted(
	h: Harness,
	reply: (request: EditRequest, sent: Sent[]) => unknown,
): Sent[] {
	const sent: Sent[] = [];
	h.server.editHandler = (request: TransportRequest) => {
		const body = request.body as EditRequest;
		sent.push({ request: body });
		return reply(body, sent) as never;
	};
	return sent;
}

// A committed change that is not about the gesture under test: it only bumps
// the revision, as the core would after applying the batch.
function bump(h: Harness): import("../api/types").Change {
	return {
		kind: "project.updated",
		id: h.server.projectId,
		changes: { name: [h.server.document.name, `${h.server.document.name}+`] },
	};
}

function firstItem(h: Harness): {
	id: string;
	start: number;
	duration: number;
} {
	const item = Object.values(h.server.document.items)[0];
	if (item === undefined) throw new Error("fixture has no item");
	return { id: item.id, start: item.start, duration: item.duration };
}

describe("builders", () => {
	test("add to timeline uses the first visual track, or adds one in the same batch", () => {
		const onTrack = addClipOperations({
			assetId: "ast_1",
			firstVisualTrack: "trk_a",
			newTrackId: ids(),
		});
		expect(onTrack).toEqual([
			{
				op: "clip.add",
				args: { track_id: "trk_a", asset_id: "ast_1" },
			},
		]);
		const created = addClipOperations({
			assetId: "ast_1",
			firstVisualTrack: null,
			newTrackId: ids(),
		});
		expect(created.map((o) => o.op)).toEqual(["track.add", "clip.add"]);
		expect(created[1]?.args.track_id).toBe("trk_new0");
	});

	test("split and trim only take items the playhead lies strictly inside", () => {
		const items = [
			{ id: "a", start: 0, duration: 100 },
			{ id: "b", start: 100, duration: 50 },
		];
		expect(splitOperations(items, 100).map((o) => o.args.item_id)).toEqual([]);
		expect(splitOperations(items, 50)).toEqual([
			{ op: "item.split", args: { item_id: "a", at: "50f" } },
		]);
		expect(trimOperations(items, "end", 120)).toEqual([
			{ op: "item.trim", args: { item_id: "b", edge: "end", to: "120f" } },
		]);
	});

	test("the caption keeps the duration the user typed, with its unit", () => {
		const ops = captionOperations({
			text: "Hello",
			duration: "3s",
			playhead: 12,
		});
		expect(ops[0]?.args).toMatchObject({
			start: "12f",
			duration: "3s",
			text: "Hello",
		});
	});

	test("the next free position is the value the core named", () => {
		const ops = withNextFree(
			[{ op: "clip.add", args: { track_id: "t", asset_id: "a", start: "0f" } }],
			120,
			0,
		);
		expect(ops[0]?.args.start).toBe("120f");
	});

	test("pasted paths lose their quotes and empty lines", () => {
		expect(
			parsePastedPaths('  "C:\\media\\a b.mp4"  \r\n\r\n/home/x/c.mov\n'),
		).toEqual(["C:\\media\\a b.mp4", "/home/x/c.mov"]);
	});
});

describe("gestures send one request and wait for the answer", () => {
	test("split: one request, and the read model does not change before the answer", async () => {
		const h = await startHarness();
		const item = firstItem(h);
		const sent = scripted(h, (body) => ({
			status: 200,
			body: h.server.commit([bump(h)], {
				description: body.operations[0]?.op,
			}).result,
		}));
		h.runtime.uiWriter.select([item.id]);
		h.runtime.uiWriter.setPlayhead(item.start + 10);
		const before = h.runtime.documents.getState().revision;
		const done = createActions(h.runtime).splitAtPlayhead();
		expect(h.runtime.documents.getState().revision).toBe(before);
		await done;
		await flush();
		expect(sent).toHaveLength(1);
		expect(sent[0]?.request.operations).toEqual([
			{
				op: "item.split",
				args: { item_id: item.id, at: `${item.start + 10}f` },
			},
		]);
		expect(sent[0]?.request.base_revision).toBe(before);
		expect(h.runtime.documents.getState().revision).toBe((before ?? 0) + 1);
	});

	test("paste attributes remembers the source across separate action sets (VV-091)", async () => {
		const h = await startHarness();
		const items = Object.values(h.server.document.items);
		const [from, to] = items;
		if (from === undefined || to === undefined)
			throw new Error("fixture needs two items");
		const sent = scripted(h, () => ({
			status: 200,
			body: h.server.commit([bump(h)]).result,
		}));
		h.runtime.uiWriter.select([from.id]);
		// The context menu builds its actions anew each time it opens.
		expect(createActions(h.runtime).copyAttributes()).toBe(true);
		h.runtime.uiWriter.select([to.id]);
		await createActions(h.runtime).pasteAttributes();
		expect(sent[0]?.request.operations[0]).toMatchObject({
			op: "item.paste_attributes",
			args: { from_item_id: from.id, item_ids: [to.id] },
		});
	});

	test("the link key with one item says so and sends nothing (VV-125)", async () => {
		const h = await startHarness();
		const item = firstItem(h);
		const sent = scripted(h, () => ({ status: 500, body: null }));
		h.runtime.uiWriter.select([item.id]);
		expect(await createActions(h.runtime).toggleLink()).toBeNull();
		expect(sent).toHaveLength(0);
		expect(h.runtime.notices.getState().notices[0]?.messageKey).toBe(
			"ui.editor.notice.link_needs_two",
		);
	});

	test("add to timeline enters the track of the selected item (VV-092)", async () => {
		const h = await startHarness();
		const doc = h.server.document;
		const titles = Object.values(doc.items).find((i) => i.type === "text");
		const asset = Object.keys(doc.assets)[0];
		if (titles === undefined || asset === undefined) throw new Error("fixture");
		const sent = scripted(h, () => ({
			status: 200,
			body: h.server.commit([bump(h)]).result,
		}));
		h.runtime.uiWriter.select([titles.id]);
		await createActions(h.runtime).addToTimeline(asset);
		expect(sent[0]?.request.operations[0]).toMatchObject({
			op: "clip.add",
			args: { track_id: titles.track },
		});
	});

	test("add to timeline enters the chosen track, or a new one, with no start (VV-092)", async () => {
		const h = await startHarness();
		const doc = h.server.document;
		const asset = Object.keys(doc.assets)[0];
		const chosen = Object.values(doc.tracks).find((t) => t.kind === "visual");
		if (asset === undefined || chosen === undefined) throw new Error("fixture");
		const sent = scripted(h, () => ({
			status: 200,
			body: h.server.commit([bump(h)]).result,
		}));
		const actions = createActions(h.runtime);
		await actions.addToTimeline(asset, chosen.id);
		expect(sent[0]?.request.operations).toEqual([
			{ op: "clip.add", args: { track_id: chosen.id, asset_id: asset } },
		]);
		await actions.addToTimeline(asset, null);
		expect(sent[1]?.request.operations.map((o) => o.op)).toEqual([
			"track.add",
			"clip.add",
		]);
		expect(sent[1]?.request.operations[1]?.args.start).toBeUndefined();
	});

	test("split with the playhead outside the item sends nothing", async () => {
		const h = await startHarness();
		const item = firstItem(h);
		const sent = scripted(h, () => ({ status: 500, body: null }));
		h.runtime.uiWriter.select([item.id]);
		h.runtime.uiWriter.setPlayhead(item.start);
		expect(await createActions(h.runtime).splitAtPlayhead()).toBeNull();
		expect(sent).toHaveLength(0);
	});

	test("trim start sends one item.trim to the playhead", async () => {
		const h = await startHarness();
		const item = firstItem(h);
		const sent = scripted(h, () => ({
			status: 200,
			body: h.server.commit([bump(h)]).result,
		}));
		h.runtime.uiWriter.select([item.id]);
		h.runtime.uiWriter.setPlayhead(item.start + 5);
		await createActions(h.runtime).trimToPlayhead("start");
		expect(sent[0]?.request.operations).toEqual([
			{
				op: "item.trim",
				args: { item_id: item.id, edge: "start", to: `${item.start + 5}f` },
			},
		]);
	});

	test("add to timeline with no visual track adds one in the same batch", async () => {
		const h = await startHarness({ tracks: 0, itemsPerTrack: 0 });
		const assetId = Object.keys(h.server.document.assets)[0] as string;
		const sent = scripted(h, () => ({
			status: 200,
			body: h.server.commit([bump(h)]).result,
		}));
		await createActions(h.runtime).addToTimeline(assetId);
		expect(sent).toHaveLength(1);
		expect(sent[0]?.request.operations.map((o) => o.op)).toEqual([
			"track.add",
			"clip.add",
		]);
	});

	test("an asset the read model does not have is never placed", async () => {
		const h = await startHarness();
		const sent = scripted(h, () => ({ status: 500, body: null }));
		expect(
			await createActions(h.runtime).addToTimeline("ast_missing"),
		).toBeNull();
		expect(sent).toHaveLength(0);
	});

	test("an aborted request (499) shows no notice", async () => {
		const h = await startHarness();
		const assetId = Object.keys(h.server.document.assets)[0] as string;
		scripted(h, () => ({
			status: 499,
			body: {
				code: "E_CLIENT_CLOSED",
				message: "the request was closed",
				retryable: false,
			},
		}));
		const actions = createActions(h.runtime);
		await actions.addToTimeline(assetId);
		expect(h.runtime.notices.getState().notices).toHaveLength(0);
	});

	test("an overlap leaves the read model alone and offers the next free position", async () => {
		const h = await startHarness();
		const assetId = Object.keys(h.server.document.assets)[0] as string;
		const sent = scripted(h, (_body, all) =>
			all.length === 1
				? {
						status: 409,
						body: {
							code: "E_OVERLAP",
							message: "frames 0 to 89 are occupied",
							hint: "use start 120f",
							retryable: false,
							details: { next_free: 120 },
						},
					}
				: {
						status: 200,
						body: h.server.commit([bump(h)]).result,
					},
		);
		const actions = createActions(h.runtime);
		const before = h.runtime.documents.getState();
		await actions.addToTimeline(assetId);
		expect(h.runtime.documents.getState().document).toBe(before.document);
		const [notice] = h.runtime.notices.getState().notices;
		expect(notice?.code).toBe("E_OVERLAP");
		expect(notice?.message).toBe("frames 0 to 89 are occupied");
		await actions.applyNotice(notice as NonNullable<typeof notice>);
		expect(sent).toHaveLength(2);
		expect(sent[1]?.request.operations[0]?.args.start).toBe("120f");
		expect(sent[1]?.request.idempotency_key).not.toBe(
			sent[0]?.request.idempotency_key,
		);
	});

	test("a conflicted gesture is sent once with the pointer-down revision, names the actor and applies again on the user's action only", async () => {
		const h = await startHarness();
		const item = firstItem(h);
		const down = h.runtime.documents.getState().revision ?? 0;
		const sent = scripted(h, (_request, all) =>
			all.length === 1
				? {
						status: 409,
						body: {
							code: "E_REVISION_CONFLICT",
							message: "changed",
							retryable: true,
							details: {
								base_revision: down,
								current_revision: down,
								conflict: {
									collection: "items",
									id: item.id,
									revision: down,
									actor: "local:claude-code",
								},
								diff: "r1 local:claude-code\n~ item",
							},
						},
					}
				: { status: 200, body: h.server.commit([bump(h)]).result },
		);
		const actions = createActions(h.runtime);
		await actions.commitGesture(moveOperations(item.id, "5s"), down);
		await flush();
		expect(sent).toHaveLength(1);
		expect(sent[0]?.request.base_revision).toBe(down);
		const [notice] = h.runtime.notices.getState().notices;
		expect(notice?.conflict).toMatchObject({
			actor: "local:claude-code",
			itemId: item.id,
			diff: "r1 local:claude-code\n~ item",
		});
		expect(sent).toHaveLength(1);
		const current = h.runtime.documents.getState().revision ?? 0;
		await actions.applyNotice(notice as NonNullable<typeof notice>);
		expect(sent).toHaveLength(2);
		expect(sent[1]?.request.idempotency_key).not.toBe(
			sent[0]?.request.idempotency_key,
		);
		expect(sent[1]?.request.base_revision).toBe(current);
	});

	test("an inspector field leaves a validation error to the field and sends no notice", async () => {
		const h = await startHarness();
		const item = firstItem(h);
		scripted(h, () => ({
			status: 400,
			body: {
				code: "E_INVALID_TIME",
				message: "not a time",
				retryable: false,
			},
		}));
		const outcome = await createActions(h.runtime).commitField(
			moveOperations(item.id, "5s"),
		);
		expect(outcome?.ok).toBe(false);
		expect(h.runtime.notices.getState().notices).toHaveLength(0);
	});

	test("an overlap from a timing field offers the next free start", async () => {
		const h = await startHarness();
		const item = firstItem(h);
		const sent = scripted(h, (_body, all) =>
			all.length === 1
				? {
						status: 409,
						body: {
							code: "E_OVERLAP",
							message: "occupied",
							retryable: false,
							details: { next_free: 240 },
						},
					}
				: { status: 200, body: h.server.commit([bump(h)]).result },
		);
		const actions = createActions(h.runtime);
		await actions.commitField(moveOperations(item.id, "5s"));
		const [notice] = h.runtime.notices.getState().notices;
		expect(notice?.code).toBe("E_OVERLAP");
		await actions.applyNotice(notice as NonNullable<typeof notice>);
		expect(sent[1]?.request.operations).toEqual([
			{ op: "item.move", args: { item_id: item.id, start: "240f" } },
		]);
	});

	test("the field builders send the documented arguments", () => {
		expect(renameOperations("itm_a", "")).toEqual([
			{ op: "item.rename", args: { item_id: "itm_a", name: "" } },
		]);
		expect(trimEndOperations("itm_a", "3s")).toEqual([
			{ op: "item.trim", args: { item_id: "itm_a", edge: "end", to: "3s" } },
		]);
	});

	test("a revision conflict offers Apply again on the current revision", async () => {
		const h = await startHarness();
		const item = firstItem(h);
		scripted(h, () => ({
			status: 409,
			body: {
				code: "E_REVISION_CONFLICT",
				message: "changed by local:claude-code",
				retryable: false,
			},
		}));
		h.runtime.uiWriter.select([item.id]);
		h.runtime.uiWriter.setPlayhead(item.start + 10);
		await createActions(h.runtime).splitAtPlayhead();
		const [notice] = h.runtime.notices.getState().notices;
		expect(notice?.action?.labelKey).toBe("ui.editor.notice.apply_again");
	});

	test("undo with nothing to undo disables the button until the next batch", async () => {
		const h = await startHarness();
		h.server.routes.set(`POST /projects/${h.server.projectId}/undo`, () => ({
			status: 409,
			body: { code: "E_NOTHING_TO_UNDO", message: "nothing", retryable: false },
		}));
		await createActions(h.runtime).undo();
		expect(h.runtime.ui.getState().historyBlocked.undo).toBe(
			h.runtime.documents.getState().revision,
		);
		expect(h.runtime.notices.getState().notices).toHaveLength(0);
	});
});

describe("library and jobs from events", () => {
	const entry = (state: MediaEntry["state"], progress: number): MediaEntry => ({
		asset_id: "ast_new",
		name: "clip.mp4",
		kind: "video",
		state,
		progress,
		duration_frames: null,
		duration_seconds: null,
		has_audio: true,
		trashed: false,
	});

	test("boot lists the media, asset.updated patches the card, ready refetches", async () => {
		const h = await startHarness();
		let listed = entry("ingesting", 0.1);
		h.server.routes.set(`GET /projects/${h.server.projectId}/media`, () => ({
			status: 200,
			body: {
				project_id: h.server.projectId,
				assets: [listed],
				next_cursor: null,
				text: "",
			},
		}));
		await h.runtime.start();
		expect(h.runtime.library.getState().entries[0]?.state).toBe("ingesting");
		h.server.deliver(
			{
				project_id: h.server.projectId,
				asset_id: "ast_new",
				state: "ingesting",
				progress: 0.6,
			},
			"asset.updated",
		);
		expect(h.runtime.library.getState().entries[0]?.progress).toBe(0.6);
		listed = entry("ready", 1);
		h.server.deliver(
			{
				project_id: h.server.projectId,
				asset_id: "ast_new",
				state: "ready",
				progress: 1,
			},
			"asset.updated",
		);
		await flush();
		expect(h.runtime.library.getState().entries[0]?.state).toBe("ready");
	});

	test("importing several lines sends one request each", async () => {
		const h = await startHarness();
		const paths: string[] = [];
		h.server.routes.set(
			`POST /projects/${h.server.projectId}/media`,
			(request) => {
				const body = request.body as { files: { path: string }[] };
				paths.push(...body.files.map((f) => f.path));
				return {
					status: 200,
					body: {
						project_id: h.server.projectId,
						revision: 0,
						assets: [],
						failed: 0,
						text: "",
					},
				};
			},
		);
		h.server.routes.set(`GET /projects/${h.server.projectId}/media`, () => ({
			status: 200,
			body: {
				project_id: h.server.projectId,
				assets: [],
				next_cursor: null,
				text: "",
			},
		}));
		await h.runtime.importPaths(["/a.mp4", "/b.mp4"]);
		expect(paths).toEqual(["/a.mp4", "/b.mp4"]);
	});

	test("job.updated moves the progress of a known job", async () => {
		const h = await startHarness();
		const base = {
			job_id: "job_1",
			kind: "export",
			project_id: h.server.projectId,
			state: "running" as const,
			progress: 0.1,
			actor: "local:ui",
			created_at: "2026-11-14T12:00:00Z",
			text: "",
		};
		h.server.routes.set(`GET /projects/${h.server.projectId}/jobs`, () => ({
			status: 200,
			body: {
				project_id: h.server.projectId,
				jobs: [base],
				next_cursor: null,
				text: "",
			},
		}));
		h.server.routes.set(`GET /projects/${h.server.projectId}/media`, () => ({
			status: 200,
			body: {
				project_id: h.server.projectId,
				assets: [],
				next_cursor: null,
				text: "",
			},
		}));
		h.server.routes.set("GET /jobs/job_1", () => ({
			status: 200,
			body: { ...base, state: "succeeded", progress: 1 },
		}));
		await h.runtime.start();
		expect(h.runtime.jobs.getState().exportJobId).toBe("job_1");
		h.server.deliver(
			{
				project_id: h.server.projectId,
				job_id: "job_1",
				kind: "export",
				state: "running",
				progress: 0.5,
			},
			"job.updated",
		);
		expect(h.runtime.jobs.getState().jobs.job_1?.progress).toBe(0.5);
	});
});
