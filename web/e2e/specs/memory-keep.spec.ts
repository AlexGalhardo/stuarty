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
		await signInWithPassword(page.request);
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
});
