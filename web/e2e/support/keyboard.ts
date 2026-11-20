import type { Page } from "@playwright/test";

// Helpers of the accessibility specs of epic 41.

// Counts the events a pointer produces, so a spec that must be keyboard-only
// asserts there were none. A click from a pointer is always preceded by a
// pointerdown and a mousedown, so the click itself is not counted: Firefox gives
// the click of Enter on a link a detail of 1.
export async function countPointerEvents(page: Page): Promise<void> {
	// The count lives in localStorage, so it survives the full page loads of a flow that visits many pages.
	await page.addInitScript(() => {
		for (const type of [
			"pointerdown",
			"pointerup",
			"mousedown",
			"mouseup",
			"dblclick",
			"contextmenu",
		]) {
			window.addEventListener(
				type,
				() =>
					localStorage.setItem(
						"__pointerEvents",
						String(Number(localStorage.getItem("__pointerEvents") ?? "0") + 1),
					),
				true,
			);
		}
	});
}

export async function pointerEvents(page: Page): Promise<number> {
	return page.evaluate(() =>
		Number(localStorage.getItem("__pointerEvents") ?? "0"),
	);
}

const NAMED_ROLES =
	"button|link|checkbox|radio|switch|textbox|searchbox|combobox|slider|spinbutton|tab|menuitem|menuitemcheckbox|menuitemradio|treeitem|option|gridcell|progressbar|tree|grid|toolbar|dialog|img";

// The lines of an accessibility-tree snapshot (Locator.ariaSnapshot) whose role
// needs a name and has none. A snapshot prints a name as `- button "Save"`; a
// control without one prints as `- button`, `- button [disabled]` or, with a
// value, `- textbox: text`, at any depth of the tree. String.raw keeps the
// backslashes: in a plain template literal `\s` is the letter s.
export function unnamedControls(snapshot: string): string[] {
	const bare = new RegExp(
		String.raw`^\s*- (${NAMED_ROLES})(\s*\[[^\]]*\])*\s*(:.*)?$`,
	);
	return snapshot
		.split("\n")
		.filter((line) => bare.test(line))
		.map((line) => line.trim());
}

// Presses Tab (Shift+Tab with `key`) until the element is focused, so a spec
// proves the keyboard reaches it and not only that it can be focused by script.
// The element must be focusable. Shift+Tab reaches the last controls of a page
// (the chat button) in a few presses however long the page is.
export async function tabTo(
	page: Page,
	target: ReturnType<Page["locator"]>,
	limit = 120,
	key: "Tab" | "Shift+Tab" = "Tab",
): Promise<void> {
	for (let i = 0; i < limit; i++) {
		const focused = await target
			.first()
			.evaluate(
				(el) =>
					el === document.activeElement || el.contains(document.activeElement),
			)
			.catch(() => false);
		if (focused) return;
		await page.keyboard.press(key);
	}
	throw new Error(`${key} did not reach the element in ${limit} presses`);
}

// Reaches a link with the keyboard. WebKit leaves links out of the Tab order
// unless a setting of the system turns them in (Safari: "Press Tab to highlight
// each item"), so there the link is focused by script and the spec goes on from
// the key that activates it.
export async function tabToLink(
	page: Page,
	link: ReturnType<Page["locator"]>,
	browserName: string,
): Promise<void> {
	if (browserName === "webkit") {
		await link.focus();
		return;
	}
	await tabTo(page, link);
}

// Presses `key` (an arrow, for a composite widget with one tab stop) until the element is focused.
export async function arrowTo(
	page: Page,
	target: ReturnType<Page["locator"]>,
	key: string,
	limit = 40,
): Promise<void> {
	for (let i = 0; i < limit; i++) {
		const focused = await target
			.first()
			.evaluate((el) => el === document.activeElement)
			.catch(() => false);
		if (focused) return;
		await page.keyboard.press(key);
	}
	throw new Error(`${key} did not reach the element in ${limit} presses`);
}
