import type { APIRequestContext, Page } from "@playwright/test";
import { caller, expect, seedProject, test } from "./helpers";

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

// A project with one visual track and captions added through the API, each one
// `10s` long from the given start.
async function seedCaptions(
	request: APIRequestContext,
	captions: { start: string; text: string }[],
): Promise<{ id: string; trackId: string; itemIds: string[] }> {
	const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
	const trackId = seeded.trackIds[0] as string;
	const call = await caller(request);
	const head = (await call("GET", `/projects/${seeded.id}`)) as {
		revision: number;
	};
	const out = (await call("POST", `/projects/${seeded.id}/edit`, {
		base_revision: head.revision,
		operations: captions.map((c) => ({
			op: "caption.add",
			args: {
				track_id: trackId,
				start: c.start,
				duration: "10s",
				text: c.text,
			},
		})),
	})) as { created: string[] };
	return { id: seeded.id, trackId, itemIds: out.created };
}

interface StoredWord {
	text: string;
	start: number;
	duration: number;
}

async function storedWords(page: Page, itemId: string): Promise<StoredWord[]> {
	return page.evaluate((id) => {
		const doc = window.__vv?.document as {
			items: Record<string, { words?: StoredWord[] }>;
		};
		return doc.items[id]?.words ?? [];
	}, itemId);
}

async function open(page: Page, id: string, count = 1): Promise<void> {
	await page.goto(`/editor/${id}?debug=1`);
	await expect(page.locator("[data-item-id]")).toHaveCount(count);
}

