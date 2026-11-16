import { readFileSync } from "node:fs";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";

type Catalog = { [key: string]: string | Catalog };

function load(name: string): Catalog {
	return JSON.parse(
		readFileSync(join(process.cwd(), "resources", "lang", name), "utf8"),
	) as Catalog;
}

function flatten(node: Catalog, prefix = "", out = new Map<string, string>()) {
	for (const [key, value] of Object.entries(node)) {
		const path = prefix === "" ? key : `${prefix}.${key}`;
		if (typeof value === "string") out.set(path, value);
		else flatten(value, path, out);
	}
	return out;
}

const en = flatten(load("en.json"));
const pt = flatten(load("pt-BR.json"));

// The English strings that Portuguese spells differently. A rendered string
// equal to one of them is English left in the Portuguese UI. A key whose text
// is the same in both languages ("Volume", "X") is not a leak, and a value
// with a placeholder is not a whole string on the screen.
const englishOnly = new Set<string>();
for (const [key, value] of en) {
	const other = pt.get(key);
	if (other !== undefined && other !== value && !value.includes("{")) {
		englishOnly.add(value);
	}
}
const sameInBoth = new Set<string>();
for (const [key, value] of en) {
	if (pt.get(key) === value) sameInBoth.add(value);
}

// Every text node a person sees, and the names assistive technology reads.
async function visibleStrings(page: Page): Promise<string[]> {
	return page.evaluate(() => {
		const out: string[] = [];
		const norm = (s: string | null) => (s ?? "").replace(/\s+/g, " ").trim();
		const shown = (el: Element | null): boolean =>
			el !== null &&
			(el as HTMLElement).checkVisibility({
				checkOpacity: false,
				checkVisibilityCSS: true,
			});
		const walker = document.createTreeWalker(
			document.body,
			NodeFilter.SHOW_TEXT,
		);
		for (let n = walker.nextNode(); n; n = walker.nextNode()) {
			const parent = n.parentElement;
			if (!parent || ["SCRIPT", "STYLE"].includes(parent.tagName)) continue;
			if (shown(parent)) out.push(norm(n.textContent));
		}
		for (const el of document.querySelectorAll(
			"[aria-label], [title], [placeholder]",
		)) {
			if (!shown(el)) continue;
			for (const attr of ["aria-label", "title", "placeholder"]) {
				out.push(norm(el.getAttribute(attr)));
			}
		}
		return out.filter((s) => s !== "");
	});
}

async function expectNoEnglish(page: Page, where: string): Promise<void> {
	const leaked = [...new Set(await visibleStrings(page))].filter(
		(s) => englishOnly.has(s) && !sameInBoth.has(s),
	);
	expect(leaked, `English left in pt-BR on ${where}`).toEqual([]);
}

async function noOverflow(page: Page): Promise<void> {
	const overflow = await page.evaluate(() => {
		const header = document.querySelector("header");
		return {
			page: document.documentElement.scrollWidth - window.innerWidth,
			header: header ? header.scrollWidth - header.clientWidth : 0,
		};
	});
	expect(overflow.page).toBeLessThanOrEqual(0);
	expect(overflow.header).toBeLessThanOrEqual(0);
}

// The language is a preference of the browser (SDD-012 section 16.3): it lives in
// localStorage, and every test starts with a fresh context, so none is stored.
async function storedLang(page: Page): Promise<string | null> {
	return page.evaluate(() => localStorage.getItem("vv.lang"));
}

// Scenario 21 (SDD-012 sections 16 and 22): switch to pt-BR from an Inertia
// page and from the editor, no English UI string remains on the main screens,
// the playhead and the selection survive the reload, the stored language holds,
// and switching back restores English.
test.describe("i18n-switch", () => {
	test.use({ viewport: { width: 1280, height: 720 } });

	test("en to pt-BR to en across the pages and the editor", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0];
		if (itemId === undefined) throw new Error("no item seeded");

		// Inertia page: the select keeps the locale and the page renders again.
		await page.goto("/projects");
		await expect(
			page.getByRole("heading", { name: "Projects", level: 1 }),
		).toBeVisible();
		await page.getByRole("combobox", { name: "Language" }).click();
		await page.getByRole("option", { name: "Português (Brasil)" }).click();
		await expect(
			page.getByRole("heading", { name: "Projetos", level: 1 }),
		).toBeVisible();
		expect(await storedLang(page)).toBe("pt-BR");
		await expectNoEnglish(page, "/projects");
		await noOverflow(page);

		// The choice persists across a reload and a new page of the context.
		await page.reload();
		await expect(
			page.getByRole("heading", { name: "Projetos", level: 1 }),
		).toBeVisible();
		await page.goto("/settings");
		await expect(
			page.getByRole("heading", { name: "Configurações", level: 1 }),
		).toBeVisible();
		await expectNoEnglish(page, "/settings");
		await page.goto("/settings/brand-kit");
		await expect(
			page.getByRole("heading", { name: "Kit de marca", level: 1 }),
		).toBeVisible();
		await expectNoEnglish(page, "/settings/brand-kit");

		// Editor in pt-BR with a clip selected.
		await page.goto(`/editor/${seeded.id}?t=45f&item=${itemId}`);
		await expect(
			page.getByRole("link", { name: "Voltar aos projetos" }),
		).toBeVisible();
		await expect(
			page.locator('[role="slider"][aria-valuenow="45"]'),
		).toBeVisible();
		await expect(page.getByRole("heading", { name: /^Clipe/ })).toBeVisible();
		await expectNoEnglish(page, "the editor");
		await noOverflow(page);

		await page.getByRole("button", { name: "Exportar" }).click();
		const dialog = page.getByRole("dialog");
		await expect(
			dialog.getByRole("button", { name: "Iniciar exportação" }),
		).toBeVisible();
		await expectNoEnglish(page, "the export dialog");
		// Escape would also clear the selection, which this scenario keeps.
		await dialog.getByRole("button", { name: "Fechar" }).first().click();
		await expect(dialog).toBeHidden();

		// Switch back from the editor: same playhead, same selection.
		const reloaded = page.waitForRequest(
			(r) => r.isNavigationRequest() && r.url().includes("t=45f"),
		);
		await page.getByRole("combobox", { name: "Idioma" }).click();
		await page.getByRole("option", { name: "English" }).click();
		await reloaded;
		await expect(
			page.getByRole("link", { name: "Back to projects" }),
		).toBeVisible();
		await expect(
			page.locator('[role="slider"][aria-valuenow="45"]'),
		).toBeVisible();
		await expect(page.getByRole("heading", { name: /^Clip/ })).toBeVisible();
		await expect(page.locator(`[data-item-id="${itemId}"]`)).toBeVisible();
		expect(await storedLang(page)).toBe("en");
	});

	test("the editor keeps the playhead and the selection when it switches to pt-BR", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?t=45f`);
		await page.locator("[data-item-id]").click();
		await expect(page.getByRole("heading", { name: /^Clip/ })).toBeVisible();

		const reloaded = page.waitForRequest(
			(r) => r.isNavigationRequest() && r.url().includes("t=45f"),
		);
		await page.getByRole("combobox", { name: "Language" }).click();
		await page.getByRole("option", { name: "Português (Brasil)" }).click();
		await reloaded;

		await expect(
			page.getByRole("link", { name: "Voltar aos projetos" }),
		).toBeVisible();
		await expect(
			page.locator('[role="slider"][aria-valuenow="45"]'),
		).toBeVisible();
		await expect(page.getByRole("heading", { name: /^Clipe/ })).toBeVisible();
		expect(await storedLang(page)).toBe("pt-BR");
	});
});
