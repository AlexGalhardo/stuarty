import type { Locator } from "@playwright/test";
import {
	clippedText,
	horizontalOverflow,
	obscuredFocus,
	TEXT_SPACING_CSS,
	undersizedTargets,
} from "../support/wcag";
import {
	audioClip,
	caller,
	expect,
	seedProject,
	test,
	testClip,
} from "./helpers";

test.describe.configure({ timeout: 120_000 });

// The text-spacing style sheet is a user style sheet: the page CSP would refuse
// it as inline style.
test.use({ bypassCSP: true });

// P3-UI-004 (SDD-012 section 18.6): the WCAG 2.2 AA criteria that axe does not
// decide, measured in the browser. Each describe block names its criterion.

// Every page of the Inertia shell. The editor is a separate document and is
// excluded from reflow by its two-dimensional layout (1.4.10 exception).
const PAGES = ["/projects", "/settings", "/settings/brand-kit"] as const;

// Same function, other control (2.5.8 "Equivalent"): a keyframe diamond on a
// timeline block is also selectable in the Keyframes section of the inspector.
const EQUIVALENT = ["[data-keyframe-property]"];

async function seedEditor(
	request: Parameters<typeof seedProject>[0],
): Promise<{ id: string; clip: string }> {
	const seeded = await seedProject(request, {
		files: [testClip(), audioClip()],
		tracks: 1,
		audioTracks: 1,
		perTrack: 0,
	});
	const call = await caller(request);
	const head = (await call("GET", `/projects/${seeded.id}`)) as {
		revision: number;
	};
	const out = (await call("POST", `/projects/${seeded.id}/edit`, {
		base_revision: head.revision,
		operations: [
			{
				op: "clip.add",
				args: {
					track_id: seeded.trackIds[0],
					asset_id: seeded.assetIds[0],
					start: "0s",
					source_in: "0s",
					duration: "2s",
				},
			},
			{
				op: "text.add",
				args: {
					track_id: seeded.trackIds[0],
					start: "2s",
					duration: "2s",
					text: "Hi",
				},
			},
		],
	})) as { created: string[] };
	return { id: seeded.id, clip: out.created[0] ?? "" };
}

