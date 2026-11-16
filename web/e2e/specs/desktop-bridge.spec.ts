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

test.describe("desktop-bridge first-run tip", () => {
	test.use({ tipsSeen: false });

	// VV-054: the tip does not say "paste a path" only, as the window also has
	// the button and the drop.
	test("the library tip names Choose files in the window", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		await fakeBridge(page, []);
		await page.goto(`/editor/${seeded.id}`);
		const tips = page.getByRole("region", { name: "First steps" });
		await expect(tips).toContainText("Choose files in the library, drop them");
	});

	test("the library tip says to paste a path in a browser", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(
			page.getByRole("region", { name: "First steps" }),
		).toContainText("Paste the path of a media file in the library");
	});
});
