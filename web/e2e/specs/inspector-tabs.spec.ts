import AxeBuilder from "@axe-core/playwright";

import {
	audioClip,
	caller,
	expect,
	seedProject,
	test,
	testClip,
} from "./helpers";

// TASK-0289 (SDD-012 section 6.4): the properties panel has a tab strip by
// domain. A tab is drawn only when the selection has a section of it; several
// items, a track and the project have no strip.
async function seedMixed(request: Parameters<typeof seedProject>[0]) {
	const seeded = await seedProject(request, {
		files: [testClip(), audioClip()],
		tracks: 1,
		audioTracks: 1,
		perTrack: 0,
	});
	const call = await caller(request);
	const head = (await call("GET", `/projects/${seeded.id}`)) as {
		revision: number;
	};
	const out = (await call("POST", `/projects/${seeded.id}/edit`, {
		base_revision: head.revision,
		operations: [
			{
				op: "clip.add",
				args: {
					track_id: seeded.trackIds[0],
					asset_id: seeded.assetIds[0],
					start: "0s",
					source_in: "0s",
					duration: "2s",
				},
			},
			{
				op: "text.add",
				args: {
					track_id: seeded.trackIds[0],
					start: "2s",
					duration: "2s",
					text: "Hi",
				},
			},
			{
				op: "audio.add",
				args: {
					track_id: seeded.audioTrackIds[0],
					asset_id: seeded.assetIds[1],
					start: "0s",
				},
			},
		],
	})) as { effective: { created?: string[] }[] };
	const [clip, text, tone] = out.effective.map((e) => e.created?.[0]);
	return { id: seeded.id, clip, text, tone };
}

test.describe("inspector tabs on a laptop screen", () => {
	test.use({ viewport: { width: 1366, height: 850 } });

	// VV-328: the strip starts at its first tab, and no label of it is cut at the left edge.
	test("the strip of a clip starts at its first tab and cuts no label", async ({
		page,
		request,
	}) => {
		const s = await seedMixed(request);
		await page.goto(`/editor/${s.id}`);
		await page.locator(`[data-item-id="${s.clip}"]`).click();
		const strip = page.getByRole("tablist", { name: "Properties" });
		await expect(strip.getByRole("tab", { name: "Video" })).toBeVisible();
		const geometry = await strip.evaluate((el) => ({
			scrollLeft: el.scrollLeft,
			left: el.getBoundingClientRect().left,
			tabs: Array.from(el.querySelectorAll('[role="tab"]')).map((tab) => ({
				text: tab.textContent,
				left: tab.getBoundingClientRect().left,
				fits: tab.scrollWidth <= tab.clientWidth + 1,
			})),
		}));
		expect(geometry.scrollLeft).toBe(0);
		expect(geometry.tabs[0].left).toBeGreaterThanOrEqual(geometry.left);
		for (const tab of geometry.tabs) {
			expect(tab.fits, tab.text ?? "").toBe(true);
		}
		const text = strip.getByRole("tab", { name: "Text" });
		await text.click();
		await expect
			.poll(() =>
				strip.evaluate((el) => {
					const tab = el.querySelector('[aria-selected="true"]');
					const box = el.getBoundingClientRect();
					const at = tab?.getBoundingClientRect();
					return !!at && at.left >= box.left - 1 && at.right <= box.right + 1;
				}),
			)
			.toBe(true);
	});

	test("the chosen tab stays whole when the panel gets narrower", async ({
		page,
		request,
	}) => {
		const s = await seedMixed(request);
		await page.setViewportSize({ width: 1600, height: 850 });
		await page.goto(`/editor/${s.id}`);
		await page.locator(`[data-item-id="${s.clip}"]`).click();
		const strip = page.getByRole("tablist", { name: "Properties" });
		await strip.getByRole("tab", { name: "Text" }).click();
		await page.setViewportSize({ width: 1300, height: 850 });
		await expect
			.poll(() =>
				strip.evaluate((el) => {
					const tab = el.querySelector('[aria-selected="true"]');
					const box = el.getBoundingClientRect();
					const at = tab?.getBoundingClientRect();
					return !!at && at.left >= box.left - 1 && at.right <= box.right + 1;
				}),
			)
			.toBe(true);
	});
});

