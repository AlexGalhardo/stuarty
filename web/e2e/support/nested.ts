import type { APIRequestContext } from "@playwright/test";
import { caller, seedProject } from "../specs/helpers";

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
