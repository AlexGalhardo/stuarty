import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdirSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";

import { home } from "./helpers";
import {
	editImage,
	expect,
	imageState,
	newImage,
	seedPhoto,
	test,
} from "./image-support";

// Epic 28 task 7: the export dialog, its findings, and the downloaded file against the CLI export.
test.describe("image export", () => {
	test.describe.configure({ timeout: 180_000 });

	let exe = "";
	let dir = "";

	test.beforeAll(() => {
		// The CLI is the binary of the product; with the workspace of the run it finds the server that owns it
		// and talks to it over HTTP (SDD-003 section 5.2), the same service the screen reaches.
		dir = join(tmpdir(), `stuarty-e2e-cli-${process.pid}`);
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

	const cli = (...args: string[]): unknown =>
		JSON.parse(
			execFileSync(exe, args, {
				env: { ...process.env, STUARTY_HOME: home },
				encoding: "utf8",
			}),
		);

	test("exports a PNG whose bytes equal the CLI export of the same revision", async ({
		page,
		request,
	}) => {
		const photo = await seedPhoto(request, 0);
		const { id, call } = await newImage(request, { name: "Export spec" });
		await editImage(call, id, [
			{ op: "layer.add_fill", args: { color: "#203040" } },
			{ op: "layer.add_image", args: { asset: photo.hash, fit: "cover" } },
			{
				op: "layer.add_text",
				args: {
					text: "EXPORTED",
					style: { size: 120, color: "#FFE600", font_family: "Anton" },
				},
			},
		]);
		const revision = (await imageState(call, id)).revision;

		await page.goto(`/edit-image/${id}`);
		await page.getByTestId("image-export-open").click();
		const dialog = page.getByTestId("image-export-dialog");
		await expect(dialog).toBeVisible();
		await dialog.getByTestId("image-export-run").click();
		const done = page.getByTestId("image-export-result");
		await expect(done).toBeVisible();
		await expect(done).toContainText("1280 x 720");

		const download = page.waitForEvent("download");
		await page.getByTestId("image-export-download").click();
		const file = await download;
		const saved = join(tmpdir(), `stuarty-e2e-${id}.png`);
		await file.saveAs(saved);
		const mine = readFileSync(saved);

		const exported = cli("image:export", id, "--json") as {
			revision: number;
			path: string;
			sha256: string;
		};
		expect(exported.revision).toBe(revision);
		const theirs = readFileSync(exported.path);
		expect(createHash("sha256").update(mine).digest("hex")).toBe(
			createHash("sha256").update(theirs).digest("hex"),
		);
		expect(Buffer.compare(mine, theirs)).toBe(0);
		expect(exported.sha256).toBe(
			createHash("sha256").update(mine).digest("hex"),
		);
		// the file is a PNG of the canvas size
		expect(mine.subarray(0, 8).toString("hex")).toBe("89504e470d0a1a0a");
		expect(mine.readUInt32BE(16)).toBe(1280);
		expect(mine.readUInt32BE(20)).toBe(720);
	});

	test("a preset size and JPEG with a quality give a file of that size", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Export size spec" });
		await editImage(call, id, [
			{ op: "layer.add_fill", args: { color: "#B03060" } },
		]);
		await page.goto(`/edit-image/${id}`);
		await page.getByTestId("image-export-open").click();
		await page.getByTestId("image-export-format-jpeg").check();
		await page.getByTestId("image-export-size").selectOption("square_post");
		await page.getByTestId("image-export-quality").fill("70");
		await page.getByTestId("image-export-run").click();
		const done = page.getByTestId("image-export-result");
		await expect(done).toContainText("1080 x 1080");
		const href = await page
			.getByTestId("image-export-download")
			.getAttribute("href");
		expect(href).toMatch(/\.jpg$/);
		const body = await (await page.request.get(href ?? "")).body();
		// JPEG magic
		expect(body.subarray(0, 3).toString("hex")).toBe("ffd8ff");
	});

	test("WebP shows the answer of the service while the encoder is not wired", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Export webp spec" });
		await editImage(call, id, [
			{ op: "layer.add_fill", args: { color: "#102030" } },
		]);
		await page.goto(`/edit-image/${id}`);
		await page.getByTestId("image-export-open").click();
		await page.getByTestId("image-export-format-webp").check();
		await page.getByTestId("image-export-run").click();
		await expect(
			page.getByTestId("image-export-dialog").getByRole("alert"),
		).toContainText(/WebP/i);
		await expect(page.getByTestId("image-export-result")).toHaveCount(0);
	});

	test("the findings are listed with a link to the layer, and an error that blocks turns the button off", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Export lint spec" });
		await editImage(call, id, [
			{ op: "layer.add_fill", args: { color: "#FFFFFF", name: "Back" } },
			{
				op: "layer.add_text",
				args: {
					text: "",
					slot: "title",
					name: "Title",
					style: { size: 90, color: "#FFFFFF" },
				},
			},
			{
				op: "layer.add_text",
				args: {
					text: "tiny",
					name: "Small",
					style: { size: 10, color: "#FFFFFF" },
				},
			},
		]);
		await page.goto(`/edit-image/${id}`);
		await page.getByTestId("image-export-open").click();
		const findings = page.getByTestId("image-export-findings");
		await expect(findings.getByRole("listitem").first()).toBeVisible();
		await expect(findings).toContainText("Error");
		await expect(findings).toContainText("Warning");
		await expect(page.getByTestId("image-export-run")).toBeDisabled();
		await expect(
			page.getByTestId("image-export-dialog").getByRole("alert"),
		).toContainText("error");

		// the link of an offending layer selects it and closes the dialog
		const state = await imageState(call, id);
		const title = state.layers.find((l) => l.name === "Title");
		await findings.getByTestId("image-finding-go").first().click();
		await expect(page.getByTestId("image-export-dialog")).toHaveCount(0);
		await expect(page.getByTestId("image-selection")).toHaveCount(1);
		expect(title).toBeDefined();

		// fill the slot: nothing blocks any more
		await page.getByTestId("image-field-text").fill("FILLED");
		await page.getByTestId("image-field-text").blur();
		await expect
			.poll(async () =>
				(await imageState(call, id)).findings.some(
					(f) => f.code === "L_IMAGE_EMPTY_SLOT",
				),
			)
			.toBe(false);
		await page.getByTestId("image-export-open").click();
		await expect(page.getByTestId("image-export-run")).toBeEnabled();
	});
});
