import { execFileSync } from "node:child_process";
import { mkdirSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import type { Page } from "@playwright/test";

import { home } from "./helpers";
import { editImage, expect, imageState, newImage, test } from "./image-support";

// Epic 48 (acceptance 3): an editor open on an image shows what an agent or another window committed from the
// event stream of the image (SDD-006 section 12), without a reload and without polling while the stream is up.
test.describe("image live", () => {
	test.describe.configure({ timeout: 120_000 });

	let exe = "";
	let dir = "";

	test.beforeAll(() => {
		// The CLI is the binary of the product; with the workspace of the run it finds the server that owns it.
		dir = join(tmpdir(), `stuarty-e2e-live-${process.pid}`);
		mkdirSync(dir, { recursive: true });
		exe = join(dir, process.platform === "win32" ? "stuarty.exe" : "stuarty");
		execFileSync("go", ["build", "-o", exe, "./web/cmd/app"], {
			cwd: resolve(process.cwd(), ".."),
			stdio: "ignore",
		});
	});

	test.afterAll(() => {
		if (dir !== "") rmSync(dir, { recursive: true, force: true });
	});

	const cli = (...args: string[]): void => {
		try {
			execFileSync(exe, [...args, "--json"], {
				env: { ...process.env, STUARTY_HOME: home },
				encoding: "utf8",
				stdio: ["ignore", "pipe", "pipe"],
			});
		} catch (error) {
			const failed = error as { stdout?: string; stderr?: string };
			throw new Error(
				`stuarty ${args.join(" ")} failed: ${failed.stdout ?? ""}${failed.stderr ?? ""}`,
			);
		}
	};

	const row = (page: Page, name: string) =>
		page.getByRole("treeitem", { name: new RegExp(`^${name},`) });

	// Counts the reads of the document a window makes, and the streams it opens.
	function watch(page: Page, id: string): { reads: number; streams: number } {
		const seen = { reads: 0, streams: 0 };
		page.on("request", (request) => {
			const { pathname } = new URL(request.url());
			if (request.method() !== "GET") return;
			if (pathname === `/api/v1/images/${id}`) seen.reads++;
			if (pathname === `/api/v1/images/${id}/events`) seen.streams++;
		});
		return seen;
	}

	// Counts, in the page, the event streams that opened: the listener is added before the one of the screen, so
	// once the count moved the screen knows its stream is open too.
	function countOpenStreams(): void {
		const Native = window.EventSource;
		const record = window as unknown as { openStreams: number };
		record.openStreams = 0;
		window.EventSource = class extends Native {
			constructor(url: string | URL, init?: EventSourceInit) {
				super(url, init);
				this.addEventListener("open", () => {
					record.openStreams++;
				});
			}
		};
	}

	const openStreams = (target: Page): Promise<number> =>
		target.evaluate(
			() => (window as unknown as { openStreams?: number }).openStreams ?? 0,
		);

	// A round trip to each page, so the requests a moved clock made have been reported to the test.
	async function settle(...pages: Page[]): Promise<void> {
		for (const target of pages) await target.evaluate(() => 0);
	}

	test("an edit by the CLI appears in a second window within a second, with no polling while the stream is up", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Live spec" });
		await editImage(call, id, [
			{ op: "layer.add_fill", args: { color: "#203040", name: "Back" } },
		]);
		// The clock of the pages is the test's, so the intervals of the poll are moved, not waited for.
		const clock = page.context().clock;
		await clock.install();
		await page.context().addInitScript(countOpenStreams);
		const second = await page.context().newPage();
		const first = watch(page, id);
		const other = watch(second, id);
		await Promise.all([
			page.goto(`/edit-image/${id}`),
			second.goto(`/edit-image/${id}`),
		]);
		await expect(row(page, "Back")).toBeVisible();
		await expect(row(second, "Back")).toBeVisible();
		await expect.poll(() => openStreams(page)).toBeGreaterThan(0);
		await expect.poll(() => openStreams(second)).toBeGreaterThan(0);
		expect(first.streams).toBeGreaterThan(0);
		expect(other.streams).toBeGreaterThan(0);

		// A window that is up and has its stream does not ask for the document on a timer: two intervals of the
		// poll (2 seconds each) pass and the reads stay where they were.
		const readsBefore = first.reads + other.reads;
		await clock.runFor(4500);
		await settle(page, second);
		expect(first.reads + other.reads).toBe(readsBefore);

		cli("image:edit", id, "layer.add_fill", "color=#112233", "name=Circle");
		const returned = Date.now();
		await expect(row(second, "Circle")).toBeVisible({ timeout: 1000 });
		await expect(row(page, "Circle")).toBeVisible({ timeout: 1000 });
		expect(Date.now() - returned).toBeLessThan(1000);

		// Each window read the document once, because of the event, and the notice names the actor.
		await expect(second.getByTestId("image-agent-notice")).toContainText(
			"local:cli",
		);
		expect(first.reads + other.reads).toBe(readsBefore + 2);
		await clock.runFor(4500);
		await settle(page, second);
		expect(first.reads + other.reads).toBe(readsBefore + 2);
		expect((await imageState(call, id)).layers.map((l) => l.name)).toContain(
			"Circle",
		);
	});

	test("a window's own edit is applied once, and an edit of another window follows it", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Own edits" });
		const second = await page.context().newPage();
		await Promise.all([
			page.goto(`/edit-image/${id}`),
			second.goto(`/edit-image/${id}`),
		]);
		await expect(page.getByTestId("image-frame")).toBeVisible();
		await page.getByTestId("image-add-fill").click();
		await expect(row(page, "Fill")).toBeVisible();
		await expect(row(second, "Fill")).toBeVisible({ timeout: 1000 });
		const state = await imageState(call, id);
		// one batch, one revision: the window that made it did not apply its own event a second time
		expect(state.layers.filter((l) => l.name === "Fill")).toHaveLength(1);
		await expect(page.getByTestId("image-frame")).toHaveAttribute(
			"data-revision",
			String(state.revision),
		);
	});

	test("while the stream is refused the window polls the revision, and the edit still arrives", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Fallback" });
		await page.route("**/api/v1/images/*/events*", (route) => route.abort());
		const seen = watch(page, id);
		await page.goto(`/edit-image/${id}`);
		await expect(page.getByTestId("image-frame")).toBeVisible();
		await editImage(call, id, [
			{ op: "layer.add_fill", args: { color: "#304050", name: "Polled" } },
		]);
		await expect(row(page, "Polled")).toBeVisible({ timeout: 8000 });
		expect(seen.reads).toBeGreaterThan(1);
	});
});
