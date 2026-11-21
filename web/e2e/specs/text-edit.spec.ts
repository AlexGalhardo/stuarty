import type { APIRequestContext, Page } from "@playwright/test";
import { caller, expect, seedProject, test } from "./helpers";
import { openInspectorTab } from "./sections";

interface Posted {
	operations: { op: string; args: Record<string, unknown> }[];
}

function recordEdits(page: Page): Posted[] {
	const posted: Posted[] = [];
	page.on("request", (r) => {
		if (r.method() === "POST" && r.url().endsWith("/edit")) {
			posted.push(r.postDataJSON() as Posted);
		}
	});
	return posted;
}

// A project with one visual track and, through the API, one text item.
async function seedText(
	request: APIRequestContext,
	text = "Hello",
): Promise<{ id: string; itemId: string; trackId: string }> {
	const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
	const trackId = seeded.trackIds[0] as string;
	const call = await caller(request);
	const head = (await call("GET", `/projects/${seeded.id}`)) as {
		revision: number;
	};
	const out = (await call("POST", `/projects/${seeded.id}/edit`, {
		base_revision: head.revision,
		operations: [
			{
				op: "text.add",
				args: { track_id: trackId, start: "0s", duration: "3s", text },
			},
		],
	})) as { created: string[] };
	return { id: seeded.id, itemId: out.created[0] as string, trackId };
}

async function stored(
	page: Page,
	itemId: string,
): Promise<{ text?: string; style?: Record<string, unknown> }> {
	return page.evaluate((id) => {
		const doc = window.__vv?.document as {
			items: Record<string, { text?: string; style?: Record<string, unknown> }>;
		};
		return doc.items[id] ?? {};
	}, itemId);
}

