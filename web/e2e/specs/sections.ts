import type { Page } from "@playwright/test";

// Opens an inspector section that starts collapsed (the project sections do,
// to keep the accessibility tree small, VV-191); one that is open stays so.
export async function openSection(page: Page, name: string): Promise<void> {
	const header = page.getByRole("button", { name, exact: true });
	if ((await header.getAttribute("aria-expanded")) === "false") {
		await header.click();
	}
}
