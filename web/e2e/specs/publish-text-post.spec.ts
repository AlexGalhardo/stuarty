import { randomBytes } from "node:crypto";
import { existsSync, readdirSync } from "node:fs";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { Page } from "@playwright/test";
import { ensureAccount, signInWithPassword } from "../support/account";
import { caller, expect, home, test } from "./helpers";

// Epic 82 (SDD-015, SDD-020 part 5, D-134, ADR-0032): a text project publishes a post with no media. The creator
// plans it to a local destination (a folder of files, no network), sees on the approval what happens, the item with its
// words and address, where, at what cost and that it cannot be undone, a person approves it, and the folder holds
// the post and nothing else. A destination that cannot take the content is shown disabled with its reason.
test.describe.configure({ mode: "serial" });

const folders: string[] = [];

test.afterAll(async () => {
	await Promise.all(
		folders.map((folder) => rm(folder, { recursive: true, force: true })),
	);
});

async function signInAsPerson(page: Page): Promise<void> {
	await ensureAccount(page.request, home);
	await signInWithPassword(page);
	await page.goto("/");
}

function localField(date: Date): string {
	const pad = (n: number): string => String(n).padStart(2, "0");
	return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

interface Publication {
	id: string;
	targets: { id: string; state: string; network: string }[];
}

test.describe("publish a text post @chromium-only", () => {
	test("a text post goes to a blog with no media: destination, approval, published", async ({
		page,
		request,
		browserName,
	}) => {
		test.setTimeout(300_000);
		const api = await caller(request);
		const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
		const slug = `text-only-${tag}`;
		const folder = await mkdtemp(join(tmpdir(), "stuarty-text-post-"));
		folders.push(folder);

		// A text project: no timeline, no video, no media anywhere (D-134).
		const made = (await api("POST", "/projects", {
			name: `Text ${tag}`,
			kind: "text",
		})) as { project_id: string };
		const projectId = made.project_id;
		await api("POST", `/projects/${projectId}/items`, {
			kind: "text",
			title: `Post ${tag}`,
			text_kind: "post",
			text: `---\ntitle: Words only ${tag}\nslug: ${slug}\nsummary: One sentence.\ntags: notes\nlanguage: en\n---\nJust a few words, no picture and no video.\n`,
		});
		const created = (await api("POST", "/connections", {
			kind: "blog_files",
			label: `Folder ${tag}`,
			endpoint: "http://localhost",
			accepts: ["text", "image", "metadata"],
			secret: JSON.stringify({ folder, template: "hugo" }),
		})) as { id: string };

		await signInAsPerson(page);
		await page.goto("/publish");
		await page.getByTestId("publish-new").click();
		const dialog = page.getByTestId("publication-editor");
		await dialog.getByLabel("Project").selectOption(projectId);
		await dialog.getByLabel("What you publish").selectOption("post");
		await dialog
			.getByLabel("Blog post", { exact: true })
			.selectOption({ label: `Post ${tag}` });
		await dialog.getByLabel("Title", { exact: true }).fill(`Words only ${tag}`);
		await dialog.getByRole("button", { name: "Add an account" }).click();
		const target = dialog.getByTestId("target-panel");
		const account = target.getByLabel("Account", { exact: true });
		await account.selectOption(created.id);
		await target.getByLabel("Status at the destination").selectOption("public");
		const at = new Date(Date.now() + 30_000);
		at.setSeconds(0, 0);
		at.setMinutes(at.getMinutes() + 1);
		await target.getByLabel("Date and time").fill(localField(at));
		await dialog.getByRole("button", { name: "Save", exact: true }).click();
		await expect(target.getByTestId("target-state")).toHaveText("Draft");
		await target.getByTestId("target-schedule").click();
		await expect(target.getByTestId("target-state")).toHaveText(
			"Waiting for approval",
		);
		await page.keyboard.press("Escape");

		// The approval says what happens, the item, where, the cost and the undo, with no media preview.
		await page.goto("/approvals");
		const item = page
			.locator("[data-approval-id]")
			.filter({ hasText: `Words only ${tag}` });
		await expect(item).toHaveCount(1);
		await expect(item.getByTestId("approval-summary")).toContainText(slug);
		await expect(item.getByTestId("approval-destination")).toHaveText(
			`Folder ${tag}`,
		);
		await expect(item.getByTestId("approval-cost")).toHaveText("Free");
		await expect(item.getByTestId("approval-undo")).toHaveText(
			"Cannot be undone",
		);
		await expect(item.getByTestId("approval-path")).toContainText(
			"Waiting for approval",
		);
		await expect(item.getByTestId("approval-words")).toHaveText(/\d+ words?/);
		await expect(item.getByTestId("approval-address")).toHaveText(slug);
		await expect(item.getByTestId("approval-excerpt")).toContainText(
			"Just a few words",
		);
		await expect(item.locator("img, video, audio, canvas")).toHaveCount(0);
		expect(existsSync(join(folder, slug))).toBe(false);

		await item.getByRole("button", { name: "Approve" }).click();
		await expect(item).toHaveCount(0);

		const pubs = (await api(
			"GET",
			`/publications?project_id=${projectId}`,
		)) as { publications: Publication[] };
		const id = pubs.publications[0]?.id ?? "";
		await expect
			.poll(
				async () =>
					((await api("GET", `/publications/${id}`)) as Publication).targets[0]
						?.state,
				{ timeout: 150_000 },
			)
			.toBe("published");

		// Sent with no media: the folder of the post holds the text and nothing else.
		expect(readdirSync(join(folder, slug))).toEqual(["index.md"]);
		expect(await readFile(join(folder, slug, "index.md"), "utf8")).toContain(
			"Just a few words",
		);
		await api("DELETE", `/connections/${created.id}`);
	});

	test("a destination that cannot take the content is disabled and says why", async ({
		page,
		request,
		browserName,
	}) => {
		const api = await caller(request);
		const tag = `${browserName}-${randomBytes(3).toString("hex")}`;
		const folder = await mkdtemp(join(tmpdir(), "stuarty-text-post-"));
		folders.push(folder);
		const made = (await api("POST", "/projects", {
			name: `Reason ${tag}`,
		})) as { project_id: string };
		const created = (await api("POST", "/connections", {
			kind: "blog_files",
			label: `Folder ${tag}`,
			endpoint: "http://localhost",
			accepts: ["text", "image", "metadata"],
			secret: JSON.stringify({ folder, template: "hugo" }),
		})) as { id: string };
		try {
			await signInAsPerson(page);
			await page.goto("/publish");
			await page.getByTestId("publish-new").click();
			const dialog = page.getByTestId("publication-editor");
			await dialog.getByLabel("Project").selectOption(made.project_id);
			// A video is the default: a blog takes text, so it is listed, disabled, with the reason.
			await dialog.getByRole("button", { name: "Add an account" }).click();
			const target = dialog.getByTestId("target-panel");
			const option = target
				.getByLabel("Account", { exact: true })
				.locator(`option[value="${created.id}"]`);
			await expect(option).toBeDisabled();
			await expect(option).toContainText(
				"takes text, images, metadata, not video",
			);
			await expect(
				target
					.getByTestId("destination-reasons")
					.locator(`[data-destination-id="${created.id}"]`),
			).toContainText("not video");
		} finally {
			await api("DELETE", `/connections/${created.id}`);
		}
	});
});
