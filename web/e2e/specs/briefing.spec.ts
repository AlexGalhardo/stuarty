import AxeBuilder from "@axe-core/playwright";
import { caller, expect, test } from "./helpers";

// Epic 16 (SDD-005 section 5.12.1, RF-301, RF-302): the overview of a project asks the questions of the guided
// briefing, and the answers become the brief of the project. The questions and the rules are the core's.
test.describe("guided briefing", () => {
	test("the answers become the brief, and the form is filled again from it", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		const created = await call("POST", "/projects", {
			name: `Briefing ${Date.now()}`,
		});
		const id = created.project_id as string;

		await page.goto(`/projects/${id}`);
		await expect(
			page.getByRole("heading", { name: "Guided briefing" }),
		).toBeVisible();

		await page.getByLabel("Who is this video for?").fill("indie developers");
		await page
			.getByLabel("What should the viewer do or feel after watching?")
			.fill("sign up for the newsletter");
		await page
			.getByLabel("Where will it be published?")
			.selectOption("youtube-shorts");
		await page.getByLabel("Which format?").selectOption("vertical-9x16");
		await page.getByLabel("How long, in seconds?").fill("30");
		await page
			.getByLabel("Any references to follow (links, creators, videos)?")
			.fill("# not a heading\nsecond line");
		await page
			.getByRole("button", { name: "Save the answers as the brief" })
			.click();
		await expect(page.getByRole("status")).toHaveText(
			"The brief was saved from your answers.",
		);

		const brief = (await call("GET", `/projects/${id}/brief`)) as {
			text: string;
		};
		expect(brief.text).toContain("# Brief\n\n## Audience\n\nindie developers");
		expect(brief.text).toContain("## Duration (seconds)\n\n30");
		// The free-form editor below shows the same text.
		await expect(page.locator("#brief-text")).toHaveValue(brief.text);

		// The form starts from what the brief holds.
		await page.reload();
		await expect(page.getByLabel("Who is this video for?")).toHaveValue(
			"indie developers",
		);
		await expect(
			page.getByLabel("Any references to follow (links, creators, videos)?"),
		).toHaveValue("# not a heading\nsecond line");
		await expect(page.getByLabel("How long, in seconds?")).toHaveValue("30");

		// An agent reads the same answers through the API.
		const asked = (await call(
			"GET",
			`/briefing/questions?project_id=${id}`,
		)) as {
			guided: boolean;
			answers: { id: string; value: string }[];
		};
		expect(asked.guided).toBe(true);
		expect(asked.answers.find((a) => a.id === "audience")?.value).toBe(
			"indie developers",
		);
	});

	test("the service refuses what is not an answer, and says which", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		const created = await call("POST", "/projects", {
			name: `Refused ${Date.now()}`,
		});
		const id = created.project_id as string;

		await page.goto(`/projects/${id}`);
		await page.getByLabel("Who is this video for?").fill("makers");
		await page
			.getByLabel("What should the viewer do or feel after watching?")
			.fill("   "); // white space passes the browser and is no answer for the core
		await page.getByLabel("Where will it be published?").selectOption("tiktok");
		await page.getByLabel("Which format?").selectOption("vertical-9x16");
		await page.getByLabel("How long, in seconds?").fill("30");
		await page
			.getByRole("button", { name: "Save the answers as the brief" })
			.click();
		await expect(page.getByRole("alert")).toContainText(
			'the question "goal" needs an answer',
		);
		const brief = (await call("GET", `/projects/${id}/brief`)) as {
			text: string;
		};
		expect(brief.text).toBe("");
	});

	test("a brief written by hand is replaced only when the person says so", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		const created = await call("POST", "/projects", {
			name: `By hand ${Date.now()}`,
		});
		const id = created.project_id as string;
		await call("PUT", `/projects/${id}/brief`, { text: "My own plan." });

		await page.goto(`/projects/${id}`);
		await expect(
			page.getByText("The brief of this production was written by hand."),
		).toBeVisible();

		await page.getByLabel("Who is this video for?").fill("makers");
		await page
			.getByLabel("What should the viewer do or feel after watching?")
			.fill("sell");
		await page.getByLabel("Where will it be published?").selectOption("tiktok");
		await page.getByLabel("Which format?").selectOption("vertical-9x16");
		await page.getByLabel("How long, in seconds?").fill("20");

		const save = page.getByRole("button", {
			name: "Save the answers as the brief",
		});
		await expect(save).toBeDisabled();
		expect(
			((await call("GET", `/projects/${id}/brief`)) as { text: string }).text,
		).toBe("My own plan.");

		await page.getByLabel("Replace the brief I wrote").check();
		await save.click();
		await expect(page.getByRole("status")).toBeVisible();
		expect(
			((await call("GET", `/projects/${id}/brief`)) as { text: string }).text,
		).toContain("# Brief");
	});

	test("the form is in Portuguese when the language is", async ({
		page,
		request,
	}) => {
		await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
		const call = await caller(request);
		const created = await call("POST", "/projects", {
			name: `Briefing pt ${Date.now()}`,
		});

		await page.goto(`/projects/${created.project_id as string}`);
		await expect(
			page.getByRole("heading", { name: "Briefing guiado" }),
		).toBeVisible();
		await expect(page.getByLabel("Para quem é este vídeo?")).toBeVisible();
		await expect(
			page.getByRole("button", { name: "Salvar as respostas como briefing" }),
		).toBeVisible();
	});

	for (const theme of ["light", "dark"] as const) {
		test(`axe finds nothing on the briefing in ${theme}`, async ({
			page,
			request,
		}) => {
			await page.addInitScript((th) => {
				localStorage.setItem("theme", th);
			}, theme);
			const call = await caller(request);
			const created = await call("POST", "/projects", {
				name: `Axe briefing ${Date.now()}`,
			});
			await page.goto(`/projects/${created.project_id as string}`);
			await expect(
				page.getByRole("heading", { name: "Guided briefing" }),
			).toBeVisible();
			const { violations } = await new AxeBuilder({ page })
				.withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
				.analyze();
			expect(violations.map((v) => v.id)).toEqual([]);
		});
	}
});
