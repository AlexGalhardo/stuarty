import {
	localField,
	type Running,
	signInAsPerson,
	startFirstFlow,
	uploads,
} from "../support/firstFlow";
import {
	caller,
	createThroughTheForm,
	expect,
	test,
	testClip,
} from "./helpers";

// ADR-0031, PRD-004 section 3, epic 38: a creator goes from an idea to a published short vertical video with
// captions and a thumbnail without leaving Stuarty, and this is the same flow as scripts/first-flow.sh, made
// with the hands of a person in the browser: the briefing, a script drafted by a model and edited, footage cut
// to a vertical short, captions from the transcript, a cover from a template, an export with a platform
// preset, a connected account, a publication with a caption, its approval by a person, its time, and the post
// on the calendar.
//
// The spec runs its own server, because the flow needs two things the shared server of the suite is not given:
// the test transcriber (STUARTY_TEST_TRANSCRIBER, which works only under APP_ENV=test) and a fake AI provider
// and a fake social network (internal/tools/firstflowfake, on loopback). It builds both programs, starts them on
// a workspace of its own and stops them at the end. Nothing leaves the machine.
const PORT = Number(process.env.STUARTY_E2E_PORT ?? 3101) + 6;
const BASE = `http://127.0.0.1:${PORT}`;

test.use({ baseURL: BASE });
test.describe.configure({ mode: "serial" });

let running: Running | undefined;

test.beforeAll(async () => {
	test.setTimeout(600_000);
	running = await startFirstFlow(PORT, "first-flow");
});

test.afterAll(() => {
	running?.server.kill();
	running?.fake.kill();
});

