import { ensureAccount, signInWithPassword } from "../support/account";
import { caller, expect, home, test } from "./helpers";

// VV-320: a note an agent wrote waits for a person; after the person keeps it the panel says it is kept, with no
// reload, and a session that is not a person's is shown the button disabled instead.
test.describe("keeping a memory note", () => {
	test("Keep this note updates the panel at once", async ({
		page,
		request,
	}) => {
		const name = `agent-note-${Date.now()}`;
		const agent = await caller(request, "claude-code");
		await agent("POST", "/library/memory", {
			name,
			description: "Viewers skip long intros",
			body: "Cut it to two seconds.",
		});

		await ensureAccount(page.request, home);
		await signInWithPassword(page);
		await page.goto("/memory");
		await page.locator(`[data-entry='${name}']`).click();

		const pane = page.getByTestId("memory-pane");
		await expect(pane).toContainText("This note was not written by a person");
		await pane.getByRole("button", { name: "Keep this note" }).click();
		await expect(pane.getByText("Saved.")).toBeVisible();
		await expect(pane).not.toContainText(
			"This note was not written by a person",
		);
		await expect(
			pane.getByRole("button", { name: "Keep this note" }),
		).toHaveCount(0);
	});

	// Epic 99 task 4 (SDD-018 section 18): a row names the agent that wrote the note and the project it is for; the
	// name of the project is the creator's text and is drawn as text.
	test("a row reads with the name of the agent and of the project", async ({
		page,
		request,
	}) => {
		const stamp = Date.now();
		const call = await caller(request);
		const project = (
			(await call("POST", "/projects", {
				name: `<b>Bold</b> film ${stamp}`,
			})) as { project_id: string }
		).project_id;
		const name = `agent-scope-${stamp}`;
		const agent = await caller(request, "claude-code");
		await agent("POST", "/library/memory", {
			name,
			description: "Keep the logo on screen",
			scope: project,
			body: "Two seconds at the end.",
		});

		await ensureAccount(page.request, home);
		await signInWithPassword(page);
		await page.goto("/memory");
		const row = page
			.locator("[data-slot=list-row]")
			.filter({ has: page.locator(`[data-entry='${name}']`) });
		await expect(row).toContainText("by Claude Code");
		const projectName = row.getByTestId("memory-project");
		await expect(projectName).toContainText(`film ${stamp}`);
		await expect(projectName.locator("b")).toHaveCount(0);
		await page.locator(`[data-entry='${name}']`).click();
		await page.getByRole("button", { name: "Reject this note" }).click();
		await expect(page.locator(`[data-entry='${name}']`)).toHaveCount(0);
	});

	test("a note that waits is listed first, and Reject removes it so it is never kept", async ({
		page,
		request,
	}) => {
		const waiting = `agent-reject-${Date.now()}`;
		const agent = await caller(request, "claude-code");
		await agent("POST", "/library/memory", {
			name: waiting,
			description: "Rejected by a person",
			body: "Not wanted.",
		});

		await ensureAccount(page.request, home);
		await signInWithPassword(page);
		await page.goto("/memory");
		const row = page.locator(`[data-entry='${waiting}']`);
		await expect(
			page.locator("[data-group=pending]").locator(`[data-entry='${waiting}']`),
		).toBeVisible();
		await row.click();

		const pane = page.getByTestId("memory-pane");
		await expect(pane.getByTestId("memory-pending")).toContainText(
			"Waiting for your approval",
		);
		await pane.getByRole("button", { name: "Reject this note" }).click();
		await expect(row).toHaveCount(0);
	});
});
