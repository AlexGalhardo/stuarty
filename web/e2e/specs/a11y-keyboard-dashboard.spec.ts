import { randomBytes } from "node:crypto";
import { seedApprovals, signInAsPerson } from "../support/approvals";
import {
	countPointerEvents,
	pointerEvents,
	tabTo,
	tabToLink,
	unnamedControls,
} from "../support/keyboard";
import { expect, seedProject, test } from "./helpers";

// Epic 41 task 3 (SDD-012 section 18.10, items K, O and N): the pages of the dashboard, their dialogs, the
// calendar, the approval queue and the chat panel are used with the keyboard alone. Each test counts the
// pointer events of the page and asserts there were none.
const activeId = (page: import("@playwright/test").Page) =>
	page.evaluate(() => document.activeElement?.id ?? "");

test.describe("a11y-keyboard-dashboard", () => {
	test("the first Tab stop is a skip link that moves the focus to the main region", async ({
		page,
		browserName,
	}) => {
		await countPointerEvents(page);
		await page.goto("/projects");
		const skip = page.getByRole("link", { name: "Skip to the main content" });
		if (browserName === "webkit") {
			await tabToLink(page, skip, browserName);
		} else {
			await page.keyboard.press("Tab");
		}
		await expect(skip).toBeFocused();
		await page.keyboard.press("Enter");
		await expect.poll(() => activeId(page)).toBe("main");
		// Tab continues inside the page, not from the top of the sidebar.
		await page.keyboard.press("Tab");
		expect(
			await page.evaluate(() =>
				document.getElementById("main")?.contains(document.activeElement),
			),
		).toBe(true);
		expect(await pointerEvents(page)).toBe(0);
	});

	test("a link of the sidebar changes the page and the focus lands on the main region", async ({
		page,
		browserName,
	}) => {
		await countPointerEvents(page);
		await page.goto("/");
		await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
		const link = page.getByTestId("nav-projects");
		await tabToLink(page, link, browserName);
		await page.keyboard.press("Enter");
		await expect(page).toHaveURL(/\/projects$/);
		await expect.poll(() => activeId(page)).toBe("main");
		expect(await pointerEvents(page)).toBe(0);
	});

	test("the chat panel opens with the keyboard, takes the focus in and gives it back on Escape", async ({
		page,
	}) => {
		await countPointerEvents(page);
		await page.goto("/projects");
		const toggle = page.getByTestId("chat-toggle");
		await tabTo(page, toggle);
		await page.keyboard.press("Enter");
		const panel = page.getByTestId("chat-panel");
		await expect(panel).toBeVisible();
		await expect
			.poll(() =>
				page.evaluate(
					() =>
						document
							.querySelector("[data-testid=chat-panel]")
							?.contains(document.activeElement) ?? false,
				),
			)
			.toBe(true);
		await page.keyboard.press("Escape");
		await expect(panel).toHaveCount(0);
		await expect(page.getByTestId("chat-toggle")).toBeFocused();
		expect(await pointerEvents(page)).toBe(0);
	});

	test("a dialog of a project card gives the focus back to the menu button on Escape", async ({
		page,
		request,
	}) => {
		const label = `A11y dialog ${randomBytes(3).toString("hex")}`;
		await seedProject(request, { name: label, tracks: 0 });
		await countPointerEvents(page);
		await page.goto("/projects");
		const actions = page.getByRole("button", {
			name: new RegExp(`Actions for project.*${label}`),
		});
		await tabTo(page, actions);
		await page.keyboard.press("Enter");
		await page.getByRole("menuitem", { name: "Delete" }).press("Enter");
		const dialog = page.getByRole("alertdialog");
		await expect(dialog).toBeVisible();
		await page.keyboard.press("Escape");
		await expect(dialog).toHaveCount(0);
		await expect(actions).toBeFocused();
		expect(await pointerEvents(page)).toBe(0);
	});

	test("the approval queue is decided with the keyboard and the focus stays in the queue", async ({
		page,
	}) => {
		const tag = randomBytes(3).toString("hex");
		const [first, second] = seedApprovals([
			`Keyboard post A ${tag}`,
			`Keyboard post B ${tag}`,
		]);
		await signInAsPerson(page);
		await countPointerEvents(page);
		await page.goto("/approvals");
		const row = page.locator(`[data-approval-id="${first}"]`);
		const other = page.locator(`[data-approval-id="${second}"]`);
		await expect(row).toBeVisible();
		await expect(other).toBeVisible();

		const approve = row.getByRole("button", { name: /^Approve/ });
		await tabTo(page, approve);
		await page.keyboard.press("Enter");
		await expect(row).toHaveCount(0);
		// The row that had the focus is gone: the focus moved into the queue, not to the page.
		await expect
			.poll(() =>
				page.evaluate(
					() =>
						document
							.querySelector("#approvals-queue")
							?.closest("section")
							?.contains(document.activeElement) ?? false,
				),
			)
			.toBe(true);
		await expect(page.getByTestId("approval-result")).toContainText(
			`Keyboard post A ${tag}`,
		);

		await other.getByRole("button", { name: /^Reject/ }).press("Enter");
		await expect(other).toHaveCount(0);
		expect(await pointerEvents(page)).toBe(0);
	});

	test("the calendar and its controls are reached with the keyboard and have names", async ({
		page,
	}) => {
		await countPointerEvents(page);
		await page.goto("/publish");
		const calendar = page.getByTestId("publish-mode-week");
		await tabTo(page, calendar);
		await page.keyboard.press("Enter");
		await expect(calendar).toHaveAttribute("aria-pressed", "true");
		await expect(page.getByTestId("calendar-week")).toBeVisible();
		const firstDay = page.getByTestId("calendar-day").first();
		const before = await firstDay.getAttribute("data-day");
		await tabTo(page, page.getByRole("button", { name: "Next" }));
		await page.keyboard.press("Enter");
		await expect(firstDay).not.toHaveAttribute("data-day", before ?? "");
		const tree = await page.locator("main").ariaSnapshot();
		expect(unnamedControls(tree)).toEqual([]);
		expect(await pointerEvents(page)).toBe(0);
	});
});