// @chromium-only: the spec owns one server on one port, and one engine is enough for a flow of the product.
test.describe("first complete flow @chromium-only", () => {
	test("an idea becomes a published short with captions and a thumbnail", async ({
		page,
		request,
	}) => {
		test.setTimeout(600_000);
		const fake = running as Running;
		const api = await caller(request);
		await signInAsPerson(page);

		let projectId = "";
		const name = `Sourdough short ${Date.now()}`;

		await test.step("create the project with the vertical preset", async () => {
			projectId = await createThroughTheForm(page, name);
			await expect(page).toHaveURL(/\/editor\/prj_[0-9a-hjkmnp-tv-z]{10}$/);
		});

		await test.step("answer the briefing", async () => {
			await page.goto(`/projects/${projectId}`);
			await page
				.getByLabel("Who is this video for?")
				.fill("Home bakers who want a first sourdough loaf");
			await page
				.getByLabel("What should the viewer do or feel after watching?")
				.fill("Watch to the end and try the recipe");
			await page
				.getByLabel("Where will it be published?")
				.selectOption("youtube-shorts");
			await page.getByLabel("Which format?").selectOption("vertical-9x16");
			await page.getByLabel("How long, in seconds?").fill("10");
			await page
				.getByRole("button", { name: "Save the answers as the brief" })
				.click();
			await expect(page.getByRole("status")).toHaveText(
				"The brief was saved from your answers.",
			);
		});

		let scriptId = "";
		await test.step("write the script: a draft from a model that a person approves, then an edit", async () => {
			// The model and its price: the connection is the person's own key, the price is theirs to set.
			await api("POST", "/connections", {
				kind: "openai",
				label: "Fake AI",
				endpoint: fake.providerUrl,
				accepts: ["text"],
				secret: "sk-first-flow-fake-key",
			});
			const priced = await page.evaluate(async () => {
				const response = await fetch("/api/v1/providers/prices", {
					method: "PUT",
					credentials: "same-origin",
					headers: {
						Accept: "application/json",
						"Content-Type": "application/json",
					},
					body: JSON.stringify({
						provider: "openai",
						model: "fake-small",
						currency: "USD",
						input_micros_per_mtok: 1_000_000,
						output_micros_per_mtok: 2_000_000,
					}),
				});
				return response.status;
			});
			expect(priced).toBe(200);

			await page.goto("/write");
			await page.getByRole("button", { name: "New text" }).click();
			const form = page.getByRole("form", { name: "New text" });
			await form.getByLabel("Project").selectOption(projectId);
			await form.getByLabel("Title").fill("Script");
			await form.getByLabel("Kind").selectOption("script");
			await form.getByRole("button", { name: "Create" }).click();
			const editor = page.getByTestId("text-editor");
			await expect(editor).toBeVisible();

			const draft = page.getByTestId("draft-panel");
			await draft.getByLabel("Connection").selectOption({ label: "Fake AI" });
			await expect(draft.getByLabel("Model")).toHaveValue("fake-small");
			await draft
				.getByLabel("What to write")
				.fill("A 10 second script about baking a first sourdough loaf");
			await draft.getByRole("button", { name: "Ask for the draft" }).click();
			await expect(draft.getByTestId("draft-waiting")).toContainText(
				"Waiting for your approval.",
			);

			await page.goto("/approvals");
			const row = page
				.locator("[data-approval-id]")
				.filter({ hasText: "first sourdough loaf" });
			await row.getByRole("button", { name: "Approve" }).click();
			await expect(row).toHaveCount(0);

			const texts = (await api("GET", `/texts?project_id=${projectId}`)) as {
				texts: { id: string }[];
			};
			scriptId = texts.texts[0]?.id ?? "";
			expect(scriptId).not.toBe("");
			await expect
				.poll(async () => {
					const r = (await api(
						"GET",
						`/projects/${projectId}/texts/${scriptId}/revisions`,
					)) as { revisions: { source: string }[] };
					return r.revisions[0]?.source;
				})
				.toBe("generated");

			await page.goto(`/write?project=${projectId}&text=${scriptId}`);
			await expect(
				page.getByTestId("text-editor").getByLabel("Text (Markdown)"),
			).toHaveValue(/Sourdough, from starter to slice/);
			await page
				.getByTestId("text-editor")
				.getByLabel("Text (Markdown)")
				.fill(
					"Sourdough, from starter to slice.\n\nFeed the starter, fold the dough, bake it hot.",
				);
			await page
				.getByTestId("text-editor")
				.getByRole("button", { name: "Save", exact: true })
				.click();
			await expect(page.getByTestId("save-status")).toContainText(
				"Saved as revision",
			);
		});

		await test.step("import the footage and cut it to four seconds", async () => {
			await page.goto(`/editor/${projectId}`);
			await page.getByLabel("Path of a file to import").fill(testClip(0));
			await page.getByRole("button", { name: "Import" }).click();
			const add = page.getByRole("button", { name: "Add to timeline" }).first();
			await expect(add).toBeEnabled({ timeout: 60_000 });
			await add.click();
			await expect(page.locator("[data-item-id]")).toHaveCount(1);
			await page.locator("[data-item-id]").first().click();
			// PageUp on the playhead steps one second (the keys of the timeline, SDD-012 section 18.9).
			await page.getByRole("slider", { name: "Playhead" }).focus();
			for (let i = 0; i < 4; i++) await page.keyboard.press("PageUp");
			await page.getByRole("button", { name: "Trim end" }).click();
			await expect
				.poll(async () => {
					const shown = (await api(
						"GET",
						`/projects/${projectId}?detail=document`,
					)) as { document?: { items?: Record<string, { duration: number }> } };
					const items = Object.values(shown.document?.items ?? {});
					return items[0]?.duration;
				})
				.toBe(120);
		});

		await test.step("captions from the transcript", async () => {
			await page.locator("[data-item-id]").first().click();
			const tab = page
				.getByRole("tablist", { name: /^(Properties|Propriedades)$/ })
				.getByRole("tab", { name: "Text", exact: true });
			await tab.click();
			await page.getByRole("button", { name: "Generate captions" }).click();
			await expect(page.locator("[data-item-id]")).toHaveCount(2, {
				timeout: 60_000,
			});
		});

		await test.step("a thumbnail from a template", async () => {
			await page.goto("/edit-image");
			await page.getByLabel("Name").first().fill("Cover template");
			await page.getByTestId("image-new-template").check();
			await page.getByRole("button", { name: "Create image" }).click();
			await expect(page).toHaveURL(/\/edit-image\/img_/);
			await page.getByTestId("image-add-text").click();
			await page.getByTestId("image-field-text").fill("");
			await page.getByTestId("image-field-text").blur();
			await page.getByTestId("image-field-slot").fill("headline");
			await page.getByTestId("image-field-slot").press("Enter");

			await page.goto("/edit-image");
			await page
				.getByLabel("Template", { exact: true })
				.selectOption({ label: "Cover template" });
			await page
				.getByLabel(/Text .*headline, required/)
				.fill("First sourdough");
			await page.getByLabel("Name").last().fill("Sourdough cover");
			await page.getByRole("button", { name: "Create from template" }).click();
			await expect(page).toHaveURL(/\/edit-image\/img_/);
			await expect(
				page.getByRole("heading", { level: 1, name: "Sourdough cover" }),
			).toBeVisible();
		});

		let exportJob = "";
		await test.step("export with the YouTube Shorts preset", async () => {
			await page.goto(`/editor/${projectId}`);
			await expect(page.locator("[data-item-id]")).toHaveCount(2);
			await page.getByRole("button", { name: "Export", exact: true }).click();
			const dialog = page.getByRole("dialog");
			await dialog.locator("#export-preset-youtube-shorts").check();
			await expect(dialog.getByText("The project fits.")).toBeVisible();
			await dialog
				.getByRole("button", { name: /^(Export 1 file|Start export)$/ })
				.click();
			await expect(
				dialog.getByRole("link", { name: /^Download/ }).first(),
			).toBeVisible({ timeout: 200_000 });
			const jobs = (await api("GET", `/projects/${projectId}/jobs`)) as {
				jobs: { job_id: string; kind: string; state: string }[];
			};
			exportJob =
				jobs.jobs.find((j) => j.kind === "export" && j.state === "succeeded")
					?.job_id ?? "";
			expect(exportJob).not.toBe("");
		});

		await test.step("connect the account of the network with the developer app of the person", async () => {
			await page.goto("/connections");
			await page.getByRole("button", { name: "Set up Fake network" }).click();
			await page.getByLabel("Client id").fill("first-flow-client");
			await page.getByLabel("Client secret").fill("first-flow-client-secret");
			await page.getByRole("button", { name: "Save and sign in" }).click();
			await expect(page.locator("[data-signed-in]")).toContainText(
				"First flow channel",
			);
		});

		await test.step("create the publication with a caption, schedule it, and a person approves it", async () => {
			await page.goto("/publish");
			await page.getByTestId("publish-new").click();
			const dialog = page.getByTestId("publication-editor");
			await dialog.getByLabel("Project").selectOption(projectId);
			await dialog.getByLabel("Exported video").selectOption(exportJob);
			await dialog.getByLabel("Title", { exact: true }).fill("Sourdough short");
			await dialog
				.getByLabel("Caption")
				.fill("From starter to slice in four seconds #sourdough");
			await dialog.getByRole("button", { name: "Add an account" }).click();
			const panel = dialog.getByTestId("target-panel");
			await panel.getByLabel("Account", { exact: true }).selectOption({
				index: 1,
			});
			const at = new Date(Date.now() + 30_000);
			at.setSeconds(0, 0);
			at.setMinutes(at.getMinutes() + 1);
			await panel.getByLabel("Date and time").fill(localField(at));
			await panel.getByLabel("No", { exact: true }).check();
			await dialog.getByRole("button", { name: "Save", exact: true }).click();
			await expect(panel.getByTestId("target-state")).toHaveText("Draft");
			expect(await uploads(fake.socialUrl)).toHaveLength(0);

			await panel.getByTestId("target-schedule").click();
			await expect(panel.getByTestId("target-state")).toHaveText(
				"Waiting for approval",
			);
			await page.keyboard.press("Escape");
			await expect(dialog).toHaveCount(0);
			expect(await uploads(fake.socialUrl)).toHaveLength(0);

			await page.goto("/approvals");
			const item = page
				.locator("[data-approval-id]")
				.filter({ hasText: "Sourdough short" });
			await item.getByRole("button", { name: "Approve" }).click();
			await expect(item).toHaveCount(0);
			expect(await uploads(fake.socialUrl)).toHaveLength(0);
		});

		await test.step("the post goes out at its time, once, and is on the calendar", async () => {
			await page.goto("/publish");
			await expect(page.getByTestId("target-chip")).toContainText("Scheduled");
			await expect
				.poll(async () => (await uploads(fake.socialUrl)).length, {
					timeout: 150_000,
				})
				.toBe(1);
			await expect(page.getByTestId("target-chip")).toContainText("Published", {
				timeout: 30_000,
			});
			await page.getByTestId("publish-mode-list").click();
			const row = page.getByTestId("publish-row");
			await expect(row).toHaveCount(1);
			await expect(row.getByTestId("target-url")).toHaveAttribute(
				"href",
				"https://fake.example/watch/ff1",
			);
			const sent = await uploads(fake.socialUrl);
			expect(sent).toHaveLength(1);
			expect(sent[0]?.path.endsWith(".mp4")).toBe(true);
			expect(sent[0]?.bytes).toBeGreaterThan(10_000);
		});
	});
});