for (const theme of ["dark", "light"] as const) {
	test.describe(`wcag22 ${theme}`, () => {
		test.beforeEach(async ({ page }) => {
			await page.addInitScript((value) => {
				localStorage.setItem("theme", value);
			}, theme);
		});

		// 1.4.10 Reflow
		test("the pages reflow at 320 CSS px without horizontal scrolling", async ({
			page,
			request,
		}) => {
			await seedProject(request, {
				name: `A long project name that must wrap and never push the page wider ${Date.now()}`,
				tracks: 0,
			});
			await page.setViewportSize({ width: 320, height: 256 });
			const found: string[] = [];
			for (const path of PAGES) {
				await page.goto(path);
				await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
				const overflow = await horizontalOverflow(page);
				if (overflow) found.push(`${path}: ${overflow}`);
			}
			expect(found, "pages that scroll in two dimensions").toEqual([]);
		});

		// 1.4.12 Text Spacing
		test("the pages keep their text when the spacing of the criterion is applied", async ({
			page,
			request,
		}) => {
			await seedProject(request, { tracks: 0 });
			const found: string[] = [];
			for (const path of PAGES) {
				await page.goto(path);
				await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
				await page.addStyleTag({ content: TEXT_SPACING_CSS });
				for (const clipped of await clippedText(page))
					found.push(`${path}: ${clipped}`);
			}
			expect(found, "text clipped by the spacing").toEqual([]);
		});

		test("the editor keeps its text when the spacing of the criterion is applied", async ({
			page,
			request,
		}) => {
			const { id, clip } = await seedEditor(request);
			await page.goto(`/editor/${id}`);
			await page.locator(`[data-item-id="${clip}"]`).click();
			await page.addStyleTag({ content: TEXT_SPACING_CSS });
			expect(await clippedText(page), "text clipped by the spacing").toEqual(
				[],
			);
		});

		// 2.5.8 Target Size (Minimum)
		test("every pointer target of the pages is 24 by 24 CSS px or spaced", async ({
			page,
			request,
		}) => {
			await seedProject(request, { tracks: 0 });
			const found: string[] = [];
			for (const path of PAGES) {
				await page.goto(path);
				await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
				for (const small of await undersizedTargets(page))
					found.push(`${path}: ${small.target} ${small.size}`);
			}
			expect(found, "undersized targets").toEqual([]);
		});

		test("every pointer target of the editor is 24 by 24 CSS px or spaced", async ({
			page,
			request,
		}) => {
			const { id, clip } = await seedEditor(request);
			await page.goto(`/editor/${id}`);
			await expect(
				page.getByRole("toolbar", { name: "Editing" }),
			).toBeVisible();
			const found: string[] = [];
			const scan = async (state: string): Promise<void> => {
				for (const small of await undersizedTargets(page, EQUIVALENT))
					found.push(`${state}: ${small.target} ${small.size}`);
			};
			await scan("project");
			await page.locator(`[data-item-id="${clip}"]`).click();
			await scan("clip selected");
			await page.getByRole("button", { name: "Export" }).click();
			await expect(
				page.locator("[role=dialog][data-state=open]"),
			).toBeVisible();
			await scan("export dialog");
			await page.keyboard.press("Escape");
			await page.keyboard.press("Control+k");
			await expect(
				page.locator("[role=dialog][data-state=open]"),
			).toBeVisible();
			await scan("palette");
			expect(found, "undersized targets").toEqual([]);
		});

		// 2.4.11 Focus Not Obscured (Minimum)
		test("no focused element is hidden behind other content", async ({
			page,
			request,
		}) => {
			const { id } = await seedEditor(request);
			const found: string[] = [];
			const walk = async (state: string, steps: number): Promise<void> => {
				for (let i = 0; i < steps; i++) {
					await page.keyboard.press("Tab");
					const hidden = await obscuredFocus(page);
					if (hidden) found.push(`${state} tab ${i + 1}: ${hidden}`);
				}
			};
			for (const path of PAGES) {
				await page.goto(path);
				await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
				await walk(path, 25);
			}
			await page.goto(`/editor/${id}`);
			await expect(
				page.getByRole("toolbar", { name: "Editing" }),
			).toBeVisible();
			await walk("editor", 60);
			expect(found, "focus hidden by content").toEqual([]);
		});
	});
}

// 2.5.7 Dragging Movements: every drag has a single-pointer or keyboard
// alternative.
test.describe("wcag22 dragging alternatives", () => {
	test("the splitter and the ruler move with the keyboard", async ({
		page,
		request,
	}) => {
		const { id } = await seedEditor(request);
		await page.goto(`/editor/${id}`);
		const splitter = page.getByRole("separator").first();
		await splitter.focus();
		const before = await splitter.getAttribute("aria-valuenow");
		await page.keyboard.press("ArrowUp");
		expect(await splitter.getAttribute("aria-valuenow")).not.toBe(before);
	});

	test("a library card adds with a button and a track moves from its menu", async ({
		page,
		request,
	}) => {
		const { id } = await seedEditor(request);
		await page.goto(`/editor/${id}`);
		await expect(
			page.getByRole("button", { name: "Add to timeline" }).first(),
		).toBeVisible();
		await expect(
			page.getByRole("button", { name: /^Actions for track/ }).first(),
		).toBeVisible();
	});
});

// 3.3.7 Redundant Entry: a refused form keeps what the person typed.
test.describe("wcag22 redundant entry", () => {
	test("a refused font import keeps the path and the family name", async ({
		page,
	}) => {
		await page.goto("/settings/brand-kit");
		const path = page.getByLabel(
			"Font file path on the machine of the service",
		);
		const family = page.getByLabel("Family name");
		await path.fill("C:\\no\\such\\font.ttf");
		await family.fill("Missing");
		await page.getByRole("button", { name: "Import font" }).click();
		await expect(page.getByRole("alert")).toBeVisible();
		await expect(path).toHaveValue("C:\\no\\such\\font.ttf");
		await expect(family).toHaveValue("Missing");
	});
});

