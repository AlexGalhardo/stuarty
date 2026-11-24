import { execFileSync } from "node:child_process";
import { mkdirSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import AxeBuilder from "@axe-core/playwright";

import {
	caller,
	expect,
	ffmpegExe,
	seedProject,
	solidClip,
	test,
} from "./helpers";

// Epic 85 task 7 (D-137, SDD-011 section 15): the Animation panel of the editor is a thin screen over the
// operations of epic 33. It shows the brief, tells the person to render on their own side, imports the file
// they made with its origin and shows the check the service computed. Nothing in it runs an engine.
test.describe("animation hand-off panel", () => {
	test.use({ viewport: { width: 1600, height: 900 } });

	interface Brief {
		width: number;
		height: number;
		frame_rate: { num: number; den: number };
		duration_frames: number;
	}

	// A render that fits the brief of the project: its size, its frame rate and its length, in one colour.
	function renderedFixture(brief: Brief, colour: string): string {
		const dir = join(tmpdir(), "stuarty-e2e-media");
		mkdirSync(dir, { recursive: true });
		const out = join(
			dir,
			`motion-${colour}-${brief.width}x${brief.height}-${brief.duration_frames}.mp4`,
		);
		execFileSync(
			ffmpegExe(),
			[
				"-y",
				"-f",
				"lavfi",
				"-i",
				`color=c=${colour}:s=${brief.width}x${brief.height}:r=${brief.frame_rate.num}/${brief.frame_rate.den}`,
				"-frames:v",
				String(brief.duration_frames),
				"-c:v",
				"libopenh264",
				"-pix_fmt",
				"yuv420p",
				out,
			],
			{ stdio: "ignore" },
		);
		return out;
	}

	test("shows the brief, imports a rendered file and shows that it fits, and the engine is never run", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const call = await caller(request);
		const brief = (await call(
			"GET",
			`/projects/${seeded.id}/motion/brief`,
		)) as unknown as Brief;
		const file = renderedFixture(brief, "blue");

		const calls: string[] = [];
		page.on("request", (req) => {
			const url = new URL(req.url());
			if (url.pathname.startsWith("/api/v1/")) {
				calls.push(`${req.method()} ${url.pathname}`);
			}
		});

		await page.goto(`/editor/${seeded.id}`);
		await page.getByRole("tab", { name: "Animation" }).click();
		const panel = page.getByRole("region", { name: "Animation" });
		await expect(panel).toBeVisible();
		// The reminder that the engine is the person's, before anything is asked of the service.
		await expect(panel).toContainText("never installs or runs the engine");

		await panel.getByRole("button", { name: "Show the brief" }).click();
		const shown = panel.locator("[data-motion-brief]");
		await expect(shown).toContainText(`${brief.width} x ${brief.height}`);

		await panel.getByLabel("Path of the rendered file").fill(file);
		await panel
			.getByLabel("Where the animation project lives")
			.fill("hyperframes/lower-third");
		await panel.getByLabel("Name of this animation").fill("lower-third");
		await panel.getByRole("button", { name: "Import and check" }).click();
		await expect(panel.locator("[data-motion-verdict]")).toHaveAttribute(
			"data-motion-verdict",
			"fits",
			{ timeout: 30_000 },
		);
		await expect(panel.locator("[data-mismatch]")).toHaveCount(0);

		// The operations of the hand-off and the media import, and nothing that installs or starts an engine.
		expect(calls).toEqual(
			expect.arrayContaining([
				`GET /api/v1/projects/${seeded.id}/motion/brief`,
				`GET /api/v1/projects/${seeded.id}/motion/check`,
				`POST /api/v1/projects/${seeded.id}/media`,
			]),
		);
		expect(calls.some((c) => /engine|hyperframes|install/i.test(c))).toBe(
			false,
		);
	});

	test("names what does not fit, with the value expected and found", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await page.getByRole("tab", { name: "Animation" }).click();
		const panel = page.getByRole("region", { name: "Animation" });
		// 160x90 is not the size of the project.
		await panel.getByLabel("Path of the rendered file").fill(solidClip("red"));
		await panel.getByLabel("Where the animation project lives").fill("x");
		await panel.getByRole("button", { name: "Import and check" }).click();
		await expect(panel.locator("[data-motion-verdict]")).toHaveAttribute(
			"data-motion-verdict",
			"mismatch",
			{ timeout: 30_000 },
		);
		const size = panel.locator("[data-mismatch='size']");
		await expect(size).toContainText("Expected");
		await expect(size).toContainText("160x90");
	});

	test("offers to replace the earlier render of the same animation, in one undoable batch", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const call = await caller(request);
		const brief = (await call(
			"GET",
			`/projects/${seeded.id}/motion/brief`,
		)) as unknown as Brief;

		// The first render, imported with its key and put on the timeline as a clip.
		const first = (await call("POST", `/projects/${seeded.id}/media`, {
			files: [{ path: renderedFixture(brief, "green") }],
			origin: { kind: "rendered_externally", source: "x", render_key: "intro" },
			wait_ms: 20000,
		})) as { assets: { asset_id: string }[] };
		const firstId = first.assets[0]?.asset_id ?? "";
		const head = (await call("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "clip.add",
					args: {
						track_id: seeded.trackIds[0],
						asset_id: firstId,
						start: "2s",
						source_in: "0s",
						duration: "1s",
					},
				},
			],
		});

		// The clip made the timeline longer, so the new render is made to the new brief.
		const longer = (await call(
			"GET",
			`/projects/${seeded.id}/motion/brief`,
		)) as unknown as Brief;
		await page.goto(`/editor/${seeded.id}`);
		await page.getByRole("tab", { name: "Animation" }).click();
		const panel = page.getByRole("region", { name: "Animation" });
		await panel
			.getByLabel("Path of the rendered file")
			.fill(renderedFixture(longer, "yellow"));
		await panel.getByLabel("Where the animation project lives").fill("x");
		await panel.getByLabel("Name of this animation").fill("intro");
		await panel.getByRole("button", { name: "Import and check" }).click();
		const offer = panel.locator("[data-motion-offer]");
		await expect(offer).toContainText("1 clip uses an earlier render", {
			timeout: 30_000,
		});
		await offer
			.getByRole("button", { name: "Replace the earlier render" })
			.click();
		await expect(offer).toContainText("Replaced in 1 clip");
	});

	for (const theme of ["light", "dark"] as const) {
		test(`axe finds nothing in the panel in ${theme}`, async ({
			page,
			request,
		}) => {
			await page.addInitScript((th) => {
				if (th === "light") localStorage.setItem("theme", "light");
			}, theme);
			const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
			await page.goto(`/editor/${seeded.id}`);
			await page.getByRole("tab", { name: "Animation" }).click();
			const panel = page.getByRole("region", { name: "Animation" });
			await panel.getByRole("button", { name: "Show the brief" }).click();
			await expect(panel.locator("[data-motion-brief]")).toBeVisible();
			const { violations } = await new AxeBuilder({ page })
				.include("aside[data-landmark='2']")
				.withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
				.analyze();
			expect(violations.map((v) => v.id)).toEqual([]);
		});
	}
});
