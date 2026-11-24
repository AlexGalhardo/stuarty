import type { Page } from "@playwright/test";
import {
	caller,
	expect,
	listProjects,
	openProjectForm,
	test,
	validName,
} from "./helpers";

// Epic 79: a new project starts with what to create, in four equal cards. A format is asked only for a video and a
// type only for a text; a text project creates no timeline and no format; the project is created at the end of the
// briefing.

interface Described {
	kind: string;
	items: { kind: string; text_kind?: string; timeline_id?: string }[];
}

const PROJECT_URL = /\/projects\/prj_[0-9a-hjkmnp-tv-z]{10}$/;

async function create(page: Page): Promise<string> {
	await page.getByRole("button", { name: "Create project" }).click();
	await page.waitForURL(PROJECT_URL);
	return page.url().split("/").pop() ?? "";
}

test.describe("new project kinds", () => {
	test("the dialog offers four equal cards and asks the extra only where it applies", async ({
		page,
	}) => {
		await page.goto("/projects");
		await page.getByRole("button", { name: "New project" }).click();
		const dialog = page.getByRole("dialog", { name: "New project" });
		const cards = dialog.locator("[data-choice]");
		await expect(cards).toHaveCount(4);
		const sizes = await cards.evaluateAll((els) =>
			els.map((el) => {
				const box = el.getBoundingClientRect();
				return [Math.round(box.width), Math.round(box.height)];
			}),
		);
		expect(new Set(sizes.map((s) => s.join("x"))).size).toBe(1);
		for (const title of ["Text", "Video", "Image", "Decide later"]) {
			await expect(
				dialog.getByRole("radio", { name: new RegExp(`^${title}`) }),
			).toHaveCount(1);
		}
		// Nothing is chosen for the person.
		await expect(dialog.getByRole("radio", { checked: true })).toHaveCount(0);
		await expect(dialog.getByLabel("Format")).toHaveCount(0);
		await expect(dialog.getByText("What kind of text?")).toHaveCount(0);

		await cards.nth(1).click();
		await expect(dialog.getByLabel("Format")).toBeVisible();
		await expect(dialog.getByText("What kind of text?")).toHaveCount(0);
		await cards.nth(0).click();
		await expect(dialog.getByLabel("Format")).toHaveCount(0);
		for (const type of ["Blog post", "Copy", "Script", "Caption"]) {
			await expect(
				dialog.getByRole("radio", { name: type, exact: true }),
			).toBeVisible();
		}
		await cards.nth(2).click();
		await expect(dialog.getByLabel("Format")).toHaveCount(0);
		await expect(dialog.getByText("What kind of text?")).toHaveCount(0);
	});

	test("the briefing follows the kind: its lead and its examples", async ({
		page,
	}) => {
		await page.goto("/projects");
		await openProjectForm(page, "text");
		const dialog = page.getByRole("dialog");
		await expect(dialog).toContainText("what the text is about");
		await expect(
			dialog.getByPlaceholder(/A blog post that explains/),
		).toBeVisible();
		await page.getByRole("button", { name: "Back" }).click();
		await page.locator('[data-choice="video"]').click();
		await page
			.getByRole("button", { name: "Continue to the briefing" })
			.click();
		await expect(dialog).toContainText("what the video is about");
		await expect(
			dialog.getByPlaceholder(/A 30 second vertical video/),
		).toBeVisible();
		await page.getByRole("button", { name: "Back" }).click();
		await page.locator('[data-choice="undecided"]').click();
		await page
			.getByRole("button", { name: "Continue to the briefing" })
			.click();
		await expect(dialog).toContainText("You choose the format after");
		await expect(dialog.getByPlaceholder(/A short teaser/)).toBeVisible();
	});

	test("a text project creates no timeline and appears as a text card", async ({
		page,
		request,
	}) => {
		const name = validName(`Kinds copy ${Date.now()}`);
		await page.goto("/projects");
		await page.getByRole("button", { name: "New project" }).click();
		await page.locator('[data-choice="text"]').click();
		await page.getByRole("radio", { name: "Copy", exact: true }).check();
		await page
			.getByRole("button", { name: "Continue to the briefing" })
			.click();
		await page.getByLabel("Name", { exact: true }).fill(name);
		await fillBriefing(page);
		const id = await create(page);

		const call = await caller(request);
		const described = (await call(
			"GET",
			`/projects/${id}/describe`,
		)) as unknown as Described;
		expect(described.kind).toBe("text");
		expect(described.items.filter((i) => i.kind === "timeline")).toHaveLength(
			0,
		);
		expect(described.items.find((i) => i.kind === "text")?.text_kind).toBe(
			"copy",
		);
		const listed = (await listProjects(request)).find((p) => p.id === id);
		expect(listed).toBeDefined();
		expect((listed as unknown as { timelines: number }).timelines).toBe(0);

		await page.goto("/projects");
		const card = page.locator(`[data-project-card='${id}']`);
		await expect(card).toHaveAttribute("data-kind", "text");
		await expect(card.locator("img")).toHaveCount(0);
		await expect(card.getByText("No text written yet.")).toBeVisible();
		await expect(card.getByRole("button", { name: /Export/ })).toHaveCount(0);

		// Once the text has words, the card says what the service counted.
		const words = Array.from({ length: 250 }, (_, i) => `word${i}`).join(" ");
		await call("POST", `/projects/${id}/items`, {
			kind: "text",
			title: "Draft",
			text: `Opening line of the copy. ${words}`,
			text_kind: "copy",
		});
		await page.goto("/projects");
		await expect(card).toContainText("Opening line of the copy.");
		await expect(card).toContainText("Copy");
		await expect(card).toContainText("255 words");
		await expect(card).toContainText("2 min read");
		await expect(card.locator("img")).toHaveCount(0);
	});

	test("a video project is made with the format that was chosen", async ({
		page,
		request,
	}) => {
		const name = validName(`Kinds video ${Date.now()}`);
		await page.goto("/projects");
		await page.getByRole("button", { name: "New project" }).click();
		await page.locator('[data-choice="video"]').click();
		await page.getByLabel("Format").selectOption("1:1");
		await page
			.getByRole("button", { name: "Continue to the briefing" })
			.click();
		await page.getByLabel("Name", { exact: true }).fill(name);
		await fillBriefing(page);
		const id = await create(page);

		const call = await caller(request);
		const described = (await call(
			"GET",
			`/projects/${id}/describe`,
		)) as unknown as Described;
		expect(described.kind).toBe("video");
		expect(described.items.filter((i) => i.kind === "timeline")).toHaveLength(
			1,
		);
		await expect
			.poll(async () => {
				const p = (await listProjects(request)).find((x) => x.id === id);
				return [p?.width, p?.height];
			})
			.toEqual([1080, 1080]);
	});

	test("an image and a project to decide later create nothing but the project", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		for (const [choice, kind] of [
			["image", "image"],
			["undecided", "undecided"],
		] as const) {
			await page.goto("/projects");
			await page.getByRole("button", { name: "New project" }).click();
			await page.locator(`[data-choice="${choice}"]`).click();
			await page
				.getByRole("button", { name: "Continue to the briefing" })
				.click();
			await page
				.getByLabel("Name", { exact: true })
				.fill(validName(`Kinds ${choice} ${Date.now()}`));
			await fillBriefing(page);
			const id = await create(page);

			const described = (await call(
				"GET",
				`/projects/${id}/describe`,
			)) as unknown as Described;
			expect(described.kind).toBe(kind);
			expect(described.items.filter((i) => i.kind !== "brief")).toHaveLength(0);
		}
	});

	test("Back returns to the cards with the choice kept", async ({ page }) => {
		await page.goto("/projects");
		await page.getByRole("button", { name: "New project" }).click();
		await page.locator('[data-choice="image"]').click();
		await page
			.getByRole("button", { name: "Continue to the briefing" })
			.click();
		await page.getByRole("button", { name: "Back" }).click();
		await expect(page.getByRole("radio", { name: /^Image/ })).toBeChecked();
	});
});

async function fillBriefing(page: Page): Promise<void> {
	const fields: [string, string][] = [
		[
			"Description",
			"A short piece made for the end to end tests: it explains one idea to the people who follow the channel, in a few words.",
		],
		["Specific", "Publish one piece that explains the idea"],
		["Measurable", "At least one hundred readers in the first week"],
		["Achievable", "The notes and the outline already exist"],
		["Relevant", "It brings the first audience to the channel"],
		["Time-bound", "Write on Monday and publish on Tuesday"],
		["Due date", "2999-12-31"],
	];
	for (const [label, value] of fields) {
		await page.getByLabel(label, { exact: true }).fill(value);
	}
}
