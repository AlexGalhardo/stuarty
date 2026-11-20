import { randomBytes } from "node:crypto";
import { caller, expect, test } from "./helpers";

// VV-329: text typed and not saved yet is not thrown away in silence. Leaving it behind (another item, another
// page) asks first; saving first makes the question go away.
test.describe("unsaved changes", () => {
	test("Write asks before another text replaces unsaved typing", async ({
		page,
		request,
	}) => {
		const api = await caller(request);
		const tag = randomBytes(3).toString("hex");
		const made = await api("POST", "/projects", { name: `Unsaved ${tag}` });
		const projectId = made.project_id as string;
		for (const title of [`First ${tag}`, `Second ${tag}`]) {
			await api("POST", `/projects/${projectId}/texts`, {
				title,
				text_kind: "copy",
				text: "saved text",
			});
		}
		await page.goto("/write");
		await page
			.getByRole("button", { name: new RegExp(`First ${tag}`) })
			.click();
		await page.getByLabel("Text (Markdown)").fill("UNSAVED PARAGRAPH XYZ");

		const questions: string[] = [];
		page.on("dialog", (dialog) => {
			questions.push(dialog.message());
			void dialog.dismiss();
		});
		await page
			.getByRole("button", { name: new RegExp(`Second ${tag}`) })
			.click();
		await expect.poll(() => questions.length).toBe(1);
		await expect(page.getByLabel("Text (Markdown)")).toHaveValue(
			"UNSAVED PARAGRAPH XYZ",
		);

		page.removeAllListeners("dialog");
		page.on("dialog", (dialog) => void dialog.accept());
		await page
			.getByRole("button", { name: new RegExp(`Second ${tag}`) })
			.click();
		await expect(page.getByLabel("Text (Markdown)")).toHaveValue("saved text");
	});

	test("a page change asks while the brief is unsaved and not after it is saved", async ({
		page,
		request,
	}) => {
		const api = await caller(request);
		const made = await api("POST", "/projects", {
			name: `Unsaved brief ${randomBytes(3).toString("hex")}`,
		});
		await page.goto(`/projects/${made.project_id as string}`);
		await page.locator("#brief-text").fill("UNSAVED BRIEF");

		let asked = 0;
		page.on("dialog", (dialog) => {
			asked += 1;
			void dialog.dismiss();
		});
		await page.getByRole("link", { name: "Back to projects" }).first().click();
		await expect.poll(() => asked).toBe(1);
		await expect(page.locator("#brief-text")).toHaveValue("UNSAVED BRIEF");

		await page.getByRole("button", { name: "Save brief" }).click();
		await expect
			.poll(async () => {
				const brief = (await api(
					"GET",
					`/projects/${made.project_id as string}/brief`,
				)) as {
					text: string;
				};
				return brief.text;
			})
			.toBe("UNSAVED BRIEF");
		await expect(
			page.getByRole("button", { name: "Save brief" }),
		).toBeDisabled();
		await page.getByRole("link", { name: "Back to projects" }).first().click();
		await expect(page).toHaveURL(/\/projects$/);
		expect(asked).toBe(1);
	});
});
