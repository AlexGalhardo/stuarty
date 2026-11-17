import type { Page } from "@playwright/test";

// Opens an inspector section that starts collapsed (the project sections do,
// to keep the accessibility tree small, VV-191); one that is open stays so.
export async function openSection(page: Page, name: string): Promise<void> {
	const header = page.getByRole("button", { name, exact: true });
	if ((await header.getAttribute("aria-expanded")) === "false") {
		await header.click();
	}
}

// Shows a tab of the inspector strip (SDD-012 section 6.4): the sections of a
// clip are spread over Video, Audio, Speed, Animation, Adjust and Text. The tab
// is kept across selections, so a spec that changes selection opens it again.
export async function openInspectorTab(
	page: Page,
	name: string,
): Promise<void> {
	const tab = page
		.getByRole("tablist", { name: /^(Properties|Propriedades)$/ })
		.getByRole("tab", { name, exact: true });
	await tab.click();
}
