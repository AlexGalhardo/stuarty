import AxeBuilder from "@axe-core/playwright";
import { caller, expect, listProjects, test } from "./helpers";

// Epic 06 (SDD-012 section 2.1, SDD-005 section 16.1): the project is a production. Its overview shows the brief
// and the items, a project can hold two timelines, and the card menu duplicates a project (VV-225).
test.describe("project overview", () => {
	test("the brief and the items are written and listed, and a second timeline opens in the editor", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		const label = `Overview ${Date.now()}`;
		const created = await call("POST", "/projects", { name: label });
		const id = created.project_id as string;

		await page.goto(`/projects/${id}`);
		await expect(page.getByRole("heading", { level: 1 })).toContainText(label);

		await page.locator("#brief-text").fill("A teaser for the launch.");
		await page.getByRole("button", { name: "Save brief" }).click();
		await expect(
			page.getByRole("button", { name: "Save brief" }),
		).toBeDisabled();
		const brief = (await call("GET", `/projects/${id}/brief`)) as {
			text: string;
		};
		expect(brief.text).toBe("A teaser for the launch.");

		await page.getByLabel("Title").first().fill("Wide cut");
		await page.getByLabel("Size").selectOption("16:9");
		await page.getByRole("button", { name: "Add item" }).click();
		const wide = page.locator("[data-item]").filter({ hasText: "Wide cut" });
		await expect(wide).toBeVisible();
		await expect(page.locator("[data-item]")).toHaveCount(2);

		const described = (await call("GET", `/projects/${id}/describe`)) as {
			items: { kind: string; title: string; timeline_id?: string }[];
		};
		const timelines = described.items.filter((i) => i.kind === "timeline");
		expect(timelines.map((t) => t.title)).toContain("Wide cut");
		expect(timelines).toHaveLength(2);
		const second = timelines.find((t) => t.title === "Wide cut");
		await expect(wide.locator("a")).toHaveAttribute(
			"href",
			`/editor/${second?.timeline_id}`,
		);

		await wide.getByRole("button", { name: /Move to trash/ }).click();
		await expect(page.locator("[data-item]")).toHaveCount(1);
		await page.getByRole("button", { name: /Restore/ }).click();
		await expect(page.locator("[data-item]")).toHaveCount(2);
	});

	test("Duplicate in the card menu adds a copy with its timelines", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		const label = `Original ${Date.now()}`;
		const created = await call("POST", "/projects", { name: label });
		const id = created.project_id as string;
		await call("POST", `/projects/${id}/items`, {
			kind: "timeline",
			title: "Second",
			preset: "1:1",
		});

		await page.goto("/projects");
		const card = page.locator(`[data-project-card='${id}']`);
		await expect(card).toContainText("Timelines: 2");
		await card.getByRole("button", { name: /Actions for project/ }).click();
		await page.getByRole("menuitem", { name: "Duplicate" }).click();

		await expect(
			page.locator("[data-project-card]").filter({ hasText: `${label} copy` }),
		).toBeVisible();
		const copies = (await listProjects(request)).filter(
			(p) => p.name === `${label} copy`,
		);
		expect(copies).toHaveLength(1);
	});

	for (const theme of ["light", "dark"] as const) {
		test(`axe finds nothing on the overview in ${theme}`, async ({
			page,
			request,
		}) => {
			await page.addInitScript((th) => {
				localStorage.setItem("theme", th);
			}, theme);
			const call = await caller(request);
			const created = await call("POST", "/projects", {
				name: `Axe ${Date.now()}`,
			});
			await page.goto(`/projects/${created.project_id as string}`);
			await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
			const { violations } = await new AxeBuilder({ page })
				.withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
				.analyze();
			expect(violations.map((v) => v.id)).toEqual([]);
		});
	}
});
