import { caller, expect, test, testClip } from "./helpers";

// Epic 08 (SDD-011 section 14, RF-320 to RF-323): a file imported in one project, with its origin, is found in
// the library by its tag, tagged and marked reusable from the page, and used from another project with no
// second copy.
test.describe.configure({ timeout: 120_000 });

test.describe("asset-library", () => {
	test("import with an origin, find it by tag, mark it reusable, use it from another project", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		const stamp = Date.now();
		const tag = `e2e-${stamp}`;
		const one = (
			(await call("POST", "/projects", { name: `Library one ${stamp}` })) as {
				project_id: string;
			}
		).project_id;
		const two = (
			(await call("POST", "/projects", { name: `Library two ${stamp}` })) as {
				project_id: string;
			}
		).project_id;

		const imported = (await call("POST", `/projects/${one}/media`, {
			files: [{ path: testClip(7) }],
			wait_ms: 20_000,
			origin: {
				kind: "generated",
				provider: "acme",
				model: "clip-1",
				prompt: "a surprised cat",
			},
			info: { tags: [tag] },
		})) as { assets: { hash: string }[] };
		const hash = imported.assets[0]?.hash;
		expect(hash).toMatch(/^sha256:[0-9a-f]{64}$/);

		// The workspace is shared by the browsers of the suite, and the same file is the same asset: start from
		// a file that is not marked.
		await call("PUT", `/assets/${hash}`, { reusable: false });

		await page.goto("/video-assets");
		await expect(
			page.getByRole("heading", { name: "Video Assets", level: 1 }),
		).toBeVisible();

		await page.getByLabel("Tag").fill(tag);
		await page.getByRole("button", { name: "Apply" }).click();
		const card = page.locator(`[data-asset-hash="${hash}"]`);
		await expect(card).toBeVisible();
		await expect(page.getByTestId("asset-grid").locator("li")).toHaveCount(1);

		await card.click();
		const detail = page.getByTestId("asset-detail");
		await expect(detail.getByTestId("asset-origin")).toHaveText("Generated");
		await expect(detail).toContainText("Provider acme, model clip-1");
		await expect(detail.getByTestId("asset-uses")).toContainText(
			`Library one ${stamp}`,
		);
		// The note says it is the person's own record, not a licence check.
		await expect(detail).toContainText("The product does not check it");

		await detail.getByLabel("New tag").fill("Harbour");
		await detail.getByRole("button", { name: "Add tag" }).click();
		await expect(detail.getByRole("status")).toHaveText("Saved.");
		await expect(detail).toContainText("harbour");

		// The box follows what the service answered, so it flips after the call, not at the click.
		await detail.getByLabel("Offer this file again (reusable)").click();
		await expect(
			detail.getByLabel("Offer this file again (reusable)"),
		).toBeChecked();
		await expect(detail.getByRole("status")).toHaveText("Saved.");

		await detail.getByLabel("Rights note").fill("mine, shot on the pier");
		await detail.getByRole("button", { name: "Save" }).click();
		await expect(detail.getByRole("status")).toHaveText("Saved.");

		// Found again by the reusable filter, from the page and from the API.
		await page.getByLabel("Reusable only").check();
		await page.getByRole("button", { name: "Apply" }).click();
		await expect(card).toBeVisible();

		const listed = (await call(
			"GET",
			`/assets?tag=harbour&reusable=true&text=${tag}`,
		)) as { assets: { hash: string; origin: { rights_note?: string } }[] };
		expect(
			listed.assets.map((a) => a.hash),
			JSON.stringify(listed),
		).toEqual([hash]);
		expect(listed.assets[0]?.origin.rights_note).toBe("mine, shot on the pier");

		// Use it in the other project: no copy, no new ingest.
		await card.click();
		await detail.getByLabel(`Library two ${stamp}`).check();
		await detail.getByRole("button", { name: "Use" }).click();
		await expect(detail.getByRole("status")).toContainText("No copy was made");
		await expect(detail.getByTestId("asset-uses")).toContainText(
			`Library two ${stamp}`,
		);

		const media = (await call("GET", `/projects/${two}/media`)) as {
			assets: { state: string }[];
		};
		expect(media.assets).toHaveLength(1);
		expect(media.assets[0]?.state).toBe("ready");

		// The workspace outlives the test and holds at most twenty tags a file: leave it as it was found.
		await call("POST", `/assets/${hash}/tags`, {
			add: [],
			remove: [tag, "harbour"],
		});

		// The same file is not in the visual section.
		await page.goto("/visual-assets");
		await expect(
			page.getByRole("heading", { name: "Visual Assets", level: 1 }),
		).toBeVisible();
		await expect(page.locator(`[data-asset-hash="${hash}"]`)).toHaveCount(0);
	});

	// VV-321: a project with two timelines asks in which one the file goes; the person picks it in the page.
	test("use a file in a project with two timelines by choosing the timeline", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		const stamp = Date.now();
		const one = (
			(await call("POST", "/projects", { name: `Library src ${stamp}` })) as {
				project_id: string;
			}
		).project_id;
		const two = (
			(await call("POST", "/projects", {
				name: `Library two tl ${stamp}`,
			})) as {
				project_id: string;
			}
		).project_id;
		await call("POST", `/projects/${two}/items`, {
			kind: "timeline",
			title: "Second cut",
			preset: "16:9",
		});
		const imported = (await call("POST", `/projects/${one}/media`, {
			files: [{ path: testClip(8) }],
			wait_ms: 20_000,
		})) as { assets: { hash: string }[] };
		const hash = imported.assets[0]?.hash;
		expect(hash).toMatch(/^sha256:[0-9a-f]{64}$/);

		await page.goto("/video-assets");
		await page.locator(`[data-asset-hash="${hash}"]`).click();
		const detail = page.getByTestId("asset-detail");
		await detail.getByLabel(`Library two tl ${stamp}`).check();
		await expect(detail.getByRole("button", { name: "Use" })).toBeDisabled();
		const timelines = detail.getByTestId("asset-use-timelines");
		await timelines.getByLabel("Second cut").check();
		await detail.getByRole("button", { name: "Use" }).click();
		await expect(detail.getByRole("status")).toContainText("No copy was made");
	});

	// VV-321: while the page asks which timelines a project holds, Use waits; an answer about a project the person
	// has left is dropped, and the file goes into the timeline the person picked.
	test("Use waits for the timelines of the chosen project and keeps only the last choice", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		const stamp = Date.now();
		const create = async (name: string): Promise<string> =>
			((await call("POST", "/projects", { name })) as { project_id: string })
				.project_id;
		const one = await create(`Library single ${stamp}`);
		const two = await create(`Library pair ${stamp}`);
		const added = (await call("POST", `/projects/${two}/items`, {
			kind: "timeline",
			title: "Wide cut",
			preset: "16:9",
		})) as { item: { timeline_id: string } };
		const second = added.item.timeline_id;
		const imported = (await call("POST", `/projects/${one}/media`, {
			files: [{ path: testClip(9) }],
			wait_ms: 20_000,
		})) as { assets: { hash: string }[] };
		const hash = imported.assets[0]?.hash;
		expect(hash).toMatch(/^sha256:[0-9a-f]{64}$/);

		// The answer about the project of two is held until the test lets it go.
		let release: () => void = () => undefined;
		const held = new Promise<void>((resolve) => {
			release = resolve;
		});
		const describePair = `**/api/v1/projects/${two}/describe**`;
		await page.route(describePair, async (route) => {
			await held;
			await route.continue();
		});

		await page.goto("/video-assets");
		await page.locator(`[data-asset-hash="${hash}"]`).click();
		const detail = page.getByTestId("asset-detail");
		const use = detail.getByRole("button", { name: "Use" });
		const timelines = detail.getByTestId("asset-use-timelines");

		await detail.getByLabel(`Library pair ${stamp}`).check();
		await expect(use).toBeDisabled();

		// The person moves to the project of one timeline before the first answer comes.
		await detail.getByLabel(`Library single ${stamp}`).check();
		await expect(use).toBeEnabled();
		const late = page.waitForResponse((r) =>
			r.url().includes(`/api/v1/projects/${two}/describe`),
		);
		release();
		await late;
		await expect(timelines).toHaveCount(0);
		await expect(use).toBeEnabled();

		await page.unroute(describePair);
		await detail.getByLabel(`Library pair ${stamp}`).check();
		await timelines.getByLabel("Wide cut").check();
		await use.click();
		await expect(detail.getByRole("status")).toContainText("No copy was made");

		// Both timelines started empty: the file is in the one the person picked, and only there.
		const mediaOf = async (timeline: string): Promise<number> =>
			(
				(await call(
					"GET",
					`/projects/${two}/media?timeline_id=${timeline}`,
				)) as { assets: unknown[] }
			).assets.length;
		expect(await mediaOf(second)).toBe(1);
		expect(await mediaOf(two)).toBe(0);
	});

	test("the sections speak Portuguese", async ({ page }) => {
		await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
		await page.goto("/visual-assets");
		await expect(
			page.getByRole("heading", { name: "Recursos Visuais", level: 1 }),
		).toBeVisible();
		await expect(page.getByLabel("Buscar")).toBeVisible();
		await expect(page.getByLabel("Só reutilizáveis")).toBeVisible();
	});
});
