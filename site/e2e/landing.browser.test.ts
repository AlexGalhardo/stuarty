// Browser tests of the landing page (epic 95): every product image follows the theme of the site, and a click on an image
// opens it enlarged in a modal dialog. They run against the built site (`bun run build` first) served from out/, in
// headless Chromium, with the Playwright and the axe of web/ (`bun install` there first); no dependency is added to the
// site. Run from site/: `bun run test:browser`.
import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { existsSync, statSync } from "node:fs";
import { createRequire } from "node:module";
import { join, resolve } from "node:path";
import type { Browser, BrowserContext, Page } from "@playwright/test";

const siteDir = resolve(import.meta.dir, "..");
const outDir = join(siteDir, "out");
const webRequire = createRequire(join(siteDir, "..", "web", "package.json"));

type Playwright = typeof import("@playwright/test");
type AxeBuilder = typeof import("@axe-core/playwright").default;

function load<T>(name: string): T {
	try {
		return webRequire(name) as T;
	} catch {
		throw new Error(`${name} is not installed: run \`bun install --frozen-lockfile\` in web/ first`);
	}
}
const { chromium } = load<Playwright>("@playwright/test");
const axeModule = load<{ default: AxeBuilder } | AxeBuilder>("@axe-core/playwright");
const AxeBuilderClass: AxeBuilder = "default" in axeModule ? axeModule.default : axeModule;

let server: ReturnType<typeof Bun.serve>;
let browser: Browser;
let url = "";

beforeAll(async () => {
	if (!existsSync(join(outDir, "index.html"))) throw new Error("the site is not built: run `bun run build` in site/ first");
	server = Bun.serve({
		port: 0,
		hostname: "127.0.0.1",
		fetch(req) {
			let file = join(outDir, decodeURIComponent(new URL(req.url).pathname));
			if (existsSync(file) && statSync(file).isDirectory()) file = join(file, "index.html");
			return existsSync(file) ? new Response(Bun.file(file)) : new Response("not found", { status: 404 });
		},
	});
	url = `http://127.0.0.1:${server.port}/`;
	browser = await chromium.launch();
});

afterAll(async () => {
	await browser?.close();
	await server?.stop(true);
});

async function open(theme: "dark" | "light", width = 1440, height = 900): Promise<{ context: BrowserContext; page: Page }> {
	const context = await browser.newContext({ viewport: { width, height }, reducedMotion: "reduce" });
	await context.addInitScript((t) => localStorage.setItem("theme", t), theme);
	const page = await context.newPage();
	await page.goto(url, { waitUntil: "load" });
	return { context, page };
}

// For each product image button that is shown: the file its visible picture loads (the capture of the theme).
async function shownCaptures(page: Page): Promise<{ shown: number; files: string[]; pictures: number[] }> {
	await page.evaluate(() => {
		for (const img of document.querySelectorAll<HTMLImageElement>(".shot-zoom img")) img.loading = "eager";
	});
	await page.waitForFunction(() => [...document.querySelectorAll<HTMLImageElement>(".shot-zoom img")].filter((i) => i.offsetParent !== null).every((i) => i.complete && i.currentSrc));
	return page.evaluate(() => {
		const buttons = [...document.querySelectorAll<HTMLElement>(".shot-zoom")].filter((b) => b.offsetParent !== null);
		return {
			shown: buttons.length,
			files: buttons.map((b) => [...b.querySelectorAll("img")].filter((i) => i.offsetParent !== null).map((i) => i.currentSrc.split("/").pop() ?? "")[0] ?? ""),
			pictures: buttons.map((b) => [...b.querySelectorAll("picture")].filter((p) => getComputedStyle(p).display !== "none").length),
		};
	});
}

describe("the product images follow the theme", () => {
	// Seven images on a wide window (the hero shows the home), and the same seven on a phone (the hero shows the phone home).
	for (const width of [1440, 390]) {
		for (const theme of ["dark", "light"] as const) {
			test(`${theme} at ${width} px: each shown image is its ${theme} capture`, async () => {
				const { context, page } = await open(theme, width);
				const { shown, files, pictures } = await shownCaptures(page);
				expect(shown).toBe(7);
				expect(pictures.every((n) => n === 1)).toBe(true);
				for (const file of files) expect(file.includes("-light")).toBe(theme === "light");
				await context.close();
			});
		}
	}

	test("switching the theme swaps every capture, with no reload", async () => {
		const { context, page } = await open("dark");
		let seen = await shownCaptures(page);
		expect(seen.files.some((f) => f.includes("-light"))).toBe(false);
		await page.evaluate(() => document.documentElement.classList.remove("dark"));
		seen = await shownCaptures(page);
		expect(seen.files.length).toBeGreaterThan(0);
		expect(seen.files.every((f) => f.includes("-light"))).toBe(true);
		await page.evaluate(() => document.documentElement.classList.add("dark"));
		seen = await shownCaptures(page);
		expect(seen.files.some((f) => f.includes("-light"))).toBe(false);
		await context.close();
	});
});

