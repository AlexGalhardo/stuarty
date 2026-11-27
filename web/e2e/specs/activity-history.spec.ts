import { caller, expect, makeProject, projectBody, test } from "./helpers";

// End-to-end verification for epic 111: global Activity History in the sidebar
// (/history) and per-project History section on /projects/{id}, in both en and pt-BR.
test.describe("activity history (epic 111)", () => {
	test("sidebar History button opens /history with global actions and working filters", async ({
		page,
	}) => {
		const person = await caller(page.request);
		const agent = await caller(page.request, "chat-history");
		const a = await makeProject(agent, "History Project Alpha");
		const b = await makeProject(person, "History Project Beta");
		const alpha = a.project_id as string;
		const beta = b.project_id as string;

		await page.goto("/");
		const navHistory = page.getByTestId("nav-history");
		await expect(navHistory).toBeVisible();
		await navHistory.click();

		await expect(page).toHaveURL(/\/history$/);
		await expect(
			page.getByRole("heading", { name: "History", level: 1 }),
		).toBeVisible();

		const timeline = page.getByTestId("activity-timeline");
		const alphaLinks = timeline.getByRole("link", {
			name: "History Project Alpha",
		});
		const betaLinks = timeline.getByRole("link", {
			name: "History Project Beta",
		});
		await expect(timeline).toBeVisible();
		await expect(alphaLinks.first()).toBeVisible();
		await expect(betaLinks.first()).toBeVisible();

		// Filtering by Project Alpha keeps only Alpha's entries.
		await timeline.getByTestId("history-filter-actor").selectOption("agent");
		await expect(alphaLinks.first()).toBeVisible();
		await expect(betaLinks).toHaveCount(0);
		await timeline.getByTestId("history-filter-actor").selectOption("all");
		await timeline.getByTestId("history-filter-project").selectOption(alpha);
		await expect(alphaLinks.first()).toBeVisible();
		await expect(betaLinks).toHaveCount(0);

		// Per-project page shows only Project Beta's history with no project dropdown.
		await page.goto(`/projects/${beta}`);
		const projectHistory = page.getByTestId("project-activity-timeline");
		await expect(projectHistory).toBeVisible();
		await expect(
			projectHistory.getByTestId("history-entry").first(),
		).toBeVisible();
		await expect(
			projectHistory.getByTestId("history-filter-project"),
		).toHaveCount(0);
		const head = await person("GET", `/projects/${beta}?detail=document`);
		await agent("POST", `/projects/${beta}/edit`, {
			base_revision: head.revision,
			operations: [{ op: "project.rename", args: { name: "History Edited" } }],
		});
		await projectHistory.getByRole("button", { name: "Refresh" }).click();
		await projectHistory
			.getByTestId("history-filter-domain")
			.selectOption("video");
		await expect(projectHistory.getByTestId("history-entry")).toHaveCount(1);
		await expect(projectHistory).toContainText("Video edited");
	});

	test("History page renders fully in Brazilian Portuguese (pt-BR)", async ({
		page,
	}) => {
		await page.addInitScript(() => {
			localStorage.setItem("vv.lang", "pt-BR");
		});
		const person = await caller(page.request);
		await person("POST", "/projects", projectBody("Projeto Histórico PT"));

		await page.goto("/history");
		await expect(page.locator("html")).toHaveAttribute("lang", "pt-BR");
		await expect(
			page.getByRole("heading", { name: "Histórico", level: 1 }),
		).toBeVisible();
		const timeline = page.getByTestId("activity-timeline");
		await expect(timeline).toContainText("Projeto criado");
		await expect(timeline).toContainText("Projeto Histórico PT");
	});
});
