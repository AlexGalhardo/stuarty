import type { APIRequestContext } from "@playwright/test";
import { caller, expect, projectBody, test } from "./helpers";

// Epic 79: the page of a project draws the path the service computed for it, with who does each step, its tool,
// its state and its cost, lets a person skip an optional step, and adds a video to a text with no reload and
// without drawing the finished steps again.

async function project(
	request: APIRequestContext,
	kind: "text" | "video" | "image" | undefined,
	label: string,
): Promise<{ id: string; call: Awaited<ReturnType<typeof caller>> }> {
	const call = await caller(request);
	const created = await call("POST", "/projects", {
		...projectBody(`${label} ${Date.now()}`),
		...(kind === undefined ? {} : { kind }),
	});
	return { id: created.project_id as string, call };
}

interface Workflow {
	steps: { id: string; state: string }[];
}

test.describe("project workflow", () => {
	test("adding a video to a text adds its steps and leaves the finished ones where they are", async ({
		page,
		request,
	}) => {
		const { id, call } = await project(request, "text", "Flow text");
		await call("PUT", `/projects/${id}/brief`, {
			text: "# Brief\n\nA post for the people who follow the channel.",
		});
		await call("POST", `/projects/${id}/items`, {
			kind: "text",
			title: "The post",
			text: "A first paragraph with enough words to count as the writing of the post.",
			text_kind: "post",
		});

		await page.goto(`/projects/${id}`);
		const trail = page.getByRole("list", { name: "Path to publication" });
		const steps = trail.locator("[data-step]");
		await expect(steps).toHaveCount(5);
		for (const step of ["briefing", "writing"]) {
			await expect(trail.locator(`[data-step="${step}"]`)).toHaveAttribute(
				"data-state",
				"done",
			);
		}
		await expect(trail.locator('[data-step="script"]')).toHaveCount(0);

		// Every step says who does it, with which tool, its state and its cost; what the service did not say reads
		// "Not reported".
		const writing = trail.locator('[data-step="writing"]');
		await expect(writing).toContainText("Done");
		await expect(writing).toContainText("You and the agent");
		await expect(writing).toContainText("Write the text");
		await expect(writing).toContainText("Cost");
		await expect(writing).toContainText("Not reported");

		// A mark on the page and on a finished step: a reload or a new draw of the step would lose them.
		await page.evaluate(() => {
			(window as unknown as { kept: string }).kept = "same page";
		});
		await trail
			.locator('[data-step="briefing"]')
			.evaluate((el) => el.setAttribute("data-mark", "kept"));
		await trail
			.locator('[data-step="writing"]')
			.evaluate((el) => el.setAttribute("data-mark", "kept"));

		await page.getByRole("button", { name: "Add media" }).click();
		const dialog = page.getByRole("dialog", { name: "Add a video" });
		await dialog.getByLabel("Format").selectOption("16:9");
		await dialog.getByRole("button", { name: "Add the video" }).click();

		await expect(trail.locator('[data-step="script"]')).toBeVisible();
		for (const step of ["recording", "editing", "finishing"]) {
			await expect(trail.locator(`[data-step="${step}"]`)).toBeVisible();
		}
		await expect(steps).toHaveCount(9);
		expect(
			await page.evaluate(() => (window as unknown as { kept: string }).kept),
		).toBe("same page");
		for (const step of ["briefing", "writing"]) {
			await expect(trail.locator(`[data-step="${step}"]`)).toHaveAttribute(
				"data-mark",
				"kept",
			);
			await expect(trail.locator(`[data-step="${step}"]`)).toHaveAttribute(
				"data-state",
				"done",
			);
		}
		// The questions about a video join the page now that it holds one, and the service agrees on the steps.
		await expect(
			page.getByRole("heading", { name: "Guided briefing" }),
		).toBeVisible();
		const described = (await call("GET", `/projects/${id}/describe`)) as {
			kind?: string;
			workflow: Workflow;
		};
		expect(described.workflow.steps).toHaveLength(9);
	});

	test("an optional step is skipped and put back", async ({
		page,
		request,
	}) => {
		const { id, call } = await project(request, "video", "Flow skip");
		await call("POST", `/projects/${id}/items`, {
			kind: "timeline",
			title: "Main cut",
		});

		await page.goto(`/projects/${id}`);
		const recording = page.locator('[data-step="recording"]');
		await expect(recording).toContainText("Optional");
		await expect(
			page
				.locator('[data-step="script"]')
				.getByRole("button", { name: /^Skip/ }),
		).toHaveCount(0);

		await page.getByTestId("skip-recording").click();
		await expect(recording).toHaveAttribute("data-state", "skipped");
		await expect(recording).toContainText("Skipped");
		const after = (await call("GET", `/projects/${id}/describe`)) as {
			workflow: Workflow;
		};
		expect(after.workflow.steps.find((s) => s.id === "recording")?.state).toBe(
			"skipped",
		);

		await page.getByTestId("restore-recording").click();
		await expect(recording).not.toHaveAttribute("data-state", "skipped");
		await expect(page.getByTestId("skip-recording")).toBeVisible();
	});

	test("the guided questions about a video are asked of a video and of no other kind", async ({
		page,
		request,
	}) => {
		const text = await project(request, "text", "Flow ask text");
		await page.goto(`/projects/${text.id}`);
		await expect(page.getByTestId("workflow")).toBeVisible();
		await expect(
			page.getByRole("heading", { name: "Guided briefing" }),
		).toHaveCount(0);
		await expect(page.getByText(/this video/i)).toHaveCount(0);
		// The brief is written by hand.
		await expect(page.locator("#brief-text")).toBeVisible();

		const video = await project(request, "video", "Flow ask video");
		await page.goto(`/projects/${video.id}`);
		await expect(
			page.getByRole("heading", { name: "Guided briefing" }),
		).toBeVisible();
	});

	test("a project not decided yet asks to choose and offers a video", async ({
		page,
		request,
	}) => {
		const { id } = await project(request, undefined, "Flow later");
		await page.goto(`/projects/${id}`);
		const trail = page.getByRole("list", { name: "Path to publication" });
		await expect(trail.locator("[data-step]")).toHaveCount(2);
		await expect(trail.locator('[data-step="choice"]')).toBeVisible();
		await expect(page.getByRole("button", { name: "Add media" })).toBeVisible();
	});
});