describe("the zoom of a product image", () => {
	const firstShot = (page: Page) => page.locator(".shot-zoom").filter({ visible: true }).nth(1);

	test("the cursor is a magnifier and the button is named by the alt text", async () => {
		const { context, page } = await open("dark");
		const shot = firstShot(page);
		expect(await shot.evaluate((e) => getComputedStyle(e).cursor)).toBe("zoom-in");
		const alt = await shot.locator("img").first().getAttribute("alt");
		expect(await shot.getAttribute("aria-label")).toBe(`Enlarge: ${alt}`);
		await context.close();
	});

	test("a click opens the dialog with the capture of the theme, the page behind blurred; Escape closes it and the focus returns", async () => {
		for (const theme of ["dark", "light"] as const) {
			const { context, page } = await open(theme);
			const shot = firstShot(page);
			await shot.scrollIntoViewIfNeeded();
			await shot.click();
			const dialog = page.locator("dialog[data-zoom-dialog]");
			expect(await dialog.evaluate((d: HTMLDialogElement) => d.open)).toBe(true);
			expect(await dialog.evaluate((d) => d.matches(":modal"))).toBe(true);
			// Chromium reports nothing for getComputedStyle(el, "::backdrop"), so the rule that styles the backdrop of this dialog is read.
			const blur = await dialog.evaluate((d) => {
				const found: string[] = [];
				const walk = (rules: CSSRuleList): void => {
					for (const rule of rules) {
						if (rule instanceof CSSStyleRule && rule.selectorText.includes("::backdrop") && rule.style.backdropFilter) {
							for (const part of rule.selectorText.split(",")) {
								const base = part.replace(/::backdrop.*$/, "").trim();
								if (part.includes("::backdrop") && base && d.matches(base)) found.push(rule.style.backdropFilter);
							}
						}
						else if ("cssRules" in rule && !(rule instanceof CSSStyleRule)) walk((rule as CSSGroupingRule).cssRules);
					}
				};
				for (const sheet of document.styleSheets) walk(sheet.cssRules);
				return found;
			});
			expect(blur.some((f) => /blur\(\d+px\)/.test(f))).toBe(true);
			const src = await dialog.locator("img").getAttribute("src");
			expect(src?.includes("-light")).toBe(theme === "light");
			expect(src).not.toContain("-960");
			await page.keyboard.press("Escape");
			expect(await dialog.evaluate((d: HTMLDialogElement) => d.open)).toBe(false);
			expect(await shot.evaluate((e) => e === document.activeElement)).toBe(true);
			await context.close();
		}
	});

	test("Enter and Space open it, the focus stays inside, and the close button closes it", async () => {
		const { context, page } = await open("dark");
		const shot = firstShot(page);
		const dialog = page.locator("dialog[data-zoom-dialog]");
		for (const key of ["Enter", "Space"]) {
			await shot.focus();
			await page.keyboard.press(key);
			expect(await dialog.evaluate((d: HTMLDialogElement) => d.open)).toBe(true);
			for (let i = 0; i < 3; i++) {
				await page.keyboard.press("Tab");
				// The page behind is inert: the focus is in the dialog, or on the document when it leaves the last control.
				expect(await page.evaluate(() => document.activeElement === document.body || document.activeElement?.closest("dialog") !== null)).toBe(true);
			}
			await page.getByRole("button", { name: "Close" }).click();
			expect(await dialog.evaluate((d: HTMLDialogElement) => d.open)).toBe(false);
			expect(await shot.evaluate((e) => e === document.activeElement)).toBe(true);
		}
		await context.close();
	});

	test("a click outside the picture closes it and returns the focus", async () => {
		const { context, page } = await open("dark");
		const shot = firstShot(page);
		await shot.click();
		const dialog = page.locator("dialog[data-zoom-dialog]");
		expect(await dialog.evaluate((d: HTMLDialogElement) => d.open)).toBe(true);
		await page.mouse.click(4, 4);
		expect(await dialog.evaluate((d: HTMLDialogElement) => d.open)).toBe(false);
		expect(await shot.evaluate((e) => e === document.activeElement)).toBe(true);
		await context.close();
	});

	test("the open dialog has no accessibility violation", async () => {
		for (const theme of ["dark", "light"] as const) {
			const { context, page } = await open(theme);
			await firstShot(page).click();
			await page.waitForFunction(() => (document.querySelector("[data-zoom-img]") as HTMLImageElement).complete);
			const results = await new AxeBuilderClass({ page }).include("dialog[open]").analyze();
			expect(results.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
			await context.close();
		}
	});

	test("with motion allowed the dialog fades in; with reduced motion it has no transition", async () => {
		const reduced = await open("dark");
		expect(await reduced.page.evaluate(() => parseFloat(getComputedStyle(document.querySelector("dialog[data-zoom-dialog]") as Element).transitionDuration))).toBeLessThan(0.001);
		await reduced.context.close();
		const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, reducedMotion: "no-preference" });
		const page = await context.newPage();
		await page.goto(url);
		expect(await page.evaluate(() => getComputedStyle(document.querySelector("dialog[data-zoom-dialog]") as Element).transitionDuration)).not.toBe("0s");
		await context.close();
	});
});

describe("the page itself", () => {
	for (const width of [1440, 390]) {
		for (const theme of ["dark", "light"] as const) {
			test(`${theme} at ${width} px: the page and its footer have no accessibility violation`, async () => {
				const { context, page } = await open(theme, width);
				const results = await new AxeBuilderClass({ page }).include("main").include("footer").analyze();
				expect(results.violations.map((v) => `${v.id}: ${v.nodes.map((n) => n.target.join(" ")).join(", ")}`)).toEqual([]);
				await context.close();
			});
		}
	}
});
