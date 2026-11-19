import { mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { expect, test } from "./helpers";

test.describe.configure({ timeout: 120_000, mode: "serial" });

// A name that no earlier run of the suite used: the workspace of the server lasts for the whole run.
function unique(prefix: string): string {
	return `${prefix}-${Date.now().toString(36)}`;
}

// A skill folder on the machine of the server, such as a person downloads and unpacks.
function skillFolder(name: string): string {
	const dir = join(tmpdir(), "stuarty-e2e-skills", name);
	mkdirSync(join(dir, "references"), { recursive: true });
	writeFileSync(
		join(dir, "SKILL.md"),
		`---\nname: ${name}\ndescription: Write the opening of a short video.\nlicense: MIT\n---\n# Hooks\n\nSee [patterns](references/patterns.md).\n`,
	);
	writeFileSync(
		join(dir, "references", "patterns.md"),
		"Open with a question.\n",
	);
	return dir;
}

// SDD-018 section 14: write a prompt and render it, import a skill, review it and export it, and keep a
// memory note, all from the three sections of the dashboard.
test.describe("library", () => {
	test("a prompt is written and rendered with the values of its variables", async ({
		page,
	}) => {
		const name = unique("intro");
		await page.goto("/prompts");
		await expect(
			page.getByRole("heading", { name: "Prompts", level: 1 }),
		).toBeVisible();

		await page.getByRole("button", { name: "New", exact: true }).click();
		await page.getByLabel("Name", { exact: true }).fill(name);
		await page.getByLabel("Description").fill("An intro for a video");
		await page
			.getByLabel("Text", { exact: true })
			.fill("Write an intro about {{topic}} in a warm tone.");
		await page.getByRole("button", { name: "Save" }).click();
		await expect(page.getByText("Saved.")).toBeVisible();
		await expect(page.locator(`[data-entry='${name}']`)).toBeVisible();

		await page.getByRole("tab", { name: "Preview" }).click();
		await page.getByLabel("Value of topic").fill("cats");
		await page.getByRole("button", { name: "Render" }).click();
		await expect(
			page.getByRole("region", { name: "Rendered text" }),
		).toContainText("Write an intro about cats in a warm tone.");
	});

	test("an imported skill stays unreviewed until a privileged session marks it", async ({
		page,
	}) => {
		const name = unique("hook");
		await page.goto("/skills");
		await expect(
			page.getByRole("heading", { name: "Skills", level: 1 }),
		).toBeVisible();

		await page.getByText("Import or export skills").click();
		await page
			.getByLabel("Folder or archive on the machine of the service")
			.fill(skillFolder(name));
		await page.getByLabel("Where it came from").fill("a friend");
		await page.getByRole("button", { name: "Import", exact: true }).click();
		await expect(
			page.getByText(`Imported ${name}. It is not reviewed yet.`),
		).toBeVisible();

		const row = page.locator(`[data-entry='${name}']`);
		await expect(row).toContainText("Not reviewed");
		await expect(row).toContainText("Imported");

		const pane = page.getByTestId("skill-pane");
		await expect(pane.getByRole("heading", { name })).toBeVisible();
		await expect(pane).toContainText("a friend");
		// the full text and the files are there to be read before the mark
		await expect(
			pane.getByRole("region", { name: "Full text of the skill" }),
		).toContainText("Write the opening of a short video.");
		await expect(pane).toContainText("references/patterns.md");

		// The server of this suite runs with no launcher secret, so this session is not a privileged one (SDD-007
		// section 3.4 item 7): the service refuses the mark, says why, and the skill stays unreviewed. The mark
		// itself is tested through the service with a privileged caller (servicetest).
		await pane.getByRole("button", { name: "Mark as read" }).click();
		await expect(pane.getByRole("alert")).toContainText(
			"only a person at the browser",
		);
		await expect(row).toContainText("Not reviewed");

		await page.getByRole("button", { name: "Export reviewed skills" }).click();
		await expect(
			page.getByText(/skills? written to|No reviewed skill/),
		).toBeVisible();
	});

	test("a note is kept as a file, found by the search and removed", async ({
		page,
	}) => {
		const name = unique("note");
		await page.goto("/memory");
		await page.getByRole("button", { name: "New", exact: true }).click();
		await page.getByLabel("Name (optional)").fill(name);
		await page
			.getByLabel("The fact, in one line")
			.fill("Viewers skip long quokka intros");
		await page
			.getByLabel("Text", { exact: true })
			.fill("Cut it to two seconds.");
		await page.getByRole("button", { name: "Save" }).click();
		await expect(page.getByText("Saved.")).toBeVisible();

		await page.getByLabel("Search", { exact: true }).fill("quokka");
		await page.getByRole("button", { name: "Search" }).click();
		await expect(page.locator(`[data-entry='${name}']`)).toBeVisible();

		await page.getByRole("button", { name: "Remove" }).click();
		await expect(page.locator(`[data-entry='${name}']`)).toHaveCount(0);
	});
});
