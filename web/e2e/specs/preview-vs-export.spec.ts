import { NO_VIDEO_DECODE_REASON, noVideoDecode } from "../support/browsers";
import {
	capture,
	compareFrame,
	goldenCases,
	loadCase,
	MAX_DISPLACEMENT,
	MIN_SSIM,
	needsImageFixtures,
	openCase,
	stepThrough,
} from "../support/golden";
import {
	centroid,
	chromaCentroid,
	globalShift,
	lumaSsim,
	type Pixels,
} from "../support/ssim";
import { caller, expect, markerClip, seedProject, test } from "./helpers";

// SDD-012 section 20.3 and SDD-004 section 11: for each golden case of
// engine/export/testdata/golden (the document, the description of its media and
// the listed frames), load the case into the workspace as a project, step the
// preview to each listed frame, capture the canvas without the truth-on-pause
// overlay, fetch ViewFrame at the width of the canvas and compare in Node:
// luma SSIM at least 0.97 and no layer displaced by more than 1 pixel.
//
// ViewFrame is requested at the width of the canvas and compared at the size of
// the canvas, so no image is resampled: the gate measures the preview. Compared
// after downscaling both to a thumbnail the SSIM of the same frame falls to 0.96.
test.describe("preview-vs-export measures", () => {
	// A grey picture with a bright box: the measures must see the box move.
	const picture = (boxX: number): Pixels => {
		const width = 96;
		const height = 96;
		const data = new Uint8Array(width * height * 4).fill(90);
		for (let row = 30; row < 60; row++) {
			for (let col = boxX; col < boxX + 30; col++) {
				const i = (row * width + col) * 4;
				data[i] = 250;
				data[i + 1] = 40;
				data[i + 2] = 40;
			}
		}
		for (let i = 3; i < data.length; i += 4) data[i] = 255;
		return { width, height, data };
	};

	test("equal pictures have SSIM 1 and no displacement", () => {
		expect(lumaSsim(picture(20), picture(20))).toBeCloseTo(1, 6);
		expect(globalShift(picture(20), picture(20)).distance).toBeLessThan(0.01);
	});

	test("a box moved by 2 pixels lowers the SSIM and is measured as 2 pixels", () => {
		expect(lumaSsim(picture(20), picture(22))).toBeLessThan(0.99);
		const shift = globalShift(picture(20), picture(22));
		expect(shift.distance).toBeGreaterThan(1.5);
		expect(shift.distance).toBeLessThan(2.5);
	});
});

// Cases that miss the gate today, measured on the reference machine (frame 0 of
// each, canvas 463 x 823 over a 180 x 320 project). Playwright reports a case
// listed here that starts to pass as a failure, so the entry is removed when the
// cause is fixed. None of them is a defect of the preview geometry.
const KNOWN_DIVERGENCES: Record<string, string> = {};