// P2-UI-018 (SDD-012 sections 7.4, 8.2 and 12.1): captions are edited word by
// word on the timeline and in the inspector; every gesture is one request.
test.describe("caption-word-edit", () => {
	test("a wide caption shows one segment per word and a click selects the word", async ({
		page,
		request,
	}) => {
		const seeded = await seedCaptions(request, [
			{ start: "0s", text: "one two three four" },
		]);
		await open(page, seeded.id);
		const words = page.locator("[data-word-index]");
		await expect(words).toHaveCount(4);
		await expect(words.nth(1)).toHaveText("two");

		await words.nth(1).click();
		await expect(words.nth(1)).toHaveAttribute("data-selected", "");
		await expect(page.locator("[data-item-id]")).toHaveAttribute(
			"aria-selected",
			"true",
		);
	});

	test("a double click edits a word and an empty text removes it", async ({
		page,
		request,
	}) => {
		const seeded = await seedCaptions(request, [
			{ start: "0s", text: "one two three four" },
		]);
		const itemId = seeded.itemIds[0] as string;
		await open(page, seeded.id);
		const posted = recordEdits(page);

		await page.locator("[data-word-index='1']").dblclick();
		const editor = page.getByLabel("Word 2", { exact: true });
		await editor.fill("2");
		await editor.press("Enter");
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{
				op: "caption.edit_word",
				args: { item_id: itemId, index: 1, text: "2" },
			},
		]);
		await expect
			.poll(async () => (await storedWords(page, itemId))[1]?.text)
			.toBe("2");

		await page.locator("[data-word-index='1']").dblclick();
		const again = page.getByLabel("Word 2", { exact: true });
		await again.fill("");
		await again.press("Enter");
		await expect.poll(() => posted.length).toBe(2);
		expect(posted[1]?.operations[0]?.args).toEqual({
			item_id: itemId,
			index: 1,
			text: "",
		});
		await expect
			.poll(async () => (await storedWords(page, itemId)).length)
			.toBe(3);
	});

	test("the context menu splits the caption before the word", async ({
		page,
		request,
	}) => {
		const seeded = await seedCaptions(request, [
			{ start: "0s", text: "one two three four" },
		]);
		const itemId = seeded.itemIds[0] as string;
		await open(page, seeded.id);
		const posted = recordEdits(page);

		await page.locator("[data-word-index='2']").click({ button: "right" });
		await page.getByRole("menuitem", { name: "Split caption here" }).click();

		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{
				op: "caption.split",
				args: { item_id: itemId, before_word: 2 },
			},
		]);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
	});

	test("two selected captions are merged with one caption.merge", async ({
		page,
		request,
	}) => {
		const seeded = await seedCaptions(request, [
			{ start: "0s", text: "one two" },
			{ start: "10s", text: "three four" },
		]);
		await open(page, seeded.id, 2);
		const items = page.locator("[data-item-id]");
		const posted = recordEdits(page);

		await items.first().click({ position: { x: 4, y: 4 } });
		await items.nth(1).click({
			modifiers: ["Shift"],
			position: { x: 4, y: 4 },
		});
		await items.nth(1).click({
			button: "right",
			position: { x: 4, y: 4 },
		});
		await page.getByRole("menuitem", { name: "Merge captions" }).click();

		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toHaveLength(1);
		expect(posted[0]?.operations[0]?.op).toBe("caption.merge");
		expect(
			[...((posted[0]?.operations[0]?.args.item_ids as string[]) ?? [])].sort(),
		).toEqual([...seeded.itemIds].sort());
		await expect(items).toHaveCount(1);
	});

	test("dragging a boundary sends exactly one caption.set_words that changes two words", async ({
		page,
		request,
	}) => {
		const seeded = await seedCaptions(request, [
			{ start: "0s", text: "one two three four" },
		]);
		const itemId = seeded.itemIds[0] as string;
		await open(page, seeded.id);
		const before = await storedWords(page, itemId);
		const posted = recordEdits(page);

		const handle = page.locator("[data-word-boundary='2']");
		const box = await handle.boundingBox();
		if (box === null) throw new Error("boundary is not laid out");
		const x = box.x + box.width / 2;
		const y = box.y + box.height / 2;
		await page.mouse.move(x, y);
		await page.mouse.down();
		await page.mouse.move(x + 30, y, { steps: 6 });
		await expect(page.getByTestId("snap-line")).toBeVisible();
		await page.mouse.up();

		await expect.poll(() => posted.length).toBe(1);
		const op = posted[0]?.operations[0];
		expect(posted[0]?.operations).toHaveLength(1);
		expect(op?.op).toBe("caption.set_words");
		const sent = op?.args.words as { text: string; start: string }[];
		expect(sent).toHaveLength(before.length);
		// The words around the boundary moved; the other two are copied.
		expect(sent[0]?.start).toBe(`${before[0]?.start}f`);
		expect(sent[3]?.start).toBe(`${before[3]?.start}f`);
		await expect
			.poll(async () => (await storedWords(page, itemId))[2]?.start)
			.toBeGreaterThan(before[2]?.start ?? 0);
	});

	test("a word with markup renders literally and no policy violation is reported", async ({
		page,
		request,
	}) => {
		await page.addInitScript(() => {
			(window as unknown as { __violations: string[] }).__violations = [];
			document.addEventListener("securitypolicyviolation", (event) => {
				(window as unknown as { __violations: string[] }).__violations.push(
					event.violatedDirective,
				);
			});
		});
		const payload = "<script>window.__pwned=1</script>";
		const seeded = await seedCaptions(request, [
			{ start: "0s", text: `one ${payload} three` },
		]);
		await open(page, seeded.id);
		await expect(page.locator("[data-word-index='1']")).toHaveText(payload);
		await page.locator("[data-item-id]").click({ position: { x: 4, y: 4 } });
		await expect(page.getByLabel("Text of word 2")).toHaveValue(payload);

		expect(
			await page.evaluate(
				() => (window as unknown as { __pwned?: unknown }).__pwned,
			),
		).toBeUndefined();
		expect(
			await page.evaluate(
				() => (window as unknown as { __violations: string[] }).__violations,
			),
		).toEqual([]);
	});

	test("the word list edits text and breaks, and the style sends the highlight", async ({
		page,
		request,
	}) => {
		const seeded = await seedCaptions(request, [
			{ start: "0s", text: "one two three four" },
		]);
		const itemId = seeded.itemIds[0] as string;
		await open(page, seeded.id);
		await page.locator("[data-item-id]").click({ position: { x: 4, y: 4 } });
		const posted = recordEdits(page);

		const cell = page.getByLabel("Text of word 3");
		await cell.fill("3");
		await cell.press("Enter");
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations[0]).toEqual({
			op: "caption.edit_word",
			args: { item_id: itemId, index: 2, text: "3" },
		});

		await page.getByLabel("Line break before word 3").click();
		await expect.poll(() => posted.length).toBe(2);
		expect(posted[1]?.operations[0]).toEqual({
			op: "caption.set_breaks",
			args: { item_id: itemId, breaks: [2] },
		});

		const start = page.getByLabel("Start of word 2");
		await start.fill("1s");
		await start.press("Enter");
		await expect.poll(() => posted.length).toBe(3);
		const words = posted[2]?.operations[0]?.args.words as {
			start: string;
		}[];
		expect(posted[2]?.operations[0]?.op).toBe("caption.set_words");
		expect(words).toHaveLength(4);
		expect(words[1]?.start).toBe("1s");

		await page
			.getByLabel("Highlight", { exact: true })
			.selectOption({ label: "Spoken word" });
		await expect.poll(() => posted.length).toBe(4);
		expect(posted[3]?.operations[0]).toEqual({
			op: "caption.set_style",
			args: { item_id: itemId, style: { highlight: "word" } },
		});

		await page.getByRole("button", { name: "Use track style" }).click();
		await expect.poll(() => posted.length).toBe(5);
		expect(posted[4]?.operations[0]).toEqual({
			op: "caption.set_style",
			args: { item_id: itemId, style: null },
		});
	});

	test("E_INVALID_STATE on a split offers a button that selects the caption", async ({
		page,
		request,
	}) => {
		const seeded = await seedCaptions(request, [
			{ start: "0s", text: "one two three four" },
		]);
		await open(page, seeded.id);
		await page.route("**/edit", async (route) => {
			if (route.request().method() !== "POST") {
				await route.continue();
				return;
			}
			await route.fulfill({
				status: 409,
				contentType: "application/json",
				json: {
					code: "E_INVALID_STATE",
					message: "a side would have no words",
					hint: "Split at a word boundary.",
				},
			});
		});

		await page.locator("[data-word-index='1']").click({ button: "right" });
		await page.getByRole("menuitem", { name: "Split caption here" }).click();
		await expect(page.getByRole("alert")).toContainText("Split at a word");
		await page.getByRole("button", { name: "Show the words" }).click();
		await expect(page.locator("[data-item-id]")).toHaveAttribute(
			"aria-selected",
			"true",
		);
		await expect(page.getByLabel("Text of word 1")).toBeVisible();
	});
});
