import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";
import { ensureAccount, signInWithPassword } from "../support/account";
import { afterRender, caller, expect, home, test } from "./helpers";

// Epic 97 (SDD-020 section 30A.6, design system "Calendar themes"): axe over the day view of the publishing calendar,
// in both themes and both languages, with a note, a quiet mark and every theme a day can take, and over the dialog of
// the weekly slots. A `serious` or `critical` violation fails; a `moderate` or `minor` one fails too unless
// a11y-allow.json lists it with a reason, as in a11y-axe-both-themes. Each combination plans its own Tuesday of June
// 2027, far from the days of the other specs, and creates no slot, since the slots belong to the whole workspace and
// publish-day counts them. The four sign in one after the other: a password sign-in locks the studio for a moment.
test.describe.configure({ mode: "serial" });

interface Allowed {
	rule: string;
	state?: string;
	reason: string;
}

const allow = JSON.parse(
	readFileSync(
		join(dirname(fileURLToPath(import.meta.url)), "../a11y-allow.json"),
		"utf8",
	),
) as Allowed[];

const TAGS = ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"];

const THEMES = ["blue", "green", "amber", "red", "teal", "gray"] as const;

interface Finding {
	state: string;
	rule: string;
	impact: string | null | undefined;
	nodes: string[];
}

async function scan(
	page: Page,
	state: string,
	found: Finding[],
	include?: string,
): Promise<void> {
	// A dialog that is still fading in has a text colour halfway to its final one, which axe reads as low contrast:
	// every animation that ends is awaited (a spinner never ends and is left out).
	await afterRender(page);
	await page.evaluate(() =>
		Promise.all(
			document
				.getAnimations()
				.filter((a) => a.effect?.getComputedTiming().iterations !== Infinity)
				.map((a) => a.finished.catch(() => undefined)),
		),
	);
	let builder = new AxeBuilder({ page }).withTags(TAGS);
	if (include !== undefined) builder = builder.include(include);
	const { violations } = await builder.analyze();
	for (const v of violations) {
		const listed =
			v.impact !== "serious" &&
			v.impact !== "critical" &&
			allow.some(
				(a) => a.rule === v.id && (a.state === undefined || a.state === state),
			);
		if (listed) continue;
		found.push({
			state,
			rule: v.id,
			impact: v.impact,
			nodes: v.nodes.map((n) => `${n.target.join(" ")} ${n.failureSummary}`),
		});
	}
}

const COMBINATIONS = [
	{ lang: "en", theme: "dark", day: "2027-06-01" },
	{ lang: "en", theme: "light", day: "2027-06-08" },
	{ lang: "pt-BR", theme: "dark", day: "2027-06-15" },
	{ lang: "pt-BR", theme: "light", day: "2027-06-22" },
] as const;

for (const { lang, theme, day } of COMBINATIONS) {
	test.describe(`a11y-publish-day ${theme} ${lang}`, () => {
		// The dark theme is checked with the system in light mode: the default ignores prefers-color-scheme.
		test.use({ colorScheme: theme === "dark" ? "light" : "dark" });

		test(`the day view has no serious or critical violation in ${theme} in ${lang}`, async ({
			page,
			request,
		}) => {
			test.setTimeout(180_000);
			await page.addInitScript(
				([th, l]) => {
					localStorage.setItem("theme", th);
					localStorage.setItem("vv.lang", l);
				},
				[theme, lang],
			);
			// A person: only a person marks a quiet day (D-146).
			await ensureAccount(page.request, home);
			await signInWithPassword(page.request);
			const api = await caller(request);
			const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
			await api("PUT", `/calendar/days/${day}`, {
				zone,
				note: "Long video day: record, cut and caption",
				theme: "amber",
			});
			const found: Finding[] = [];
			try {
				await page.goto("/publish");
				await page.getByTestId("mini-go").fill(day);
				await page.getByTestId("mini-go-submit").click();
				const view = page.getByTestId("calendar-day-view");
				await expect(view).toHaveAttribute("data-date", day);
				await expect(page.getByTestId("day-note")).toHaveValue(
					"Long video day: record, cut and caption",
				);
				await page.getByTestId("day-quiet").click();
				await expect(page.getByTestId("day-quiet-badge")).toBeVisible();
				await scan(page, "publish day view", found);

				// Every theme a day takes, drawn in the header of the day.
				for (const name of THEMES) {
					await page.getByTestId(`day-theme-${name}`).click();
					await expect(page.getByTestId("day-header")).toHaveAttribute(
						"data-theme",
						name,
					);
					await scan(
						page,
						`publish day theme ${name}`,
						found,
						'[data-testid="calendar-day-view"]',
					);
				}

				await page.getByTestId("publish-slots").click();
				await expect(page.getByTestId("slots-dialog")).toBeVisible();
				await scan(page, "publish slots dialog", found);
				await page.keyboard.press("Escape");
				await expect(page.getByTestId("slots-dialog")).toHaveCount(0);

				// The person clears the mark the run made.
				await page.getByTestId("day-quiet").click();
				await expect(page.getByTestId("day-quiet-badge")).toHaveCount(0);
			} finally {
				await api("PUT", `/calendar/days/${day}`, {
					note: "",
					theme: "",
				}).catch(() => undefined);
			}
			expect(found, "axe violations").toEqual([]);
		});
	});
}
