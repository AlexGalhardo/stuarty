import { writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { caller, expect, seedProject, test } from "./helpers";

// Scenarios of SDD-012 section 20.1 that run against the real server: 1, 2
// (the undo half), 4, 7, 8 and 11.

test.describe("import-errors", () => {
	test("a missing path and a text file show the code headline, the message and the hint", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 0 });
		await page.goto(`/editor/${seeded.id}`);
		const path = page.getByLabel("Path of a file to import");

		await path.fill(join(tmpdir(), "vukem-video-e2e-no-such-file.mp4"));
		await page.getByRole("button", { name: "Import" }).click();
		await expect(page.getByText("File not found").first()).toBeVisible();

		const text = join(tmpdir(), "vukem-video-e2e-not-media.txt");
		writeFileSync(text, "this is not a video\n");
		await path.fill(text);
		await page.getByRole("button", { name: "Import" }).click();
		await expect(page.getByText("Unsupported media").first()).toBeVisible();
		await expect(page.getByText(/not a media file/).first()).toBeVisible();
		await expect(
			page.getByText(/Import a video, audio or image file/).first(),
		).toBeVisible();
	});
});

test.describe("export-lint-blocking", () => {
	test("findings are listed, Start is disabled and a forced JobStart answers E_LINT_BLOCKING", async ({
		page,
		request,
	}) => {
		// A track with no clip: the project has nothing to export.
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		await page.goto(`/editor/${seeded.id}`);
		await page.getByRole("button", { name: "Export" }).click();
		await expect(page.getByRole("heading", { name: /^Errors/ })).toBeVisible();
		await expect(
			page.getByRole("button", { name: "Start export" }),
		).toBeDisabled();

		const call = await caller(request);
		await expect(
			call("POST", `/projects/${seeded.id}/jobs`, { kind: "export" }),
		).rejects.toThrow(/E_LINT_BLOCKING/);
	});
});

test.describe("undo-redo-per-actor", () => {
	test("the UI undo reverts the UI step and leaves the step of another actor", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);

		// The UI splits the clip.
		await page.getByRole("button", { name: "Step forward one frame" }).click();
		await page.locator("[data-item-id]").click();
		await page.getByRole("button", { name: "Split" }).click();
		await expect(page.locator("[data-item-id]")).toHaveCount(2);

		// Another actor adds a clip after the UI step.
		const agent = await caller(request, "claude-code");
		const head = (await agent("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		await agent("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "clip.add",
					args: {
						track_id: seeded.trackIds[0],
						asset_id: seeded.assetId,
						start: "5s",
						source_in: "0s",
						duration: "1s",
					},
				},
			],
		});
		await expect(page.locator("[data-item-id]")).toHaveCount(3);

		// Undo reverts the split only: the agent's clip stays.
		await page.getByRole("button", { name: "Undo" }).click();
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		const doc = (await agent(
			"GET",
			`/projects/${seeded.id}?detail=document`,
		)) as {
			document: { items: Record<string, { start: unknown }> };
		};
		expect(Object.keys(doc.document.items)).toHaveLength(2);

		await page.getByRole("button", { name: "Redo" }).click();
		await expect(page.locator("[data-item-id]")).toHaveCount(3);
	});
});

test.describe("two-actor-live-update", () => {
	test("changed items are outlined and the revision advances", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const revision = page.getByText(/^rev \d+$/).first();
		const before = await revision.textContent();

		const agent = await caller(request, "claude-code");
		const head = (await agent("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		await agent("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "clip.add",
					args: {
						track_id: seeded.trackIds[0],
						asset_id: seeded.assetId,
						start: "5s",
						source_in: "0s",
						duration: "1s",
					},
				},
			],
		});
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		await expect(page.getByText("claude-code is editing")).toBeVisible();
		await expect(revision).not.toHaveText(before ?? "");
		// The new item carries the outline of a remote edit for 1.5 s.
		await expect(page.locator("[data-item-id].outline-2")).toHaveCount(1);
	});
});

test.describe("event-gap-resync", () => {
	test("a stream that dies mid-session ends at the same document with no duplicate", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		let streams = 0;
		await page.route("**/events**", async (route) => {
			streams++;
			// The first stream is killed; the client reconnects with `since`.
			if (streams === 1) await route.abort();
			else await route.continue();
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);

		const agent = await caller(request, "claude-code");
		for (const start of ["5s", "7s"]) {
			const head = (await agent("GET", `/projects/${seeded.id}`)) as {
				revision: number;
			};
			await agent("POST", `/projects/${seeded.id}/edit`, {
				base_revision: head.revision,
				operations: [
					{
						op: "clip.add",
						args: {
							track_id: seeded.trackIds[0],
							asset_id: seeded.assetId,
							start,
							source_in: "0s",
							duration: "1s",
						},
					},
				],
			});
		}
		await expect(page.locator("[data-item-id]")).toHaveCount(3, {
			timeout: 20_000,
		});
		expect(streams).toBeGreaterThanOrEqual(2);
		const ids = await page
			.locator("[data-item-id]")
			.evaluateAll((els) => els.map((e) => e.getAttribute("data-item-id")));
		expect(new Set(ids).size).toBe(ids.length);
	});
});
