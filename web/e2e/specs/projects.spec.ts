import {
	caller,
	DESCRIPTION,
	expect,
	fillProjectForm,
	GOAL,
	listProjects,
	test,
} from "./helpers";

// RF-001 and epic 56: a project is made with a name, a description and a SMART goal; the page explains each field,
// the button waits for every field to be valid, and the size is asked when a video is added.
test.describe("projects-page", () => {
	test("the button waits for a valid form and each error shows under its field", async ({
		page,
	}) => {
		await page.goto("/projects");
		const create = page.getByRole("button", { name: "Create project" });
		await expect(create).toBeDisabled();

		const name = page.getByLabel("Name", { exact: true });
		await name.fill("abc");
		await expect(
			page.getByRole("alert").filter({ hasText: "at least 4 characters" }),
		).toBeVisible();
		await expect(name).toHaveAttribute("aria-invalid", "true");
		await name.fill("Two  spaces");
		await expect(
			page.getByRole("alert").filter({ hasText: "single spaces" }),
		).toBeVisible();

		await fillProjectForm(page, `E2E ${Date.now()}`);
		await expect(create).toBeEnabled();

		await page.getByLabel("Description", { exact: true }).fill("too short");
		await expect(create).toBeDisabled();
		await expect(
			page.getByRole("alert").filter({ hasText: "at least 64 characters" }),
		).toBeVisible();

		await page.getByLabel("Description", { exact: true }).fill(DESCRIPTION);
		await page.getByLabel("Specific", { exact: true }).fill("short");
		await expect(create).toBeDisabled();
		await page.getByLabel("Specific", { exact: true }).fill(GOAL.specific);
		await page.getByLabel("Due date", { exact: true }).fill("2001-01-01");
		await expect(create).toBeDisabled();
		await expect(
			page.getByRole("alert").filter({ hasText: "before today" }),
		).toBeVisible();
		await page.getByLabel("Due date", { exact: true }).fill(GOAL.due);
		await expect(create).toBeEnabled();
	});

	test("every field explains itself with a tip and a worked example", async ({
		page,
	}) => {
		await page.goto("/projects");
		await expect(page.getByPlaceholder("Launch teaser")).toBeVisible();
		await expect(
			page.getByPlaceholder("Publish one teaser that explains the course"),
		).toBeVisible();

		const tip = page.getByRole("button", { name: "What to write here" }).nth(2); // Specific
		await tip.focus();
		await expect(page.getByText("One concrete result")).toBeVisible();
		await expect(
			page.getByRole("button", { name: "What to write here" }),
		).toHaveCount(8);
	});

	test("creating a project makes a project with no timeline, and a video asks for the size", async ({
		page,
		request,
	}) => {
		const label = `E2E ${Date.now()}`;

		await page.goto("/projects");
		await fillProjectForm(page, label);
		await page.getByRole("button", { name: "Create project" }).click();
		await expect(page).toHaveURL(/\/projects\/prj_[0-9a-hjkmnp-tv-z]{10}$/);

		const created = (await listProjects(request)).filter(
			(p) => p.name === label,
		);
		expect(created).toHaveLength(1);
		expect(created[0].timelines).toBe(0);

		await expect(page.getByText(GOAL.specific)).toBeVisible();
		await page.getByLabel("Title", { exact: true }).fill("Main cut");
		await page.getByLabel("Size").selectOption("1:1");
		await page.getByRole("button", { name: "Add item" }).click();

		await expect
			.poll(async () => {
				const again = (await listProjects(request)).filter(
					(p) => p.name === label,
				);
				return [again[0]?.width, again[0]?.height];
			})
			.toEqual([1080, 1080]);

		await page.goto("/projects");
		await expect(
			page.getByRole("link", { name: new RegExp(`Open.*${label}`) }),
		).toBeVisible();
	});

	test("a project made before descriptions opens with a notice and is completed", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		const templates = (await call("GET", "/templates")) as {
			templates: { template_id: string }[];
		};
		const label = `Old ${Date.now()}`;
		const made = (await call(
			"POST",
			`/templates/${templates.templates[0].template_id}/instantiate`,
			{ name: label },
		)) as { project_id: string };

		await page.goto(`/projects/${made.project_id}`);
		await expect(page.getByTestId("complete-notice")).toBeVisible();

		await page.getByRole("button", { name: "Complete the project" }).click();
		const dialog = page.getByRole("dialog");
		await dialog.getByLabel("Name", { exact: true }).fill(`Done ${Date.now()}`);
		await fillProjectFormIn(dialog);
		await dialog.getByRole("button", { name: "Save the project" }).click();

		await expect(page.getByTestId("complete-notice")).toHaveCount(0);
		await expect(page.getByText(GOAL.measurable)).toBeVisible();
	});

	test("an open link goes to the page of the project", async ({
		page,
		request,
	}) => {
		const label = `E2E open ${Date.now()}`;
		const call = await caller(request);
		const made = (await call("POST", "/projects", { name: label })) as {
			project_id: string;
		};

		await page.goto("/projects");
		await page
			.getByRole("link", { name: new RegExp(`Open.*${label}`) })
			.click();
		await expect(page).toHaveURL(new RegExp(`/editor/${made.project_id}$`));
	});
});

async function fillProjectFormIn(
	scope: import("@playwright/test").Locator,
): Promise<void> {
	await scope.getByLabel("Description", { exact: true }).fill(DESCRIPTION);
	await scope.getByLabel("Specific", { exact: true }).fill(GOAL.specific);
	await scope.getByLabel("Measurable", { exact: true }).fill(GOAL.measurable);
	await scope.getByLabel("Achievable", { exact: true }).fill(GOAL.achievable);
	await scope.getByLabel("Relevant", { exact: true }).fill(GOAL.relevant);
	await scope.getByLabel("Time-bound", { exact: true }).fill(GOAL.time_bound);
	await scope.getByLabel("Due date", { exact: true }).fill(GOAL.due);
}
