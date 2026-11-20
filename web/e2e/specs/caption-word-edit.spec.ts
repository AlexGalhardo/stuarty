import type { APIRequestContext, Page } from "@playwright/test";
import { cspViolations } from "../support/browsers";
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

	// VV-095: the words are reachable by role and name, and Enter edits one.
	test("a word is a button with a name and Enter edits it", async ({
		page,
		request,
	}) => {
		const seeded = await seedCaptions(request, [
			{ start: "0s", text: "one two three four" },
		]);
		const itemId = seeded.itemIds[0] as string;
		await open(page, seeded.id);
		const posted = recordEdits(page);

		const word = page.getByRole("button", { name: "Word 2: two" });
		await word.focus();
		await word.press("Enter");
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
		browserName,
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
		await openInspectorTab(page, "Text");
		await expect(page.getByLabel("Text of word 2")).toHaveValue(payload);

		expect(
			await page.evaluate(
				() => (window as unknown as { __pwned?: unknown }).__pwned,
			),
		).toBeUndefined();
		expect(
			cspViolations(
				browserName,
				await page.evaluate(
					() => (window as unknown as { __violations: string[] }).__violations,
				),
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

		await openInspectorTab(page, "Text");
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
			args: { item_id: itemId, style: { highlight: "word" }, replace: true },
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
		await openInspectorTab(page, "Text");
		await expect(page.locator("[data-item-id]")).toHaveAttribute(
			"aria-selected",
			"true",
		);
		await expect(page.getByLabel("Text of word 1")).toBeVisible();
	});
});

interface Recorded {
	method: string;
	path: string;
}

// Every request of the page to the API, in order, so a test can say which ones
// were sent and which were not.
function recordRequests(page: Page): Recorded[] {
	const seen: Recorded[] = [];
	page.on("request", (r) => {
		const url = new URL(r.url());
		if (url.pathname.startsWith("/api/v1/")) {
			seen.push({ method: r.method(), path: url.pathname });
		}
	});
	return seen;
}

// A project with one clip of the test video on one visual track, selected.
async function openClip(
	page: Page,
	request: APIRequestContext,
): Promise<{ id: string; clipId: string; assetId: string; trackId: string }> {
	const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
	const clipId = seeded.itemIds[0] as string;
	await page.goto(`/editor/${seeded.id}?debug=1`);
	await page.locator(`[data-item-id="${clipId}"]`).click();
	await openInspectorTab(page, "Text");
	await expect(
		page.getByRole("button", { name: "Generate captions" }),
	).toBeVisible();
	return {
		id: seeded.id,
		clipId,
		assetId: seeded.assetId,
		trackId: seeded.trackIds[0] as string,
	};
}

const transcribeJob = (
	state: "running" | "succeeded",
	progress: number,
): Record<string, unknown> => ({
	job_id: "job_fake000001",
	kind: "transcribe",
	project_id: null,
	state,
	progress,
	actor: "local:ui",
	created_at: "2026-11-15T10:00:00Z",
	text: "",
});

// The error the real service answers when the helper is not installed.
const unavailableBody = {
	code: "E_TRANSCRIBE_UNAVAILABLE",
	message: "Local transcription needs whisper-cli, which is not installed.",
	hint: "Run the install command, or configure an API provider.",
	details: {
		missing: [
			{ name: "whisper-cli", url_host: "example.test", bytes: 4_200_000 },
		],
		download_bytes: 4_200_000,
		install_command: "stuarty tools:install whisper --model small",
	},
};

// A caption linked to the clip, on a track of its own, added through the API.
async function seedLinkedCaption(
	request: APIRequestContext,
): Promise<{ id: string; clipId: string; captionId: string }> {
	const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
	const clipId = seeded.itemIds[0] as string;
	const call = await caller(request);
	const head = (await call("GET", `/projects/${seeded.id}`)) as {
		revision: number;
	};
	const added = (await call("POST", `/projects/${seeded.id}/edit`, {
		base_revision: head.revision,
		operations: [
			{ op: "track.add", args: { kind: "visual", id: "trk_caps000001" } },
			{
				op: "caption.add",
				args: {
					track_id: "trk_caps000001",
					start: "0s",
					duration: "1s",
					text: "hello there",
					linked_item: clipId,
				},
			},
		],
	})) as { created: string[] };
	return {
		id: seeded.id,
		clipId,
		captionId: added.created.at(-1) as string,
	};
}

