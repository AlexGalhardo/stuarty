import type { Locator, Page } from "@playwright/test";
import {
	localField,
	type Running,
	signInAsPerson,
	startFirstFlow,
	uploads,
} from "../support/firstFlow";
import {
	arrowTo,
	countPointerEvents,
	pointerEvents,
	tabTo,
} from "../support/keyboard";
import { caller, expect, test, testClip } from "./helpers";

// Epic 41 task 8, acceptance criterion 2 (SDD-012 section 18.10, item K): the first flow of the product, from the
// briefing to the approval of the publication, made in the UI with the keyboard alone: no pointer event reaches
// the page (the count of countPointerEvents), and every control is reached with Tab or with the arrows of its
// widget, not by script. Pages are entered by address, as a person does in the address bar. The server is the
// one of first-flow.spec.ts (a real server with a fake AI provider and a fake social network); the wait for the
// post to go out at its time belongs to that spec, and this one ends when the person has approved the post.
const PORT = Number(process.env.STUARTY_E2E_PORT ?? 3101) + 7;
// The order of the segments of a date field follows the language of the browser window, which a headless Chromium
// takes from the system (month first in English, day first in Portuguese), and the keys below are the English ones.
test.use({
	baseURL: `http://127.0.0.1:${PORT}`,
	locale: "en-US",
	launchOptions: { args: ["--lang=en-US"] },
});
test.describe.configure({ mode: "serial" });

let running: Running | undefined;
test.beforeAll(async () => {
	test.setTimeout(600_000);
	running = await startFirstFlow(PORT, "first-flow-keyboard");
});
test.afterAll(() => {
	running?.server.kill();
	running?.fake.kill();
});

// Activates a button or a link: Tab to it, then Enter.
async function press(page: Page, target: Locator): Promise<void> {
	await tabTo(page, target);
	await page.keyboard.press("Enter");
}

// Types into a field: Tab to it, select what is there, type.
async function type(page: Page, field: Locator, text: string): Promise<void> {
	await tabTo(page, field);
	await page.keyboard.press("Control+a");
	await page.keyboard.type(text);
}

// Chooses an option of a native select: Tab to it, then the typeahead of the select (the start of the label).
async function choose(
	page: Page,
	select: Locator,
	label: string,
): Promise<void> {
	await tabTo(page, select);
	await page.keyboard.type(label);
}

// Chooses the nth option after the first one of a native select, with the arrow key.
async function chooseNth(
	page: Page,
	select: Locator,
	n: number,
): Promise<void> {
	await tabTo(page, select);
	for (let i = 0; i < n; i++) await page.keyboard.press("ArrowDown");
}

// The keys of a datetime-local field of the browser in English: month, day and year, then (the year takes up to six
// digits, so the arrow key moves on) hour, minute and AM or PM.
async function typeDate(page: Page, at: Date): Promise<void> {
	const pad = (n: number): string => String(n).padStart(2, "0");
	const hours = at.getHours() % 12 === 0 ? 12 : at.getHours() % 12;
	await page.keyboard.type(
		`${pad(at.getMonth() + 1)}${pad(at.getDate())}${at.getFullYear()}`,
	);
	await page.keyboard.press("ArrowRight");
	await page.keyboard.type(
		`${pad(hours)}${pad(at.getMinutes())}${at.getHours() < 12 ? "A" : "P"}`,
	);
}

