import type { Page } from "@playwright/test";
import { NO_VIDEO_DECODE_REASON, noVideoDecode } from "../support/browsers";
import { caller, expect, seedProject, solidClip, test } from "./helpers";

type Rgb = [number, number, number];
type Operation = { op: string; args: Record<string, unknown> };

// The colour of the canvas buffer at a fraction of its width and height, once
// the preview says it drew `frame`.
async function sampleAt(
	page: Page,
	frame: number,
	points: [number, number][],
): Promise<Rgb[]> {
	const canvas = page.getByRole("main", { name: "Preview" }).locator("canvas");
	await expect(canvas).toHaveAttribute("data-drawn", String(frame), {
		timeout: 20_000,
	});
	return canvas.evaluate(async (el, at) => {
		const source = el as HTMLCanvasElement;
		const copy = document.createElement("canvas");
		copy.width = source.width;
		copy.height = source.height;
		const ctx = copy.getContext("2d");
		if (ctx === null) throw new Error("no 2d context");
		const image = new Image();
		image.src = source.toDataURL("image/png");
		await image.decode();
		ctx.drawImage(image, 0, 0);
		return at.map(([fx, fy]) => {
			const d = ctx.getImageData(
				Math.floor(fx * copy.width),
				Math.floor(fy * copy.height),
				1,
				1,
			).data;
			return [d[0] ?? 0, d[1] ?? 0, d[2] ?? 0] as Rgb;
		});
	}, points);
}

const distance = (a: Rgb, b: Rgb): number =>
	Math.abs(a[0] - b[0]) + Math.abs(a[1] - b[1]) + Math.abs(a[2] - b[2]);

// A green clip on a project whose background is the default: the preview samples
// of one frame, with the operations applied first.
async function seeded(
	request: Parameters<typeof caller>[0],
	operations: (itemId: string) => Operation[],
): Promise<{ id: string }> {
	const project = await seedProject(request, {
		tracks: 1,
		perTrack: 1,
		clipDuration: "1s",
		files: [solidClip("green")],
	});
	const call = await caller(request);
	const head = (await call("GET", `/projects/${project.id}`)) as {
		revision: number;
	};
	const ops = operations(project.itemIds[0] ?? "");
	if (ops.length > 0) {
		await call("POST", `/projects/${project.id}/edit`, {
			base_revision: head.revision,
			operations: ops,
		});
	}
	return project;
}

async function open(page: Page, id: string, frame: number): Promise<void> {
	await page.goto(`/editor/${id}?t=${frame}f`);
}

const CENTER: [number, number] = [0.5, 0.5];