// P2-UI-017 (SDD-012 sections 8.2, 11.6 and 12.1): text items are added,
// edited inline and styled in the inspector; every gesture is one request and
// the pixels of the text are a bitmap of the plan.
test.describe("text-edit", () => {
	test("Add text and T each send one text.add, and the text is a plan bitmap", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		const bitmaps: string[] = [];
		page.on("request", (r) => {
			if (r.url().includes("/bitmaps/")) bitmaps.push(r.url());
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.getByRole("main", { name: "Preview" })).toBeVisible();
		const posted = recordEdits(page);

		await page.getByRole("tab", { name: "Text" }).click();
		await page.getByRole("button", { name: "Add text" }).click();
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{
				op: "text.add",
				// No track_id: the core picks a free visual track (SDD-002 C.6).
				args: {
					start: "0f",
					duration: "3s",
					text: "Your text",
				},
			},
		]);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await expect.poll(() => bitmaps.length).toBeGreaterThan(0);

		await page.locator("body").press("t");
		await expect.poll(() => posted.length).toBe(2);
		expect(posted[1]?.operations).toHaveLength(1);
		expect(posted[1]?.operations[0]?.op).toBe("text.add");
	});

	test("Add text sends one text.add and the core creates the visual track when there is none", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 0 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.getByRole("main", { name: "Preview" })).toBeVisible();
		const posted = recordEdits(page);

		await page.getByRole("tab", { name: "Text" }).click();
		await page.getByRole("button", { name: "Add text" }).click();
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations.map((o) => o.op)).toEqual(["text.add"]);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
	});

	test("a double click on the timeline item edits inline; Ctrl+Enter sends one text.set", async ({
		page,
		request,
	}) => {
		const seeded = await seedText(request);
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const item = page.locator("[data-item-id]");
		await expect(item).toHaveCount(1);
		await expect(item).toContainText("Hello");
		const posted = recordEdits(page);

		await item.dblclick();
		const editor = page.getByTestId("inline-text-editor");
		await expect(editor).toBeFocused();
		await editor.fill("Line one\nLine two");
		await editor.press("Control+Enter");

		await expect(editor).toHaveCount(0);
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{
				op: "text.set",
				args: { item_id: seeded.itemId, text: "Line one\nLine two" },
			},
		]);
		await expect
			.poll(async () => (await stored(page, seeded.itemId)).text)
			.toBe("Line one\nLine two");
		// The label is the first line (SDD-012 section 7.4).
		await expect(item).toContainText("Line one");
		await expect(item).not.toContainText("Line two");
	});

	test("a double click on the canvas opens the editor; blur commits and Esc discards", async ({
		page,
		request,
	}) => {
		const seeded = await seedText(request);
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const canvas = page.getByTestId("canvas-overlay");
		const editor = page.getByTestId("inline-text-editor");
		await expect(async () => {
			const box = await canvas.boundingBox();
			if (box === null) throw new Error("canvas is not laid out");
			await page.mouse.dblclick(box.x + box.width / 2, box.y + box.height / 2);
			await expect(editor).toBeVisible({ timeout: 500 });
		}).toPass();
		const posted = recordEdits(page);

		await editor.fill("Discarded");
		await editor.press("Escape");
		await expect(editor).toHaveCount(0);
		expect(posted).toHaveLength(0);
		expect((await stored(page, seeded.itemId)).text).toBe("Hello");

		await expect(async () => {
			const box = await canvas.boundingBox();
			if (box === null) throw new Error("canvas is not laid out");
			await page.mouse.dblclick(box.x + box.width / 2, box.y + box.height / 2);
			await expect(editor).toBeVisible({ timeout: 500 });
		}).toPass();
		await editor.fill("Kept");
		await page.locator("[data-region=inspector] h2").click();
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{ op: "text.set", args: { item_id: seeded.itemId, text: "Kept" } },
		]);
	});

	test("changing the font in the inspector sends one text.set with only font_family", async ({
		page,
		request,
	}) => {
		const seeded = await seedText(request);
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator("[data-item-id]").click();
		await openInspectorTab(page, "Text");
		const font = page.getByLabel("Font", { exact: true });
		await expect(font.locator("option")).not.toHaveCount(1);
		// The core refuses a family that lacks the default weight 700, so the test
		// picks one that has it.
		const call = await caller(request);
		const listed = (await call("GET", "/fonts")) as {
			fonts: { family: string; weights: number[] }[];
		};
		const other = listed.fonts.find(
			(f) => f.family !== "Inter" && f.weights.includes(700),
		)?.family;
		if (other === undefined) throw new Error("no second bundled family");
		const posted = recordEdits(page);

		await font.selectOption({ label: other });

		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{
				op: "text.set",
				args: { item_id: seeded.itemId, style: { font_family: other } },
			},
		]);
		await expect
			.poll(async () => (await stored(page, seeded.itemId)).style?.font_family)
			.toBe(other);
	});

	test("a family without the current weight is sent with the nearest weight it has", async ({
		page,
		request,
	}) => {
		const seeded = await seedText(request);
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator("[data-item-id]").click();
		await openInspectorTab(page, "Text");
		const font = page.getByLabel("Font", { exact: true });
		await expect(font.locator("option")).not.toHaveCount(1);
		const call = await caller(request);
		const listed = (await call("GET", "/fonts")) as {
			fonts: { family: string; weights: number[] }[];
		};
		const anton = listed.fonts.find((f) => f.family === "Anton");
		if (anton === undefined) throw new Error("Anton is not bundled");
		expect(anton.weights).not.toContain(700);
		const posted = recordEdits(page);

		await font.selectOption({ label: "Anton" });

		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{
				op: "text.set",
				args: {
					item_id: seeded.itemId,
					style: { font_family: "Anton", font_weight: anton.weights[0] },
				},
			},
		]);
		await expect
			.poll(async () => (await stored(page, seeded.itemId)).style?.font_family)
			.toBe("Anton");
		await expect(page.getByRole("alert")).toHaveCount(0);
	});

	test("E_FONT_UNKNOWN shows under the field and highlights the available families", async ({
		page,
		request,
	}) => {
		const seeded = await seedText(request);
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator("[data-item-id]").click();
		await openInspectorTab(page, "Text");
		const font = page.getByLabel("Font", { exact: true });
		await expect(font.locator("option")).not.toHaveCount(1);
		const refetched: string[] = [];
		page.on("request", (r) => {
			if (r.url().endsWith("/fonts")) refetched.push(r.url());
		});
		await page.route("**/edit", async (route) => {
			if (route.request().method() !== "POST") {
				await route.continue();
				return;
			}
			await route.fulfill({
				status: 422,
				contentType: "application/json",
				json: {
					code: "E_FONT_UNKNOWN",
					message: 'Font "Nope" is not available.',
					hint: 'Use font_family "Inter".',
					details: { available: ["Inter"] },
				},
			});
		});

		await font.selectOption({ index: 1 });

		await expect(page.getByRole("alert")).toContainText("is not available");
		await expect(font.locator("option[data-available]")).toHaveText(["Inter"]);
		await expect.poll(() => refetched.length).toBeGreaterThan(0);
	});

	test("an HTML payload in a text item renders literally and nothing runs", async ({
		page,
		request,
	}) => {
		const payload = "<img src=x onerror=window.__pwned=1>";
		const seeded = await seedText(request, payload);
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const item = page.locator("[data-item-id]");
		await expect(item).toContainText(payload);
		await item.click();
		await openInspectorTab(page, "Text");
		await expect(page.getByLabel("Content")).toHaveValue(payload);
		await item.dblclick();
		await expect(page.getByTestId("inline-text-editor")).toHaveValue(payload);
		await page.keyboard.press("Escape");

		expect(
			await page.evaluate(
				() => (window as unknown as { __pwned?: unknown }).__pwned,
			),
		).toBeUndefined();
		await expect(page.locator("img[src='x']")).toHaveCount(0);
	});
});
