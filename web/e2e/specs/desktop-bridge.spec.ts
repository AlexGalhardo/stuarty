import type { Page } from "@playwright/test";
import { expect, seedProject, test, testClip } from "./helpers";

// P3-DSK-003 (SDD-012 sections 13.1 item 2 and 21.1): the desktop shell defines
// window.vukemDesktop; the library then offers "Choose files" and accepts drops,
// and sends the paths to the same importMedia route. In a browser the object is
// absent and the Prototype message stays.

function recordImports(page: Page): string[][] {
	const imports: string[][] = [];
	page.on("request", (r) => {
		if (r.method() === "POST" && r.url().endsWith("/media")) {
			const body = r.postDataJSON() as { files: { path: string }[] };
			imports.push(body.files.map((f) => f.path));
		}
	});
	return imports;
}

async function fakeBridge(page: Page, picked: string[]): Promise<void> {
	await page.addInitScript((paths) => {
		const w = window as unknown as Record<string, unknown>;
		let drop: ((p: string[]) => void) | null = null;
		w.vukemDesktop = {
			pickFiles: () => Promise.resolve(paths),
			onDropPaths: (cb: (p: string[]) => void) => {
				drop = cb;
				return () => {
					drop = null;
				};
			},
		};
		w.__dropPaths = (p: string[]) => drop?.(p);
		const revealed: string[] = [];
		w.__revealed = revealed;
		(w.vukemDesktop as Record<string, unknown>).showInFolder = (p: string) => {
			revealed.push(p);
			return Promise.resolve([]);
		};
	}, picked);
}

test.describe("desktop-bridge", () => {
	test("Choose files sends one importMedia request per picked path", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		const picked = [testClip(1), testClip(2)];
		await fakeBridge(page, picked);
		await page.goto(`/editor/${seeded.id}`);
		const imports = recordImports(page);

		await page.getByRole("button", { name: "Choose files" }).click();
		await expect.poll(() => imports.length).toBe(2);
		expect(imports).toEqual([[picked[0]], [picked[1]]]);
	});

	test("dropped paths go to importMedia and the browser message stays hidden", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		await fakeBridge(page, []);
		await page.goto(`/editor/${seeded.id}`);
		await expect(
			page.getByRole("button", { name: "Choose files" }),
		).toBeVisible();
		const imports = recordImports(page);

		const dropped = [testClip(3)];
		await page.evaluate((paths) => {
			(window as unknown as { __dropPaths(p: string[]): void }).__dropPaths(
				paths,
			);
		}, dropped);
		await expect.poll(() => imports.length).toBe(1);
		expect(imports[0]).toEqual(dropped);

		await page.getByRole("region", { name: "Media" }).dispatchEvent("drop");
		await expect(page.getByText("A browser page cannot see")).toHaveCount(0);
		await expect(page.getByText("The editor imports by path")).toHaveCount(0);
	});

	test("without the object the button is absent and the plain message shows", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.getByRole("button", { name: "Import" })).toBeVisible();
		await expect(
			page.getByRole("button", { name: "Choose files" }),
		).toHaveCount(0);

		// VV-062: the page explains why the import is by path.
		await expect(page.getByText("The editor imports by path")).toBeVisible();

		await page.getByRole("region", { name: "Media" }).dispatchEvent("drop");
		await expect(
			page.getByText("A browser page cannot see the path of a file"),
		).toBeVisible();
	});
});

test.describe("desktop-bridge show in folder", () => {
	// VV-027: a finished export offers "Show in folder" in the window, with the
	// path the server wrote; a browser has no such button.
	test("a finished export is shown in its folder through the bridge", async ({
		page,
		request,
	}) => {
		test.setTimeout(120_000);
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await fakeBridge(page, []);
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.getByRole("button", { name: "Export" }).click();
		await page.getByRole("button", { name: "Start export" }).click();
		const dialog = page.getByRole("dialog");
		await expect(dialog.getByText("Export finished")).toBeVisible({
			timeout: 90_000,
		});
		await dialog.getByRole("button", { name: "Show in folder" }).click();
		const revealed = await page.evaluate(
			() => (window as unknown as { __revealed: string[] }).__revealed,
		);
		expect(revealed).toHaveLength(1);
		expect(revealed[0]).toMatch(/\.mp4$/);
	});
});

test.describe("desktop-bridge first-run tour", () => {
	test.use({ tipsSeen: false });

	// VV-054: the library step does not say "paste a path" only, as the window
	// also has the button and the drop.
	test("the library step names Choose files in the window", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		await fakeBridge(page, []);
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-tour-card]")).toContainText(
			"Choose files, drop them here",
		);
	});

	test("the library step says to paste a path in a browser", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-tour-card]")).toContainText(
			"Paste the path of a media file",
		);
	});
});

// TASK-0295 (SDD-012 section 21.3 item 4): the native menu bar sends an action id
// through onMenuAction, and the page runs the same action as its own control.
test.describe("desktop-bridge native menu", () => {
	async function menuBridge(page: Page): Promise<void> {
		await page.addInitScript(() => {
			const w = window as unknown as Record<string, unknown>;
			const callbacks: ((action: string) => void)[] = [];
			w.vukemDesktop = {
				onMenuAction: (cb: (action: string) => void) => {
					callbacks.push(cb);
					return () => {
						callbacks.splice(callbacks.indexOf(cb), 1);
					};
				},
			};
			w.__menu = (action: string) => {
				for (const cb of callbacks) cb(action);
			};
		});
	}

	test("a menu action runs the page action, and an unknown id does nothing", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		await menuBridge(page);
		const batches: string[][] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/edit")) {
				const body = r.postDataJSON() as { operations: { op: string }[] };
				batches.push(body.operations.map((o) => o.op));
			}
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const call = (action: string): Promise<void> =>
			page.evaluate(
				(a) => (window as unknown as { __menu(a: string): void }).__menu(a),
				action,
			);

		await call("surface.select_all");
		await expect(
			page.locator("[data-item-id][aria-selected='true']"),
		).toHaveCount(1);
		await call("not.an.action");
		await call("addTrack.audio");
		await expect.poll(() => batches).toEqual([["track.add"]]);
		await call("surface.cheat_sheet");
		await expect(
			page.getByRole("dialog", { name: "Keyboard shortcuts" }),
		).toBeVisible();
	});
});