test.describe("preview-vs-export", () => {
	test.use({ viewport: { width: 1920, height: 1300 } });
	test.describe.configure({ timeout: 300_000 });
	test.beforeEach(({ browserName }) => {
		test.skip(noVideoDecode(browserName), NO_VIDEO_DECODE_REASON);
	});

	// A frame that fails is reported by case and frame.
	for (const name of goldenCases()) {
		test(`golden ${name}: SSIM of at least ${MIN_SSIM} on luma and no layer displaced by more than ${MAX_DISPLACEMENT} px`, async ({
			page,
			request,
		}) => {
			test.skip(
				needsImageFixtures(name),
				"the fixtures are an alpha PNG or a GIF with delays that only the Go suite generates",
			);
			test.fail(name in KNOWN_DIVERGENCES, KNOWN_DIVERGENCES[name]);
			const loaded = await loadCase(request, name);
			await openCase(page, loaded.id);
			await stepThrough(page, loaded.frames, async (frame) => {
				await compareFrame(page, name, loaded.id, frame, "preview-vs-export");
			});
		});
	}

	// The blend modes (VV-268): the cases above are sent without their blend, so these are loaded with
	// all their edits, over an opaque canvas and over a transparent one. PixiJS blends `multiply` as
	// `src x dst + dst x (1 - sa)`, which is black where nothing is drawn yet, and the export draws the
	// general form of SDD-013 section 3.11.2, which is the source itself there.
	for (const name of goldenCases().filter((n) => /^blend-/.test(n))) {
		test(`golden ${name} with its blend: SSIM of at least ${MIN_SSIM} on luma and no layer displaced by more than ${MAX_DISPLACEMENT} px`, async ({
			page,
			request,
		}) => {
			const loaded = await loadCase(request, name, true);
			await openCase(page, loaded.id);
			await stepThrough(page, loaded.frames, async (frame) => {
				await compareFrame(page, name, loaded.id, frame, "preview-vs-export");
			});
		});
	}

	// The golden cases blend over a backdrop that is opaque or empty, and the SSIM of the luma does not see
	// the alpha. Here a layer at opacity 0.5 is blended over another at opacity 0.5, so the backdrop has
	// `0 < da < 255`, and three points are read as premultiplied colours (what the export's JPEG holds, the
	// frame over black): the overlap, a corner of the ellipse's box where the blended layer has `a == 0`, and
	// a point outside it. The frame is drawn three times, so a canvas that keeps the pixels of the frame
	// before under a clear one is seen too (VV-268 review).
	const halfAlpha: [string, string][] = [
		["multiply", "transparent"],
		["screen", "transparent"],
		["overlay", "transparent"],
		["add", "transparent"],
		["overlay", "#204060"],
	];
	for (const [mode, background] of halfAlpha) {
		test(`${mode} at opacity 0.5 over a layer at opacity 0.5 on a ${background} canvas matches the export at three points`, async ({
			page,
			request,
		}) => {
			const call = await caller(request);
			const created = await call("POST", "/projects", {
				name: `Blend ${mode} over half alpha`,
			});
			const id = created.project_id as string;
			const { revision } = (await call("GET", `/projects/${id}`)) as {
				revision: number;
			};
			await call("POST", `/projects/${id}/edit`, {
				base_revision: revision,
				operations: [
					{
						op: "settings.set",
						args: { width: 180, height: 320, background },
					},
					{
						op: "track.add",
						args: { kind: "visual", name: "Under", id: "trk_k000000001" },
					},
					{
						op: "track.add",
						args: { kind: "visual", name: "Over", id: "trk_k000000002" },
					},
					{
						op: "shape.add",
						args: {
							track_id: "trk_k000000001",
							start: "0f",
							duration: "30f",
							shape: "rect",
							width: 180,
							height: 320,
							fill: "#C08040",
							x: 90,
							y: 160,
							id: "itm_h000000001",
						},
					},
					{
						op: "shape.add",
						args: {
							track_id: "trk_k000000002",
							start: "0f",
							duration: "30f",
							shape: "ellipse",
							width: 120,
							height: 200,
							fill: "#80C0FF",
							x: 90,
							y: 160,
							id: "itm_h000000002",
						},
					},
					{
						op: "item.set_transform",
						args: { item_id: "itm_h000000001", opacity: 0.5 },
					},
					{
						op: "item.set_transform",
						args: { item_id: "itm_h000000002", opacity: 0.5 },
					},
					{ op: "clip.set_blend", args: { item_id: "itm_h000000002", mode } },
				],
			});
			await openCase(page, id);
			// Project points: the centre, a corner of the ellipse's box (outside the ellipse), the top.
			const points: [string, number, number][] = [
				["overlap", 90, 160],
				["corner", 36, 66],
				["outside", 90, 20],
			];
			await stepThrough(page, [0, 1, 2], async (frame) => {
				const shot = await capture(page, id, frame);
				const { width, height } = shot.canvas;
				for (const [where, px, py] of points) {
					const x = Math.round((px * width) / 180);
					const y = Math.round((py * height) / 320);
					const i = (y * width + x) * 4;
					const c = shot.canvas.data;
					const a = c[i + 3] ?? 0;
					for (let k = 0; k < 3; k++) {
						const premultiplied = Math.round(((c[i + k] ?? 0) * a) / 255);
						const exported = shot.frame.data[i + k] ?? 0;
						expect(
							Math.abs(premultiplied - exported),
							`${mode} on ${background}, frame ${frame}, ${where}, channel ${k}: canvas ${premultiplied} (alpha ${a}), export ${exported}`,
						).toBeLessThanOrEqual(4);
					}
				}
			});
		});
	}
});