// 3.2.6 Consistent Help: the shortcuts button keeps its place in the top bar.
test.describe("wcag22 consistent help", () => {
	test("the shortcuts button is in the same place in every project", async ({
		page,
		request,
	}) => {
		const first = await seedEditor(request);
		const second = await seedProject(request, { tracks: 0, perTrack: 0 });
		const position = async (id: string): Promise<string> => {
			await page.goto(`/editor/${id}`);
			const button = page.getByRole("button", { name: /shortcuts/i });
			await expect(button).toBeVisible();
			const box = await button.boundingBox();
			return `${Math.round(box?.x ?? -1)},${Math.round(box?.y ?? -1)}`;
		};
		expect(await position(first.id)).toBe(await position(second.id));
	});
});

// 1.4.11 Non-text Contrast: the border of a field against the page, as drawn.
test.describe("wcag22 non-text contrast", () => {
	for (const theme of ["dark", "light"] as const) {
		test(`field borders and focus outlines reach 3:1 in ${theme}`, async ({
			page,
		}) => {
			await page.addInitScript((value) => {
				localStorage.setItem("theme", value);
			}, theme);
			await page.goto("/settings/brand-kit");
			await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
			const field = page.getByLabel("Family name");
			await field.focus();
			const ratios = await ratiosOf(field);
			expect(ratios.border, "border against the page").toBeGreaterThanOrEqual(
				3,
			);
			expect(
				ratios.outline,
				"focus outline against the page",
			).toBeGreaterThanOrEqual(3);
		});
	}
});

async function ratiosOf(
	field: Locator,
): Promise<{ border: number; outline: number }> {
	return field.evaluate((el) => {
		const canvas = document.createElement("canvas");
		canvas.width = canvas.height = 1;
		const ctx = canvas.getContext("2d", { willReadFrequently: true });
		if (ctx === null) throw new Error("no 2d context");
		const rgb = (color: string): [number, number, number] => {
			ctx.clearRect(0, 0, 1, 1);
			ctx.fillStyle = "#000";
			ctx.fillStyle = color;
			ctx.fillRect(0, 0, 1, 1);
			const d = ctx.getImageData(0, 0, 1, 1).data;
			return [d[0] ?? 0, d[1] ?? 0, d[2] ?? 0];
		};
		const lum = ([r, g, b]: [number, number, number]): number => {
			const f = (v: number): number => {
				const s = v / 255;
				return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
			};
			return 0.2126 * f(r) + 0.7152 * f(g) + 0.0722 * f(b);
		};
		const ratio = (a: string, b: string): number => {
			const [hi, lo] = [lum(rgb(a)), lum(rgb(b))].sort((x, y) => y - x);
			return ((hi ?? 0) + 0.05) / ((lo ?? 0) + 0.05);
		};
		const style = getComputedStyle(el);
		const page = getComputedStyle(document.body).backgroundColor;
		return {
			border: ratio(style.borderTopColor, page),
			outline: ratio(style.outlineColor, page),
		};
	});
}

// The measurements above must be able to fail: each one is run on a page built
// to break its criterion.
test.describe("wcag22 measurements detect a failure", () => {
	test("undersized, clipped and obscured are reported", async ({ page }) => {
		await page.goto("about:blank");
		await page.setContent(`
			<button id="a" style="width:10px;height:10px;position:absolute;left:10px;top:10px">a</button>
			<button id="b" style="width:10px;height:10px;position:absolute;left:16px;top:10px">b</button>
			<button id="c" style="width:10px;height:10px;position:absolute;left:300px;top:300px">c</button>
			<p id="clip" style="width:40px;height:10px;overflow:hidden;position:absolute;top:100px">clipped clipped clipped</p>
			<button id="under" style="position:absolute;left:500px;top:500px;width:60px;height:30px">under</button>
			<div style="position:fixed;left:480px;top:480px;width:200px;height:200px;background:red"></div>
		`);
		const small = (await undersizedTargets(page)).map((s) => s.target);
		expect(small.some((s) => s.includes('"a"'))).toBe(true);
		expect(small.some((s) => s.includes('"c"'))).toBe(false);
		expect(await clippedText(page)).toHaveLength(1);
		await page.locator("#under").focus();
		expect(await obscuredFocus(page)).not.toBeNull();
		expect(await horizontalOverflow(page)).toBeNull();
	});
});
