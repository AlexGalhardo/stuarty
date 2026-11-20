import { randomBytes } from "node:crypto";
import { unnamedControls } from "../support/keyboard";
import { NETWORK_URL } from "../support/network";
import { caller, expect, seedProject, test, testClip } from "./helpers";
import { editImage, newImage } from "./image-support";

// Epic 41 task 6 (SDD-012 section 18.10, items L, E and N; acceptance criteria 3 and 4): what changes without a
// focus move is told in a live region: an edit by an agent while a person works, in the video editor and in the
// image editor, and the progress of an export. A refused form says so in text and tied to its fields.
test.describe("a11y-live-regions", () => {
	test("an edit by an agent is announced while the person works in the video editor", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			files: [testClip()],
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		await page.goto(`/editor/${seeded.id}`);
		const item = page.locator("[data-item-id]");
		await expect(item).toHaveCount(1);
		// The person is in the middle of something: an item is selected and has the focus.
		await item.focus();
		await page.keyboard.press("Enter");
		const live = page.locator("[data-live-region]");
		await expect(live).toContainText("Selected");

		const agent = await caller(request, "claude-code");
		const head = (await agent("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		await agent("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "item.rename",
					args: { item_id: seeded.itemIds[0], name: "Renamed by the agent" },
				},
			],
		});
		await expect(live).toHaveText("claude-code changed the project");
		// The region is polite, so it never interrupts what the person is hearing.
		await expect(live).toHaveAttribute("aria-live", "polite");
		await expect(item).toBeFocused();
	});

	test("an edit by an agent is announced in the image editor", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Live spec" });
		await editImage(call, id, [
			{ op: "layer.add_fill", args: { color: "#203040", name: "Back" } },
		]);
		await page.goto(`/edit-image/${id}`);
		await expect(page.getByRole("treeitem")).toHaveCount(1);
		const notice = page.getByTestId("image-agent-notice");
		await expect(notice).toHaveAttribute("aria-live", "polite");
		await expect(notice).toHaveText("");
		await editImage(call, id, [
			{ op: "layer.add_fill", args: { color: "#802020", name: "Added" } },
		]);
		await expect(notice).toContainText("changed");
		await expect(page.getByRole("treeitem")).toHaveCount(2);
	});

	test("the progress of an export is named, and told to a screen reader only in steps of 10", async ({
		page,
		request,
	}) => {
		test.setTimeout(120_000);
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "2s",
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.getByRole("button", { name: "Export", exact: true }).click();
		const dialog = page.getByRole("dialog");
		const start = dialog.getByRole("button", { name: /^Start export$/ });
		await expect(start).toBeEnabled();
		// Every text of the polite region of the dialog, in the order it changed.
		await dialog.locator("[data-phase]").evaluate((el) => {
			const log: string[] = [];
			(window as unknown as { __said: string[] }).__said = log;
			new MutationObserver(() => {
				const text =
					el
						.querySelector("[data-testid=export-announced]")
						?.textContent?.trim() ?? "";
				if (text !== "" && log[log.length - 1] !== text) log.push(text);
			}).observe(el, { childList: true, characterData: true, subtree: true });
		});
		await start.click();
		await expect(dialog.locator("[data-phase='succeeded']")).toBeVisible({
			timeout: 100_000,
		});
		const said = await page.evaluate(
			() => (window as unknown as { __said: string[] }).__said,
		);
		expect(said.length).toBeGreaterThan(0);
		for (const text of said)
			expect(text).toMatch(/^(0|10|20|30|40|50|60|70|80|90|100)%$/);
		const tree = await dialog.ariaSnapshot();
		expect(unnamedControls(tree)).toEqual([]);
	});

	test("a refused sign-in is an alert tied to the fields it concerns", async ({
		page,
	}) => {
		await page.goto(`${NETWORK_URL}/login`);
		await page
			.getByLabel("Email")
			.fill(`nobody-${randomBytes(3).toString("hex")}@example.test`);
		await page.getByLabel("Password").fill("not-the-password-1");
		await page.getByRole("button", { name: /^Log in$/ }).click();
		const alert = page.locator("#login-error");
		await expect(alert).toHaveAttribute("role", "alert");
		await expect(alert).not.toBeEmpty();
		for (const field of ["Email", "Password"]) {
			const input = page.getByLabel(field);
			await expect(input).toHaveAttribute("aria-invalid", "true");
			await expect(input).toHaveAttribute("aria-describedby", "login-error");
		}
		await expect(page.getByLabel("Email")).toHaveAccessibleDescription(/.+/);
	});
});
