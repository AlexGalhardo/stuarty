import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { APIRequestContext } from "@playwright/test";
import { caller, expect, ffmpegExe, seedProject } from "../specs/helpers";

// A project whose timeline holds a composition drawn as a group (epic 52): two
// clips on two tracks, the upper one smaller, moved into a nested timeline, and
// the composition given an opacity and a blend, so it is one unit with a look of
// its own (SDD-004 section 3.16). With `levels` of 2 the nested timeline's own
// items become a composition too, with an opacity of its own, so the group holds
// a group (SDD-013 section 3.12).
export interface NestedProject {
	id: string;
	// The composition item of the project's own timeline.
	composition: string;
	frames: number[];
}

type Call = Awaited<ReturnType<typeof caller>>;

async function edit(
	call: Call,
	timeline: string,
	operations: unknown[],
): Promise<{ created: string[] }> {
	const head = (await call("GET", `/projects/${timeline}`)) as {
		revision: number;
	};
	const out = (await call("POST", `/projects/${timeline}/edit`, {
		timeline_id: timeline,
		base_revision: head.revision,
		operations,
	})) as { effective: { created?: string[] }[] };
	return { created: out.effective.flatMap((e) => e.created ?? []) };
}

async function compositionOf(
	call: Call,
	timeline: string,
): Promise<{ id: string; timeline: string }> {
	const doc = (await call(
		"GET",
		`/projects/${timeline}?detail=document&timeline_id=${timeline}`,
	)) as {
		document: {
			items: Record<string, { type: string; timeline?: string }>;
		};
	};
	const found = Object.entries(doc.document.items).find(
		([, item]) => item.type === "composition",
	);
	if (found === undefined || found[1].timeline === undefined) {
		throw new Error(`no composition in ${timeline}`);
	}
	return { id: found[0], timeline: found[1].timeline };
}

export async function nestedProject(
	request: APIRequestContext,
	options: { levels: 1 | 2; blend?: string } = { levels: 1 },
): Promise<NestedProject> {
	const call = await caller(request);
	const seeded = await seedProject(request, {
		tracks: 2,
		perTrack: 1,
		clipDuration: "4s",
		name: `Nested live ${Date.now()}`,
	});
	const [lower, upper] = seeded.itemIds;
	if (lower === undefined || upper === undefined) throw new Error("no clips");

	await edit(call, seeded.id, [
		{
			op: "composition.create_from_selection",
			args: { item_ids: [lower, upper] },
		},
	]);
	const outer = await compositionOf(call, seeded.id);

	if (options.levels === 2) {
		// The nested timeline holds its items as one more composition, at 80 %.
		const nested = (await call(
			"GET",
			`/projects/${outer.timeline}?detail=document&timeline_id=${outer.timeline}`,
		)) as { document: { items: Record<string, unknown> } };
		await edit(call, outer.timeline, [
			{
				op: "composition.create_from_selection",
				args: { item_ids: Object.keys(nested.document.items) },
			},
		]);
		const inner = await compositionOf(call, outer.timeline);
		await edit(call, outer.timeline, [
			{ op: "item.set_transform", args: { item_id: inner.id, opacity: 0.8 } },
		]);
		// The upper clip of the innermost timeline is smaller, so both layers show.
		const innermost = (await call(
			"GET",
			`/projects/${inner.timeline}?detail=document&timeline_id=${inner.timeline}`,
		)) as { document: { items: Record<string, { track_id: string }> } };
		const ids = Object.keys(innermost.document.items);
		const second = ids[1];
		if (second !== undefined) {
			await edit(call, inner.timeline, [
				{ op: "item.set_transform", args: { item_id: second, scale: 0.5 } },
			]);
		}
	} else {
		const nested = (await call(
			"GET",
			`/projects/${outer.timeline}?detail=document&timeline_id=${outer.timeline}`,
		)) as { document: { items: Record<string, unknown> } };
		const second = Object.keys(nested.document.items)[1];
		if (second !== undefined) {
			await edit(call, outer.timeline, [
				{ op: "item.set_transform", args: { item_id: second, scale: 0.5 } },
			]);
		}
	}

	await edit(call, seeded.id, [
		{ op: "item.set_transform", args: { item_id: outer.id, opacity: 0.5 } },
		{
			op: "clip.set_blend",
			args: { item_id: outer.id, mode: options.blend ?? "screen" },
		},
	]);
	return { id: seeded.id, composition: outer.id, frames: [0, 20, 45, 90] };
}

// A 1280x720 picture with sharp edges, wider than the 640 of the poster the
// preview draws a still from, made once per run.
function widePicture(): string {
	const dir = join(tmpdir(), "stuarty-e2e-media");
	mkdirSync(dir, { recursive: true });
	const out = join(dir, "picture-1280.png");
	if (!existsSync(out)) {
		execFileSync(
			ffmpegExe(),
			[
				"-y",
				"-f",
				"lavfi",
				"-i",
				"testsrc2=size=1280x720:rate=1",
				"-frames:v",
				"1",
				out,
			],
			{ stdio: "ignore" },
		);
	}
	return out;
}

// A project whose timeline holds, as a group at 50 %, another timeline of the
// same production (clip.add_composition), and that timeline holds a picture
// wider than the poster. The asset is in the nested timeline's document only,
// so the preview asks that timeline for its size before it cuts the poster
// (preview/sourceSizes.ts). The picture has edges, so the displacement of the
// group is measured too.
export async function pictureGroupProject(
	request: APIRequestContext,
): Promise<NestedProject> {
	const call = await caller(request);
	const created = (await call("POST", "/projects", {
		name: `Picture group ${Date.now()}`,
	})) as { project_id: string };
	const id = created.project_id;
	const added = (await call("POST", `/projects/${id}/items`, {
		kind: "timeline",
		title: "Picture",
	})) as { item: { timeline_id: string } };
	const nested = added.item.timeline_id;

	const imported = (await call("POST", `/projects/${nested}/media`, {
		files: [{ path: widePicture() }],
		wait_ms: 20000,
	})) as { assets: { asset_id: string }[] };
	const asset = imported.assets[0]?.asset_id;
	if (asset === undefined) throw new Error("no picture after import");
	// The import may answer before the asset is in the document (see seedProject).
	await expect
		.poll(
			async () => {
				const doc = (await call(
					"GET",
					`/projects/${nested}?detail=document&timeline_id=${nested}`,
				)) as { document?: { assets?: Record<string, unknown> } };
				return doc.document?.assets?.[asset] !== undefined;
			},
			{ message: "the picture never reached the document", timeout: 60_000 },
		)
		.toBe(true);

	const [nestedTrack] = (
		await edit(call, nested, [
			{ op: "track.add", args: { kind: "visual", name: "Picture" } },
		])
	).created;
	await edit(call, nested, [
		{
			op: "clip.add",
			args: {
				track_id: nestedTrack,
				asset_id: asset,
				start: "0s",
				source_in: "0s",
				duration: "4s",
			},
		},
	]);
	const [track] = (
		await edit(call, id, [
			{ op: "track.add", args: { kind: "visual", name: "Video 1" } },
		])
	).created;
	const [composition] = (
		await edit(call, id, [
			{
				op: "clip.add_composition",
				args: { track_id: track, timeline_id: nested, start: "0s" },
			},
		])
	).created;
	if (composition === undefined) throw new Error("no composition");
	await edit(call, id, [
		{
			op: "item.set_transform",
			args: { item_id: composition, opacity: 0.5 },
		},
	]);
	return { id, composition, frames: [0, 20] };
}