test.describe("a11y-first-flow-keyboard @chromium-only", () => {
	test("brief, edit, thumbnail, schedule and approve with the keyboard alone", async ({
		page,
		request,
	}) => {
		test.setTimeout(600_000);
		const fake = running as Running;
		const api = await caller(request);
		await signInAsPerson(page);
		await countPointerEvents(page);

		let projectId = "";
		const name = `Keyboard short ${Date.now()}`;

		await test.step("create the project", async () => {
			await page.goto("/projects");
			await type(page, page.getByLabel("Name"), name);
			await page.keyboard.press("Enter");
			await expect(page).toHaveURL(/\/editor\/prj_[0-9a-hjkmnp-tv-z]{10}$/);
			projectId = page.url().split("/").pop() ?? "";
		});

		await test.step("answer the briefing", async () => {
			await page.goto(`/projects/${projectId}`);
			await type(
				page,
				page.getByLabel("Who is this video for?"),
				"Home bakers who want a first sourdough loaf",
			);
			await type(
				page,
				page.getByLabel("What should the viewer do or feel after watching?"),
				"Watch to the end and try the recipe",
			);
			await choose(
				page,
				page.getByLabel("Where will it be published?"),
				"YouTube Shorts",
			);
			await choose(page, page.getByLabel("Which format?"), "Vertical");
			await type(page, page.getByLabel("How long, in seconds?"), "10");
			await press(
				page,
				page.getByRole("button", { name: "Save the answers as the brief" }),
			);
			await expect(
				page.getByRole("status").filter({ hasText: "saved" }),
			).toBeVisible();
		});

		await test.step("import the footage, put it on the timeline and cut it to four seconds", async () => {
			await page.goto(`/editor/${projectId}`);
			await type(
				page,
				page.getByLabel("Path of a file to import"),
				testClip(0),
			);
			await press(page, page.getByRole("button", { name: "Import" }));
			const add = page.getByRole("button", { name: "Add to timeline" }).first();
			await expect(add).toBeEnabled({ timeout: 60_000 });
			await press(page, add);
			const clip = page.locator("[data-item-id]");
			await expect(clip).toHaveCount(1);
			await tabTo(page, clip.first());
			await page.keyboard.press("Enter");
			await tabTo(page, page.getByRole("slider", { name: "Playhead" }));
			for (let i = 0; i < 4; i++) await page.keyboard.press("PageUp");
			await press(page, page.getByRole("button", { name: "Trim end" }));
			await expect
				.poll(async () => {
					const shown = (await api(
						"GET",
						`/projects/${projectId}?detail=document`,
					)) as { document?: { items?: Record<string, { duration: number }> } };
					return Object.values(shown.document?.items ?? {})[0]?.duration;
				})
				.toBe(120);
		});

		await test.step("captions from the transcript", async () => {
			await tabTo(page, page.locator("[data-item-id]").first());
			await page.keyboard.press("Enter");
			const tabs = page.getByRole("tablist", { name: /^Properties$/ });
			await tabTo(page, tabs.getByRole("tab", { selected: true }));
			await arrowTo(
				page,
				tabs.getByRole("tab", { name: "Text", exact: true }),
				"ArrowRight",
			);
			await press(
				page,
				page.getByRole("button", { name: "Generate captions" }),
			);
			await expect(page.locator("[data-item-id]")).toHaveCount(2, {
				timeout: 60_000,
			});
		});

		await test.step("a thumbnail: a new image with a text layer", async () => {
			await page.goto("/edit-image");
			await type(page, page.getByLabel("Name").first(), "Keyboard cover");
			await page.keyboard.press("Enter");
			await expect(page).toHaveURL(/\/edit-image\/img_/);
			// The toolbar is one tab stop and the arrows walk it.
			await tabTo(page, page.locator("[data-tool]").first());
			await arrowTo(page, page.getByTestId("image-add-text"), "ArrowRight");
			await page.keyboard.press("Enter");
			await expect(page.getByRole("treeitem")).not.toHaveCount(0);
		});

		let exportJob = "";
		await test.step("export with the platform preset", async () => {
			await page.goto(`/editor/${projectId}`);
			await expect(page.locator("[data-item-id]")).toHaveCount(2);
			await press(
				page,
				page.getByRole("button", { name: "Export", exact: true }),
			);
			const dialog = page.getByRole("dialog");
			await tabTo(page, dialog.locator("#export-preset-youtube-shorts"));
			await page.keyboard.press("Space");
			await expect(dialog.getByText("The project fits.")).toBeVisible();
			await press(
				page,
				dialog.getByRole("button", { name: /^(Export 1 file|Start export)$/ }),
			);
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

		await test.step("connect the account of the network", async () => {
			await page.goto("/connections");
			await press(
				page,
				page.getByRole("button", { name: "Set up Fake network" }),
			);
			await type(page, page.getByLabel("Client id"), "first-flow-client");
			await type(
				page,
				page.getByLabel("Client secret"),
				"first-flow-client-secret",
			);
			await press(page, page.getByRole("button", { name: "Save and sign in" }));
			await expect(page.locator("[data-signed-in]")).toContainText(
				"First flow channel",
			);
		});

		await test.step("write the publication, schedule it, and a person approves it", async () => {
			await page.goto("/publish");
			await press(page, page.getByTestId("publish-new"));
			const dialog = page.getByTestId("publication-editor");
			await choose(page, dialog.getByLabel("Project"), "Keyboard short");
			await chooseNth(page, dialog.getByLabel("Exported video"), 1);
			await type(
				page,
				dialog.getByLabel("Title", { exact: true }),
				"Keyboard short",
			);
			await type(
				page,
				dialog.getByLabel("Caption"),
				"From starter to slice #sourdough",
			);
			await press(page, dialog.getByRole("button", { name: "Add an account" }));
			const panel = dialog.getByTestId("target-panel");
			await chooseNth(page, panel.getByLabel("Account", { exact: true }), 1);
			const at = new Date(Date.now() + 30_000);
			at.setSeconds(0, 0);
			at.setMinutes(at.getMinutes() + 1);
			await tabTo(page, panel.getByLabel("Date and time"));
			await typeDate(page, at);
			await expect(panel.getByLabel("Date and time")).toHaveValue(
				localField(at),
			);
			// A group of radios is one tab stop, and the arrow keys move inside it and check what they reach.
			const no = panel.getByLabel("No", { exact: true });
			const group = await no.getAttribute("name");
			await tabTo(
				page,
				panel.locator(`input[type=radio][name="${group}"]`).first(),
			);
			await arrowTo(page, no, "ArrowRight");
			await expect(no).toBeChecked();
			await press(
				page,
				dialog.getByRole("button", { name: "Save", exact: true }),
			);
			await expect(panel.getByTestId("target-state")).toHaveText("Draft");
			await press(page, panel.getByTestId("target-schedule"));
			await expect(dialog.getByRole("alert")).toHaveCount(0);
			await expect(panel.getByTestId("target-state")).toHaveText(
				"Waiting for approval",
			);
			await page.keyboard.press("Escape");
			await expect(dialog).toHaveCount(0);
			expect(await uploads(fake.socialUrl)).toHaveLength(0);

			await page.goto("/approvals");
			const item = page
				.locator("[data-approval-id]")
				.filter({ hasText: "Keyboard short" });
			await press(page, item.getByRole("button", { name: /^Approve/ }));
			await expect(item).toHaveCount(0);
			await page.goto("/publish");
			await expect(page.getByTestId("target-chip")).toContainText("Scheduled");
		});

		expect(await pointerEvents(page), "pointer events during the flow").toBe(0);
	});
});