// The three Prototype cases of SDD-012 section 20.3 on a clip with markers: cover
// fit, contain fit and one caption. Red and blue boxes in the clip give the
// displacement check a centroid of their own: weighted by how red or blue each
// pixel is, so that the antialiased edge and the JPEG of ViewFrame do not move
// it (the centroid of the pixels over a threshold jitters by half a pixel).
const PROTOTYPE: {
	name: string;
	fit: "cover" | "contain";
	caption?: string;
}[] = [
	{ name: "cover", fit: "cover" },
	{ name: "contain", fit: "contain" },
	{ name: "caption", fit: "cover", caption: "Meet the new editor" },
];

test.describe("preview-vs-export prototype cases", () => {
	test.use({ viewport: { width: 1920, height: 1300 } });
	test.describe.configure({ timeout: 300_000 });
	test.beforeEach(({ browserName }) => {
		test.skip(noVideoDecode(browserName), NO_VIDEO_DECODE_REASON);
	});

	for (const scene of PROTOTYPE) {
		test(`${scene.name}: SSIM of at least ${MIN_SSIM} on luma and markers within ${MAX_DISPLACEMENT} px`, async ({
			page,
			request,
		}) => {
			const seeded = await seedProject(request, {
				tracks: 1,
				perTrack: 1,
				clipDuration: "5s",
				files: [markerClip(640, 360)],
			});
			const call = await caller(request);
			const head = (await call("GET", `/projects/${seeded.id}`)) as {
				revision: number;
			};
			const operations: unknown[] = [
				{
					op: "clip.set_framing",
					args: { item_id: seeded.itemIds[0], fit: scene.fit },
				},
			];
			if (scene.caption) {
				// A caption needs its own track: it would overlap the clip on theirs.
				operations.push(
					{
						op: "track.add",
						args: { kind: "visual", name: "Captions", id: "trk_c4p710n2m3" },
					},
					{
						op: "caption.add",
						args: {
							text: scene.caption,
							start: "0s",
							duration: "4s",
							track_id: "trk_c4p710n2m3",
						},
					},
				);
			}
			await call("POST", `/projects/${seeded.id}/edit`, {
				base_revision: head.revision,
				operations,
			});

			await openCase(page, seeded.id);
			await stepThrough(page, [30], async (frame) => {
				const { canvas, frame: exported } = await capture(
					page,
					seeded.id,
					frame,
				);
				const ssim = lumaSsim(canvas, exported);
				const red = (r: number, g: number, b: number): boolean =>
					r > 180 && g < 80 && b < 80;
				const blue = (r: number, g: number, b: number): boolean =>
					b > 180 && r < 80 && g < 80;
				const shifts = (
					[
						["red", red],
						["blue", blue],
					] as const
				).map(([label, hit]) => {
					const p = centroid(canvas, hit);
					const q = centroid(exported, hit);
					const wp = chromaCentroid(canvas, label);
					const wq = chromaCentroid(exported, label);
					return {
						label,
						q,
						distance: Math.hypot(p.x - q.x, p.y - q.y),
						weighted: Math.hypot(wp.x - wq.x, wp.y - wq.y),
					};
				});
				console.log(
					`preview-vs-export ${scene.name}: SSIM(luma) ${ssim.toFixed(4)}, ${shifts.map((s) => `${s.label} marker ${s.distance.toFixed(2)} px, weighted ${s.weighted.toFixed(2)} px (${s.q.count} px)`).join(", ")}`,
				);
				expect(ssim).toBeGreaterThanOrEqual(MIN_SSIM);
				for (const s of shifts) {
					expect(
						s.q.count,
						`${s.label} marker visible in ViewFrame`,
					).toBeGreaterThan(20);
					expect(
						s.weighted,
						`${s.label} marker displacement`,
					).toBeLessThanOrEqual(MAX_DISPLACEMENT);
				}
			});
		});
	}
});