// P2-UI-019 (SDD-012 sections 12.4, 12.5 and 14.2): the transcription job is
// faked at the HTTP layer, because the end-to-end server has no transcription
// provider; the edit that follows goes to the real core.
test.describe("auto captions", () => {
	test("Generate starts one job, shows its progress and sends one caption.from_transcript", async ({
		page,
		request,
	}) => {
		const clip = await openClip(page, request);
		let jobStarts = 0;
		await page.route("**/api/v1/projects/*/jobs", async (route) => {
			if (route.request().method() !== "POST") {
				await route.continue();
				return;
			}
			jobStarts++;
			expect(route.request().postDataJSON()).toEqual({
				kind: "transcribe",
				args: { asset_id: clip.assetId },
				timeline_id: clip.id,
			});
			await route.fulfill({
				status: 202,
				contentType: "application/json",
				json: transcribeJob("running", 0.4),
			});
		});
		await page.route("**/api/v1/jobs/job_fake000001", (route) =>
			route.fulfill({
				contentType: "application/json",
				json: transcribeJob("succeeded", 1),
			}),
		);
		const posted = recordEdits(page);

		await page.getByRole("button", { name: "Generate captions" }).click();

		await expect(page.getByRole("progressbar")).toBeVisible();
		await expect(page.getByText("40%", { exact: true })).toBeVisible();
		await expect.poll(() => posted.length, { timeout: 15_000 }).toBe(1);
		expect(jobStarts).toBe(1);
		const ops = posted[0]?.operations ?? [];
		expect(ops.map((o) => o.op)).toEqual([
			"track.add",
			"caption.from_transcript",
		]);
		expect(ops[0]?.args).toMatchObject({ kind: "visual", name: "Captions" });
		expect(ops[1]?.args).toMatchObject({
			clip_item_id: clip.clipId,
			track_id: ops[0]?.args.id,
			max_words: 5,
			max_chars: 28,
			max_gap: "0.6s",
			replace: true,
		});
	});

	test("an unavailable provider shows the hint and the install dialog; nothing downloads until Install", async ({
		page,
		request,
	}) => {
		const clip = await openClip(page, request);
		await page.route("**/api/v1/projects/*/jobs", (route) =>
			route.request().method() === "POST"
				? route.fulfill({
						status: 422,
						contentType: "application/json",
						json: unavailableBody,
					})
				: route.continue(),
		);
		let polls = 0;
		await page.route("**/api/v1/system/tools", (route) => {
			polls++;
			return route.fulfill({
				contentType: "application/json",
				json: {
					installed: [],
					state: polls < 2 ? "running" : "idle",
					done_bytes: 2_100_000,
					total_bytes: 4_200_000,
					text: "",
				},
			});
		});
		let installBody: unknown;
		await page.route("**/api/v1/system/tools/install", (route) => {
			installBody = route.request().postDataJSON();
			return route.fulfill({
				status: 202,
				contentType: "application/json",
				json: {
					state: "running",
					files: [],
					download_bytes: 4_200_000,
					text: "",
				},
			});
		});
		const seen = recordRequests(page);
		const touchedTools = (): boolean =>
			seen.some((r) => r.path.startsWith("/api/v1/system/tools"));

		await page.getByRole("button", { name: "Generate captions" }).click();

		const panel = page.locator("[data-unavailable]");
		await expect(panel).toContainText("Run the install command");
		const dialog = page.getByRole("dialog");
		await expect(dialog).toContainText("4.2 MB");
		await expect(dialog).toContainText("example.test");
		await expect(
			dialog.getByLabel("Or run this command in a terminal"),
		).toHaveValue("stuarty tools:install whisper --model small");
		expect(seen.filter((r) => r.method !== "GET")).toEqual([
			{ method: "POST", path: `/api/v1/projects/${clip.id}/jobs` },
		]);
		expect(touchedTools()).toBe(false);

		await dialog.getByRole("button", { name: "Cancel" }).click();
		await expect(dialog).toBeHidden();
		expect(touchedTools()).toBe(false);

		await page
			.getByRole("button", { name: "Install local transcription" })
			.click();
		const installDialog = page.getByRole("dialog");
		await expect(installDialog.getByLabel("Model")).toHaveValue("small");
		await expect(installDialog).toContainText("becomes the default");
		await installDialog.getByLabel("Model").selectOption("base");
		await installDialog
			.getByRole("button", { name: "Install", exact: true })
			.click();
		await expect(page.getByRole("dialog")).toContainText("Installed.");
		expect(installBody).toEqual({ name: "whisper", model: "base" });
		const installs = seen.filter(
			(r) => r.method === "POST" && r.path === "/api/v1/system/tools/install",
		);
		expect(installs).toHaveLength(1);
		expect(seen.filter((r) => r.method === "POST")).toHaveLength(2);
	});

	test("a platform with no helper shows only the message and no Install button", async ({
		page,
		request,
	}) => {
		await openClip(page, request);
		await page.route("**/api/v1/projects/*/jobs", (route) =>
			route.request().method() === "POST"
				? route.fulfill({
						status: 422,
						contentType: "application/json",
						json: {
							code: "E_TRANSCRIBE_UNAVAILABLE",
							message: "no transcription provider is available.",
							hint: "Configure an API provider.",
						},
					})
				: route.continue(),
		);

		await page.getByRole("button", { name: "Generate captions" }).click();

		const dialog = page.getByRole("dialog");
		await expect(dialog).toContainText("Configure an API provider.");
		await expect(
			dialog.getByRole("button", { name: "Install", exact: true }),
		).toHaveCount(0);
	});

	test("a clip with linked captions has Re-sync, which sends one caption.retime_from_clip", async ({
		page,
		request,
	}) => {
		const seeded = await seedLinkedCaption(request);
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator(`[data-item-id="${seeded.clipId}"]`).click();
		const posted = recordEdits(page);

		await openInspectorTab(page, "Text");
		await page.getByRole("button", { name: "Re-sync captions" }).click();

		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{
				op: "caption.retime_from_clip",
				args: { clip_item_id: seeded.clipId },
			},
		]);
	});

	test("the out-of-sync lint finding offers Re-sync and sends the same command", async ({
		page,
		request,
	}) => {
		const seeded = await seedLinkedCaption(request);
		await page.route("**/api/v1/projects/*/lint*", (route) =>
			route.fulfill({
				contentType: "application/json",
				json: {
					project_id: seeded.id,
					revision: 1,
					text: "",
					findings: [
						{
							code: "L_CAPTION_DESYNC",
							severity: "warning",
							message: "The caption is out of sync with its clip.",
							item: seeded.captionId,
						},
					],
				},
			}),
		);
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(
			page.locator(`[data-item-id="${seeded.clipId}"]`),
		).toBeVisible();
		const posted = recordEdits(page);

		await page.getByRole("button", { name: "Export" }).click();
		await page
			.getByRole("dialog")
			.getByRole("button", { name: "Re-sync captions" })
			.click();

		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{
				op: "caption.retime_from_clip",
				args: { clip_item_id: seeded.clipId },
			},
		]);
	});
});
