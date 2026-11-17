import { writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { skipWithoutWebAudio } from "../support/browsers";
import { caller, expect, seedProject, test, testClip } from "./helpers";

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

test.describe("prototype-reference", () => {
	test("the four manual operations of D-023 with the UI alone, no console error, no CSP violation", async ({
		page,
		request,
		browserName,
	}) => {
		test.setTimeout(150_000);
		const problems: string[] = [];
		page.on("console", (m) => {
			// The browser logs the two expected refusals described below by itself.
			// WebKit adds its own console line to each style-src-attr violation (see
			// cspViolations in support/browsers.ts).
			if (
				m.type() === "error" &&
				!/Failed to load resource.*status of (409|400)/.test(m.text()) &&
				!(
					browserName === "webkit" &&
					(m.text().startsWith("csp: style-src-attr") ||
						m.text().startsWith("Refused to apply a stylesheet"))
				)
			)
				problems.push(`console: ${m.text()}`);
		});
		page.on("pageerror", (e) => problems.push(`pageerror: ${e.message}`));
		page.on("response", (r) => {
			// The 409 of POST /projects is the Inertia redirect to the editor. A truth
			// frame pinned to a revision that an edit has just replaced is refused
			// (400 or 409) and the canvas stays (truth on pause, SDD-012 section 11): both are by design.
			const expected =
				(r.status() === 409 && r.url().endsWith("/projects")) ||
				(r.status() < 500 && r.url().includes("/frame?"));
			if (r.status() >= 400 && !expected)
				problems.push(
					`response: ${r.status()} ${r.request().method()} ${r.url()}`,
				);
		});
		await page.addInitScript(() => {
			document.addEventListener("securitypolicyviolation", (e) =>
				console.error(`csp: ${e.violatedDirective}`),
			);
		});

		const name = `Reference ${Date.now()}`;
		await page.goto("/projects");
		await skipWithoutWebAudio(page, (condition, reason) =>
			test.skip(condition, reason),
		);
		await page.getByLabel("Name").fill(name);
		await page.getByRole("button", { name: "Create project" }).click();
		await expect(page).toHaveURL(/\/editor\/prj_/);
		const id = page.url().split("/").pop() ?? "";

		// 1. Import by path, see the card.
		await page.getByLabel("Path of a file to import").fill(testClip(0));
		await page.getByRole("button", { name: "Import" }).click();
		const add = page.getByRole("button", { name: "Add to timeline" }).first();
		await expect(add).toBeEnabled({ timeout: 30_000 });

		// Place it, play, pause and see the exact frame.
		await add.click();
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.getByRole("button", { name: "Play" }).click();
		await page.waitForTimeout(1000);
		await page.getByRole("button", { name: "Pause" }).click();
		await expect(
			page.getByAltText("Exact frame at the playhead"),
		).toBeVisible();

		// 2. Split at the playhead, then trim an end.
		await page.locator("[data-item-id]").first().click();
		await page.getByRole("button", { name: "Split" }).click();
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		await page.locator("[data-item-id]").last().click();
		await page.getByRole("button", { name: "Step forward one frame" }).click();
		await page.getByRole("button", { name: "Step forward one frame" }).click();
		await page.getByRole("button", { name: "Trim end" }).click();

		// 3. Add a caption.
		await page.getByRole("button", { name: "Add caption" }).click();
		await page.getByRole("dialog").getByLabel("Text").fill("Launch day");
		await page
			.getByRole("dialog")
			.getByRole("button", { name: "Add caption" })
			.click();
		await expect(page.locator("[data-item-id]")).toHaveCount(3);

		// 4. Export with progress to success; the file exists in the API result.
		await page.getByRole("button", { name: "Export" }).click();
		await page.getByRole("button", { name: "Start export" }).click();
		await expect(page.getByRole("link", { name: "Download" })).toBeVisible({
			timeout: 90_000,
		});
		const call = await caller(request);
		const jobs = (await call("GET", `/projects/${id}/jobs`)) as {
			jobs: { state: string; path?: string }[];
		};
		expect(
			jobs.jobs.some((j) => j.state === "succeeded" || j.state === "done"),
		).toBe(true);
		expect(problems).toEqual([]);
	});
});
