import { randomBytes } from "node:crypto";
import { rmSync } from "node:fs";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import type { Page } from "@playwright/test";

import { ensureAccount, signInWithPassword } from "../support/account";
import { caller, expect, home, seedProject, solidClip, test } from "./helpers";

// Epic 94 (SDD-011 section 14.10, SDD-012 section 23.5 item 5): the folder of a stored file. The page sends the
// hash, the service checks the session, the file and the path; the browser in local mode copies the path, the
// desktop app hands it to the shell, a session that is not a person's cannot, and a refusal is shown as it came.
// The desktop shell itself is not in a browser: a stub of window.vukemDesktop stands for it here, and the shell's
// own checks are in the Go tests of desktop/shell.
test.describe.configure({ timeout: 180_000 });

const JSON_HEADERS = {
	"Content-Type": "application/json",
	"Sec-Fetch-Site": "same-origin",
};

const colour = (): string => `0x${randomBytes(3).toString("hex")}`;

async function seededFile(
	request: import("@playwright/test").APIRequestContext,
): Promise<{ hash: string; name: string }> {
	const clip = colour();
	const seeded = await seedProject(request, {
		tracks: 0,
		files: [solidClip(clip)],
	});
	const call = await caller(request);
	const found = (await call("GET", `/assets?project=${seeded.id}`)) as {
		assets: { hash: string }[];
	};
	return { hash: found.assets[0]?.hash ?? "", name: `solid-${clip}.mp4` };
}

// The session of a person: the password sign-in of the account every server of the run has (ADR-0052).
async function signInAsPerson(page: Page): Promise<void> {
	await ensureAccount(page.request, home);
	await signInWithPassword(page.request);
}

async function pathOf(page: Page, hash: string): Promise<string> {
	const res = await page.request.post(`/api/v1/assets/${hash}/reveal`, {
		headers: JSON_HEADERS,
		data: {},
	});
	expect(res.ok(), await res.text()).toBe(true);
	return ((await res.json()) as { path: string }).path;
}

test.describe("show in folder and copy path", () => {
	test.use({ viewport: { width: 1440, height: 900 } });

	test("a session that is not a person's sees the action off, with the reason", async ({
		page,
		request,
	}) => {
		const { hash } = await seededFile(request);
		await page.goto("/video-assets");
		await page.locator(`button[data-asset-hash="${hash}"]`).click();
		const detail = page.getByTestId("asset-detail");
		const copy = detail.getByRole("button", { name: "Copy path" });
		await expect(copy).toBeDisabled();
		await expect(detail).toContainText(
			"Only a person in the Stuarty desktop app can do this",
		);

		// The service says the same, whoever asks with the token alone.
		const token = (await readFile(join(home, "token"), "utf8")).trim();
		const refused = await request.post(`/api/v1/assets/${hash}/reveal`, {
			headers: { Authorization: `Bearer ${token}` },
			data: {},
		});
		expect(refused.status()).toBe(403);
		expect(((await refused.json()) as { code: string }).code).toBe(
			"E_PRIVILEGED_SESSION",
		);
	});

	test("a person copies the path from the detail and from the card menu, and the path is shown isolated", async ({
		page,
		request,
		context,
		browserName,
	}) => {
		test.skip(browserName !== "chromium", "clipboard permissions: Chromium"); // verify:env-skip a Chromium-only feature
		await context.grantPermissions(["clipboard-read", "clipboard-write"]);
		const { hash } = await seededFile(request);
		await signInAsPerson(page);
		const expected = await pathOf(page, hash);
		expect(expected).toContain(hash.slice("sha256:".length));

		await page.goto("/video-assets");
		await page.locator(`button[data-asset-hash="${hash}"]`).click();
		const detail = page.getByTestId("asset-detail");
		await detail.getByRole("button", { name: "Copy path" }).click();
		const notice = detail.getByTestId("reveal-notice");
		await expect(notice).toContainText("Path copied:");
		await expect(notice.locator("[dir='auto']")).toBeVisible();
		expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
			expected,
		);

		// The same from the menu of the card, with the notice above the grid.
		await page.evaluate(() => navigator.clipboard.writeText("nothing"));
		await page
			.locator(`button[data-asset-hash="${hash}"]`)
			.click({ button: "right" });
		await page.getByRole("menuitem", { name: "Copy path" }).click();
		await expect(page.getByTestId("reveal-notice").first()).toContainText(
			"Path copied:",
		);
		expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(
			expected,
		);
	});

	test("the desktop app hands the path to the shell, and says what the shell refused", async ({
		page,
		request,
	}) => {
		const { hash } = await seededFile(request);
		await signInAsPerson(page);
		const expected = await pathOf(page, hash);

		await page.addInitScript(() => {
			const w = window as unknown as {
				__revealed: string[];
				__refuse: boolean;
				vukemDesktop: unknown;
			};
			w.__revealed = [];
			w.__refuse = false;
			w.vukemDesktop = {
				revealAsset: async (path: string) => {
					w.__revealed.push(path);
					if (w.__refuse)
						throw new Error("no file manager was found on this computer");
				},
			};
		});
		await page.goto("/video-assets");
		await page.locator(`button[data-asset-hash="${hash}"]`).click();
		const detail = page.getByTestId("asset-detail");
		// With the shell's action the browser's copy is not offered: the desktop shows the folder.
		await expect(detail.getByRole("button", { name: "Copy path" })).toHaveCount(
			0,
		);
		await detail.getByRole("button", { name: "Show in folder" }).click();
		await expect(detail.getByTestId("reveal-notice")).toContainText(
			"Opened the folder of the file.",
		);
		expect(
			await page.evaluate(
				() => (window as never as { __revealed: string[] }).__revealed,
			),
		).toEqual([expected]);

		await page.evaluate(() => {
			(window as never as { __refuse: boolean }).__refuse = true;
		});
		await detail.getByRole("button", { name: "Show in folder" }).click();
		await expect(detail.getByRole("alert")).toContainText(
			"no file manager was found on this computer",
		);
	});

	test("a file missing from the store is listed as missing and refused with its words and no path", async ({
		page,
		request,
	}) => {
		const { hash } = await seededFile(request);
		await signInAsPerson(page);
		const stored = await pathOf(page, hash);
		rmSync(stored);

		await page.goto("/video-assets");
		const card = page.locator(`button[data-asset-hash="${hash}"]`);
		await expect(card.getByTestId("asset-size")).toHaveAttribute(
			"data-state",
			"missing",
		);
		await expect(card).toContainText("File missing");
		await expect(card).not.toContainText("0 B");

		await card.click();
		const detail = page.getByTestId("asset-detail");
		await detail.getByRole("button", { name: "Copy path" }).click();
		const alert = detail.getByRole("alert");
		await expect(alert).toContainText("the file is missing from the workspace");
		await expect(alert).not.toContainText(home);
		await expect(alert).not.toContainText(/[\\/]store[\\/]/);
	});
});