// TASK-0280 (SDD-012 section 11.8, SDD-004 sections 3.2 and 3.13): the preview
// draws the pixel effects and the transitions of the D-040 features from what
// the plan says.
test.describe("effects-preview", () => {
	test.beforeEach(({ browserName }) => {
		test.skip(noVideoDecode(browserName), NO_VIDEO_DECODE_REASON);
	});

	test("a shader that does not compile would show here", async ({
		page,
		request,
	}) => {
		const errors: string[] = [];
		page.on("console", (message) => {
			if (message.type() === "error" || message.type() === "warning") {
				errors.push(message.text());
			}
		});
		const project = await seeded(request, (item) => [
			{
				op: "clip.set_effects",
				args: {
					item_id: item,
					effects: [
						{ kind: "motion_blur" },
						{ kind: "glow" },
						{ kind: "vignette" },
						{ kind: "grain" },
						{ kind: "rgb_split" },
						{ kind: "shake" },
					],
				},
			},
		]);
		await open(page, project.id, 0);
		await sampleAt(page, 0, [CENTER]);
		expect(
			errors.filter((e) =>
				/could not initialize shader|not compiled|invalid_operation|wgsl/i.test(
					e,
				),
			),
		).toEqual([]);
	});

	test("a vignette darkens the edges of the layer and leaves the centre", async ({
		page,
		request,
	}) => {
		const project = await seeded(request, (item) => [
			{
				op: "clip.set_effects",
				args: {
					item_id: item,
					effects: [{ kind: "vignette", amount: 1, softness: 1 }],
				},
			},
		]);
		await open(page, project.id, 0);
		// A clip fitted to cover the frame is wider than it, so only the top and
		// bottom edges of the frame are the edges of the layer.
		const [centre, edge] = (await sampleAt(page, 0, [CENTER, [0.5, 0.02]])) as [
			Rgb,
			Rgb,
		];
		expect(centre[1]).toBeGreaterThan(100);
		expect(edge[1]).toBeLessThan(centre[1] * 0.7);
	});

	test("the grain changes pixels of a flat colour", async ({
		page,
		request,
	}) => {
		const project = await seeded(request, (item) => [
			{
				op: "clip.set_effects",
				args: {
					item_id: item,
					effects: [{ kind: "grain", amount: 1, size: 1 }],
				},
			},
		]);
		await open(page, project.id, 0);
		const points: [number, number][] = Array.from({ length: 12 }, (_, i) => [
			0.4 + i * 0.013,
			0.5,
		]);
		const samples = await sampleAt(page, 0, points);
		expect(new Set(samples.map((s) => s.join(","))).size).toBeGreaterThan(3);
	});

	test("a motion blur spreads a shrunken clip past its edge", async ({
		page,
		request,
	}) => {
		const project = await seeded(request, (item) => [
			{ op: "item.set_transform", args: { item_id: item, scale: 0.4 } },
			{
				op: "clip.set_effects",
				args: {
					item_id: item,
					effects: [{ kind: "motion_blur", amount: 1, angle: 90 }],
				},
			},
		]);
		const plain = await seeded(request, (item) => [
			{ op: "item.set_transform", args: { item_id: item, scale: 0.4 } },
		]);
		// Just above the top edge of the layer, which is 0.4 of the frame high and
		// centred, so its top edge is at 0.3.
		const outside: [number, number] = [0.5, 0.295];
		await open(page, plain.id, 0);
		const [still] = (await sampleAt(page, 0, [outside])) as [Rgb];
		await open(page, project.id, 0);
		const [blurred] = (await sampleAt(page, 0, [outside])) as [Rgb];
		expect(distance(blurred, still)).toBeGreaterThan(20);
	});

	test("a dip starts from the colour and ends on the clip", async ({
		page,
		request,
	}) => {
		const project = await seeded(request, (item) => [
			{
				op: "item.set_transition",
				args: {
					item_id: item,
					edge: "in",
					kind: "dip",
					color: "#0000FF",
					duration: "0.5s",
				},
			},
		]);
		await open(page, project.id, 0);
		const [start] = (await sampleAt(page, 0, [CENTER])) as [Rgb];
		expect(start[2]).toBeGreaterThan(start[1] + 100);
		await open(page, project.id, 14);
		const [end] = (await sampleAt(page, 14, [CENTER])) as [Rgb];
		expect(end[1]).toBeGreaterThan(end[2] + 80);
	});

	test("a wipe to the left reveals the left side first", async ({
		page,
		request,
	}) => {
		const project = await seeded(request, (item) => [
			{
				op: "item.set_transition",
				args: {
					item_id: item,
					edge: "in",
					kind: "wipe",
					direction: "left",
					duration: "0.5s",
				},
			},
		]);
		await open(page, project.id, 7);
		const [left, right] = (await sampleAt(page, 7, [
			[0.15, 0.5],
			[0.85, 0.5],
		])) as [Rgb, Rgb];
		expect(left[1]).toBeGreaterThan(100);
		expect(distance(right, left)).toBeGreaterThan(80);
	});

	test("a zoom and a whip settle on the clip at the end", async ({
		page,
		request,
	}) => {
		for (const kind of ["zoom", "whip"]) {
			const project = await seeded(request, (item) => [
				{ op: "item.set_transform", args: { item_id: item, scale: 0.4 } },
				{
					op: "item.set_transition",
					args: { item_id: item, edge: "in", kind, duration: "0.5s" },
				},
			]);
			await open(page, project.id, 0);
			const [start] = (await sampleAt(page, 0, [CENTER])) as [Rgb];
			await open(page, project.id, 14);
			const [end] = (await sampleAt(page, 14, [CENTER])) as [Rgb];
			expect(end[1], `${kind} at the end`).toBeGreaterThan(100);
			expect(distance(start, end), `${kind} at the start`).toBeGreaterThan(50);
		}
	});

	test("a whip blurs along its direction past the edge of the layer", async ({
		page,
		request,
	}) => {
		const project = await seeded(request, (item) => [
			{ op: "item.set_transform", args: { item_id: item, scale: 0.4 } },
			{
				op: "item.set_transition",
				args: {
					item_id: item,
					edge: "in",
					kind: "whip",
					direction: "up",
					duration: "0.5s",
				},
			},
		]);
		// At the middle of the whip the layer is 0.03 of the frame above where it
		// rests, so its top edge is at 0.269 of the frame height.
		await open(page, project.id, 7);
		const samples = await sampleAt(page, 7, [
			[0.5, 0.2663],
			[0.5, 0.2626],
			[0.5, 0.2701],
		]);
		expect(samples.some((s) => s[1] > 10)).toBe(true);
		const [inside] = (await sampleAt(page, 7, [[0.5, 0.5]])) as [Rgb];
		expect(inside[1]).toBeGreaterThan(100);
	});
});
