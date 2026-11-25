import type { Page } from "@playwright/test";
import { seedApprovals } from "../support/approvals";
import { caller, expect, projectBody, test } from "./helpers";

// Epic 79: the home says what to do next. With no project it teaches the first step and names no single format;
// with one it shows the path the service drew for that project, and what waits for the person.

// The workspace of the e2e server is shared with the other specs, so the list the home reads is narrowed to what
// the spec made (or to nothing): the server's own answer, only filtered.
async function showOnly(page: Page, ids: readonly string[]): Promise<void> {
	await page.route("**/api/v1/projects", async (route) => {
		if (route.request().method() !== "GET") return route.fallback();
		const response = await route.fetch();
		const body = (await response.json()) as {
			projects: { id: string }[];
		};
		await route.fulfill({
			response,
			json: {
				...body,
				projects: body.projects.filter((p) => ids.includes(p.id)),
			},
		});
	});
}

test.describe("home trail", () => {
	test("an empty workspace teaches the first step and does not assume a video", async ({
		page,
	}) => {
		await showOnly(page, []);
		await page.goto("/");

		const empty = page.getByRole("region", { name: "Make your first project" });
		await expect(empty).toBeVisible();
		for (const word of ["text", "video", "image"]) {
			await expect(empty).toContainText(word);
		}
		const guide = page.getByRole("region", { name: "How it works" });
		await expect(guide.getByRole("listitem")).toHaveCount(3);
		// The first step names every format, so a person who writes is not told to make a video.
		for (const word of ["text", "image"]) {
			await expect(guide).toContainText(word);
		}
		await expect(
			page.getByRole("link", { name: "New project" }).first(),
		).toHaveAttribute("href", "/projects?new=1");
		await expect(page.getByTestId("home-progress")).toHaveCount(0);
	});

	test("a text project shows its five steps and its next step", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		const created = await call("POST", "/projects", {
			...projectBody(`Home text ${Date.now()}`),
			kind: "text",
		});
		const id = created.project_id as string;
		await showOnly(page, [id]);
		await page.goto("/");

		const progress = page.getByTestId("home-progress");
		await expect(progress).toBeVisible();
		await expect(progress.locator('[data-kind="text"]')).toBeVisible();
		const steps = progress.getByRole("list", { name: "Path to publication" });
		await expect(steps.getByRole("listitem")).toHaveCount(5);
		for (const title of [
			"Briefing",
			"Writing",
			"Review",
			"Approval",
			"Publishing",
		]) {
			await expect(steps).toContainText(title);
		}
		await expect(steps).not.toContainText("Script");
		await expect(steps).not.toContainText("Edit");
		await expect(page.getByTestId("home-at")).toContainText("You are at");
		await expect(page.getByTestId("home-waiting")).toBeVisible();

		await progress.getByRole("link", { name: /Open project/ }).click();
		await expect(page).toHaveURL(new RegExp(`/projects/${id}$`));
	});

	test("the home greets the person by name in the morning and lists what waits as rows", async ({
		page,
	}) => {
		const stamp = Date.now();
		seedApprovals([
			`Publish the first post ${stamp}`,
			`Publish the second post ${stamp}`,
		]);
		await page.clock.setFixedTime(new Date(2026, 10, 24, 9, 30));
		await page.goto("/");

		await expect(page.getByRole("heading", { level: 1 })).toHaveText(
			"Good morning, Stuart.",
		);
		const waiting = page.getByTestId("home-waiting");
		await expect(waiting.getByTestId("home-waiting-row").first()).toBeVisible();
		// An approval is worded by its kind: the English summary the server wrote is not the title of the row.
		await expect(waiting).toContainText("Publish");
		await expect(waiting).not.toContainText(`Publish the second post ${stamp}`);
		await expect(
			waiting.getByRole("link", { name: "Open queue" }),
		).toHaveAttribute("href", "/approvals");
		await expect(page.getByTestId("home-studio")).toContainText("Your studio");
		for (const area of [
			"projects",
			"writing",
			"assets",
			"agent",
			"publish",
			"connections",
		]) {
			await expect(page.getByTestId(`home-area-${area}`)).toBeVisible();
		}
	});

	test("a video project shows the steps of a video", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		const created = await call("POST", "/projects", {
			...projectBody(`Home video ${Date.now()}`),
			kind: "video",
		});
		const id = created.project_id as string;
		await showOnly(page, [id]);
		await page.goto("/");

		const steps = page
			.getByTestId("home-progress")
			.getByRole("list", { name: "Path to publication" });
		for (const title of ["Script", "Footage", "Edit", "Finishing"]) {
			await expect(steps).toContainText(title);
		}
	});
});