test.describe("inspector tabs", () => {
	test.use({ viewport: { width: 1600, height: 900 } });

	test("each selection shows the tabs of its sections", async ({
		page,
		request,
	}) => {
		const s = await seedMixed(request);
		await page.goto(`/editor/${s.id}`);
		const strip = page.getByRole("tablist", { name: "Properties" });
		await expect(strip).toHaveCount(0);

		await page.locator(`[data-item-id="${s.clip}"]`).click();
		const names = async (): Promise<string[]> =>
			strip.getByRole("tab").allInnerTexts();
		await expect(strip.getByRole("tab", { name: "Video" })).toBeVisible();
		expect(await names()).toEqual(
			expect.arrayContaining([
				"Video",
				"Audio",
				"Speed",
				"Animation",
				"Adjust",
			]),
		);

		await page.locator(`[data-item-id="${s.text}"]`).click();
		await expect.poll(names).toEqual(["Video", "Animation", "Text"]);
		await page.locator(`[data-item-id="${s.tone}"]`).click();
		await expect.poll(names).toEqual(["Video", "Audio"]);

		// Several items: no strip. The project: no strip.
		await page
			.locator(`[data-item-id="${s.text}"]`)
			.click({ modifiers: ["Control"] });
		await expect(strip).toHaveCount(0);
		await page.keyboard.press("Escape");
		await expect(strip).toHaveCount(0);
	});

	test("a tab shows its sections, with arrow keys and the choice kept", async ({
		page,
		request,
	}) => {
		const s = await seedMixed(request);
		await page.goto(`/editor/${s.id}`);
		await page.locator(`[data-item-id="${s.clip}"]`).click();
		const strip = page.getByRole("tablist", { name: "Properties" });
		const video = strip.getByRole("tab", { name: "Video" });
		await expect(video).toHaveAttribute("aria-selected", "true");
		await expect(
			page.getByRole("button", { name: "Transform", exact: true }),
		).toBeVisible();
		await expect(
			page.getByRole("button", { name: "Speed", exact: true }),
		).toHaveCount(0);

		await video.focus();
		await page.keyboard.press("ArrowRight");
		const audio = strip.getByRole("tab", { name: "Audio" });
		await expect(audio).toBeFocused();
		await page.keyboard.press("ArrowRight");
		await expect(strip.getByRole("tab", { name: "Speed" })).toHaveAttribute(
			"aria-selected",
			"true",
		);
		await expect(
			page.getByRole("button", { name: "Speed", exact: true }),
		).toBeVisible();
		await expect(
			page.getByRole("button", { name: "Transform", exact: true }),
		).toHaveCount(0);
		await page.keyboard.press("End");
		await expect(strip.getByRole("tab").last()).toBeFocused();
		await page.keyboard.press("Home");
		await expect(video).toBeFocused();

		// The chosen tab comes back with the next selection that has it.
		await strip.getByRole("tab", { name: "Animation" }).click();
		await page.locator(`[data-item-id="${s.text}"]`).click();
		await expect(strip.getByRole("tab", { name: "Animation" })).toHaveAttribute(
			"aria-selected",
			"true",
		);
		// A selection without the chosen tab shows its first one.
		await strip.getByRole("tab", { name: "Text" }).click();
		await page.locator(`[data-item-id="${s.tone}"]`).click();
		await expect(strip.getByRole("tab", { name: "Video" })).toHaveAttribute(
			"aria-selected",
			"true",
		);
	});

	for (const lang of ["en", "pt-BR"] as const) {
		for (const theme of ["light", "dark"] as const) {
			test(`axe finds nothing in any tab in ${theme} in ${lang}`, async ({
				page,
				request,
			}) => {
				await page.addInitScript(
					([l, th]) => {
						localStorage.setItem("vv.lang", l);
						if (th === "light") localStorage.setItem("theme", "light");
					},
					[lang, theme],
				);
				const s = await seedMixed(request);
				await page.goto(`/editor/${s.id}`);
				await page.locator(`[data-item-id="${s.clip}"]`).click();
				const tabs = page.locator("[data-inspector-tab]");
				await expect(tabs).not.toHaveCount(0);
				const count = await tabs.count();
				for (let i = 0; i < count; i++) {
					await tabs.nth(i).click();
					await expect(tabs.nth(i)).toHaveAttribute("aria-selected", "true");
					await page.evaluate(() =>
						Promise.all(
							document
								.getAnimations()
								.map((a) => a.finished.catch(() => undefined)),
						),
					);
					const { violations } = await new AxeBuilder({ page })
						.include("aside[data-landmark='5']")
						.withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
						.analyze();
					expect(violations.map((v) => v.id)).toEqual([]);
				}
			});
		}
	}
});
