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
	// Fifteen images on a wide window (the hero, the twelve steps of the path, the trail of a video project and of a text
	// project), and the same fifteen on a phone, where the hero shows the phone home.
	for (const width of [1440, 390]) {
		for (const theme of ["dark", "light"] as const) {
			test(`${theme} at ${width} px: each shown image is its ${theme} capture`, async () => {
				const { context, page } = await open(theme, width);
				const { shown, files, pictures } = await shownCaptures(page);
				expect(shown).toBe(15);
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

describe("the path of a project", () => {
	test("twelve numbered steps, each with its own capture in both themes, lazy and sized", async () => {
		const { context, page } = await open("dark");
		const steps = await page.locator("#path .l-step").evaluateAll((items) =>
			items.map((li) => ({
				n: li.querySelector(".l-step-n")?.textContent,
				title: li.querySelector(".l-step-title")?.textContent,
				align: getComputedStyle(li.querySelector(".l-step-text p.l-lead") as Element).textAlign,
				pictures: [...li.querySelectorAll("picture img")].map((img) => ({ src: (img as HTMLImageElement).getAttribute("src"), loading: (img as HTMLImageElement).loading, w: img.getAttribute("width"), h: img.getAttribute("height"), srcset: (img as HTMLImageElement).srcset })),
			})),
		);
		expect(steps.length).toBe(12);
		expect(steps.map((s) => s.n)).toEqual(steps.map((_, i) => String(i + 1)));
		expect(new Set(steps.map((s) => s.pictures[0].src)).size).toBe(12);
		for (const s of steps) {
			expect(s.align === "start" || s.align === "left").toBe(true);
			expect(s.pictures.map((p) => p.src?.includes("-light"))).toEqual([false, true]);
			for (const p of s.pictures) {
				expect(p.loading).toBe("lazy");
				expect(p.w && p.h).toBeTruthy();
				expect(p.srcset).toContain("-960.webp 960w");
				expect((await page.request.get(new URL(p.src ?? "", url).href)).status(), p.src ?? "").toBe(200);
			}
		}
		await context.close();
	});

	test("the image of the last step opens enlarged in the theme of the page", async () => {
		for (const theme of ["dark", "light"] as const) {
			const { context, page } = await open(theme);
			const shot = page.locator("#path .l-step").last().locator(".shot-zoom");
			await shot.scrollIntoViewIfNeeded();
			await shot.click();
			const dialog = page.locator("dialog[data-zoom-dialog]");
			expect(await dialog.evaluate((d: HTMLDialogElement) => d.open)).toBe(true);
			const src = (await dialog.locator("img").getAttribute("src")) ?? "";
			expect(src).toContain("connections");
			expect(src.includes("-light")).toBe(theme === "light");
			await page.keyboard.press("Escape");
			expect(await shot.evaluate((e) => e === document.activeElement)).toBe(true);
			await context.close();
		}
	});
});

describe("the copy of the phone layout", () => {
	// The 390 px frames leave out the lead of the path and the title over the permission card, and shorten its reason.
	const visibleText = (page: Page, selector: string): Promise<string[]> =>
		page.locator(selector).evaluateAll((els) => els.filter((e) => (e as HTMLElement).offsetParent !== null).map((e) => (e as HTMLElement).innerText.trim()));
	for (const [width, lead, title, why] of [
		[1440, true, true, "One item is added. You can undo it from the history."],
		[390, false, false, "One item is added. You can undo it."],
	] as const) {
		test(`at ${width} px`, async () => {
			const { context, page } = await open("dark", width);
			const leads = await visibleText(page, "#path .l-head .l-lead");
			expect(leads.includes("Stuarty always shows where you are and what comes next, so you can learn it by using it.")).toBe(lead);
			expect((await visibleText(page, ".l-permission > h3")).includes("Every tool call asks first")).toBe(title);
			expect(await visibleText(page, ".l-notice-why")).toEqual([why]);
			await context.close();
		});
	}
});

// The changes the author asked for after the review of epic 95 (2026-11-24).
describe("the layout the author asked for", () => {
	for (const width of [390, 700, 1024, 1440, 1920]) {
		test(`at ${width} px the content lines up with the navbar`, async () => {
			const { context, page } = await open("dark", width);
			const edges = await page.evaluate(() => {
				const row = document.querySelector("header[data-variant='landing'] .site-container") as HTMLElement;
				const kids = [...row.children].map((c) => c.getBoundingClientRect()).filter((r) => r.width > 0);
				const box = (document.querySelector("#path .l-container") as HTMLElement).getBoundingClientRect();
				return { left: Math.min(...kids.map((r) => r.left)), right: Math.max(...kids.map((r) => r.right)), cLeft: box.left, cRight: box.right };
			});
			expect(Math.abs(edges.left - edges.cLeft)).toBeLessThanOrEqual(1);
			expect(Math.abs(edges.right - edges.cRight)).toBeLessThanOrEqual(1);
			await context.close();
		});
	}

	test("the navbar of the landing shows no release badge, and the page no alpha note", async () => {
		const { context, page } = await open("light");
		expect(await page.locator("header [data-release-badge]").isVisible()).toBe(false);
		expect(await page.locator("main, footer").allInnerTexts()).not.toContain("rough edges");
		expect((await page.locator("main").innerText()).toLowerCase()).not.toContain("rough edges");
		expect(await page.locator(".l-hero .l-eyebrow").count()).toBe(0);
		await context.close();
	});

	for (const width of [1440, 390]) {
		test(`at ${width} px the headings, the hero and the footer notes are centred, the cards are not`, async () => {
			const { context, page } = await open("dark", width);
			const result = await page.evaluate(() => {
				const centred = (el: Element): boolean => {
					const r = el.getBoundingClientRect();
					const box = (el.closest(".l-container") as HTMLElement).getBoundingClientRect();
					return getComputedStyle(el).textAlign === "center" && Math.abs(r.left + r.width / 2 - (box.left + box.width / 2)) <= 1.5;
				};
				const heads = [...document.querySelectorAll("main h1, main h2, main .l-head .l-eyebrow, main .l-head .l-lead, main .l-hero-head .l-lead, main .l-subtitle, .l-footer-notes p")].filter((e) => (e as HTMLElement).offsetParent !== null);
				const actions = [...document.querySelectorAll(".l-hero-head .l-actions, .l-hero-head .l-note")].map((e) => {
					const kids = [...e.children].map((k) => k.getBoundingClientRect()).filter((r) => r.width > 0);
					const box = (e.closest(".l-container") as HTMLElement).getBoundingClientRect();
					const mid = (Math.min(...kids.map((k) => k.left)) + Math.max(...kids.map((k) => k.right))) / 2;
					return Math.abs(mid - (box.left + box.width / 2));
				});
				return {
					notCentred: heads.filter((h) => !centred(h)).map((h) => h.textContent?.slice(0, 40)),
					actions,
					cards: [...document.querySelectorAll(".l-card .l-h3, .l-card .l-small")].map((e) => getComputedStyle(e).textAlign),
				};
			});
			expect(result.notCentred).toEqual([]);
			for (const off of result.actions) expect(off).toBeLessThanOrEqual(1.5);
			expect(result.cards.every((a) => a === "start" || a === "left")).toBe(true);
			await context.close();
		});
	}

	for (const width of [1440, 390]) {
		test(`at ${width} px the hero holds the five download cards in place of the two buttons, with valid links`, async () => {
			const { context, page } = await open("light", width);
			const hero = page.locator(".l-hero");
			expect(await hero.getByRole("link", { name: /get started|read the docs/i }).count()).toBe(0);
			const read = (scope: string) =>
				page.locator(`${scope} a.l-download`).evaluateAll((as) => as.map((a) => ({ id: (a as HTMLElement).dataset.download, href: (a as HTMLAnchorElement).href, text: (a as HTMLElement).innerText.replace(/\s+/g, " ").trim(), shown: a.getBoundingClientRect().width > 0 })));
			const top = await read(".l-hero");
			expect(top.map((c) => c.id)).toEqual(["windows", "macos", "linux", "docker-local", "docker-vps"]);
			expect(top.every((c) => c.shown)).toBe(true);
			expect(await read("#download")).toEqual(top);
			for (const c of top) {
				const link = new URL(c.href);
				if (link.origin === new URL(url).origin) expect((await page.request.get(c.href)).status(), c.href).toBe(200);
				else expect(link.protocol).toBe("https:");
			}
			const note = await hero.locator(".l-note").innerText();
			expect(note.trim()).toBe("Free and open source. No subscriptions.");
			expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
			await context.close();
		});
	}

	test("the coloured words of the hero title are text and keep 4.5:1 in both themes", async () => {
		for (const theme of ["dark", "light"] as const) {
			const { context, page } = await open(theme);
			const found = await page.evaluate(() => {
				const focus = document.querySelector(".l-hero-focus") as HTMLElement;
				const style = getComputedStyle(focus);
				let bgEl: Element | null = focus;
				let bg = "rgba(0, 0, 0, 0)";
				while (bgEl && /rgba\(0, 0, 0, 0\)|transparent/.test(bg)) {
					bg = getComputedStyle(bgEl).backgroundColor;
					bgEl = bgEl.parentElement;
				}
				return { text: focus.textContent, image: style.backgroundImage, clip: style.backgroundClip || style.getPropertyValue("-webkit-background-clip"), bg };
			});
			expect(found.text).toBe("Create, edit, manage and publish");
			expect(found.clip).toBe("text");
			const rgb = (s: string): number[] => (s.match(/\d+(\.\d+)?/g) ?? []).slice(0, 3).map(Number);
			const stops = [...found.image.matchAll(/rgb\([^)]+\)/g)].map((m) => rgb(m[0]));
			// Orange to purple (author choice): the first stop leans red, the last leans blue.
			expect(stops.length).toBeGreaterThanOrEqual(2);
			expect(new Set(stops.map((s) => s.join())).size).toBe(stops.length);
			expect(stops[0][0]).toBeGreaterThan(stops[0][2]);
			expect(stops[stops.length - 1][2]).toBeGreaterThan(stops[stops.length - 1][1]);
			const lum = (c: number[]): number => {
				const [r, g, b] = c.map((v) => {
					const x = v / 255;
					return x <= 0.04045 ? x / 12.92 : ((x + 0.055) / 1.055) ** 2.4;
				});
				return 0.2126 * r + 0.7152 * g + 0.0722 * b;
			};
			const bg = rgb(found.bg);
			const ratio = (c: number[]): number => {
				const [a, b] = [lum(c), lum(bg)].sort((x, y) => y - x);
				return (a + 0.05) / (b + 0.05);
			};
			for (let i = 0; i < stops.length - 1; i++) {
				for (let t = 0; t <= 1; t += 0.05) {
					const c = stops[i].map((v, k) => v + (stops[i + 1][k] - v) * t);
					expect(ratio(c), `${theme}: ${c.map(Math.round).join(",")} on ${bg.join(",")}`).toBeGreaterThanOrEqual(4.5);
				}
			}
			await context.close();
		}
	});
});

describe("the entrance of the sections as the page scrolls", () => {
	const below = "#download .l-head";
	const opacity = (page: Page, selector: string): Promise<number> => page.locator(selector).first().evaluate((e) => Number(getComputedStyle(e).opacity));

	test("with motion allowed a section below the window fades in as it is scrolled into view", async () => {
		const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, reducedMotion: "no-preference" });
		const page = await context.newPage();
		await page.goto(url, { waitUntil: "load" });
		expect(await opacity(page, below)).toBeLessThan(0.5);
		const layoutBefore = await page.locator(below).evaluate((e) => (e as HTMLElement).offsetHeight);
		await page.locator(below).evaluate((e) => e.scrollIntoView({ block: "center" }));
		await page.waitForFunction((s) => Number(getComputedStyle(document.querySelector(s) as Element).opacity) === 1, below);
		// offsetHeight is the layout height: the entrance scales the picture, never the layout.
		const after = await page.locator(below).evaluate((e) => (e as HTMLElement).offsetHeight);
		expect(after).toBe(layoutBefore);
		await context.close();
	});

	test("with reduced motion every section is shown at once, with no animation", async () => {
		const { context, page } = await open("dark");
		const hidden = await page.evaluate(() => [...document.querySelectorAll<HTMLElement>("[data-reveal]")].filter((e) => getComputedStyle(e).opacity !== "1" || getComputedStyle(e).animationName !== "none").length);
		expect(hidden).toBe(0);
		await context.close();
	});

	test("without JavaScript nothing waits for a script to be shown", async () => {
		const context = await browser.newContext({ viewport: { width: 1440, height: 900 }, reducedMotion: "no-preference", javaScriptEnabled: false });
		const page = await context.newPage();
		await page.goto(url, { waitUntil: "load" });
		expect(await page.locator(".l-reveal-out").count()).toBe(0);
		await page.locator(below).evaluate((e) => e.scrollIntoView({ block: "center" }));
		// A page without JavaScript runs no animation frame, so the wait polls on an interval instead.
		await page.waitForFunction((s) => Number(getComputedStyle(document.querySelector(s) as Element).opacity) === 1, below, { polling: 100, timeout: 5000 });
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
